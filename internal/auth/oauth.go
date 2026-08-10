package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	OpenAICodexProviderID = "openai-codex"
	openAIIssuer          = "https://auth.openai.com"
	openAIClientID        = "app_EMoamEEZ73f0CkXaXp7hrann"
	openAIRedirectURI     = "http://localhost:1455/auth/callback"
	oauthPort             = "1455"
)

type BrowserOpener func(url string) error

type OAuthManager struct {
	Store       *Store
	HTTPClient  *http.Client
	OpenBrowser BrowserOpener
	Issuer      string
	ClientID    string
	RedirectURI string

	startServer func(context.Context, string, chan<- oauthCallbackResult) (*http.Server, error)
	refreshMu   sync.Mutex
}

type tokenResponse struct {
	IDToken      string `json:"id_token"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
}

type OAuthLoginSession struct {
	AuthURL  string
	provider string
	manager  *OAuthManager
	pkce     PKCE
	codeCh   chan oauthCallbackResult
	server   *http.Server
}

func NewOAuthManager(store *Store) *OAuthManager {
	if store == nil {
		store = NewStore()
	}
	return &OAuthManager{
		Store:       store,
		HTTPClient:  http.DefaultClient,
		OpenBrowser: OpenDefaultBrowser,
		Issuer:      openAIIssuer,
		ClientID:    openAIClientID,
		RedirectURI: openAIRedirectURI,
	}
}

func (m *OAuthManager) HasCredential(provider string) bool {
	cred, ok, err := m.Store.Get(provider)
	return err == nil && ok && cred.RefreshToken != ""
}

func (m *OAuthManager) Login(ctx context.Context, provider string) error {
	session, err := m.StartLogin(ctx, provider)
	if err != nil {
		return err
	}
	return session.Wait(ctx)
}

func (m *OAuthManager) StartLogin(ctx context.Context, provider string) (*OAuthLoginSession, error) {
	pkce, err := GeneratePKCE()
	if err != nil {
		return nil, err
	}
	state, err := GenerateState()
	if err != nil {
		return nil, err
	}

	authURL := m.AuthorizationURL(pkce.Challenge, state)
	codeCh := make(chan oauthCallbackResult, 1)
	startServer := m.startServer
	if startServer == nil {
		startServer = m.startCallbackServer
	}
	server, err := startServer(ctx, state, codeCh)
	if err != nil {
		return nil, err
	}

	if m.OpenBrowser != nil {
		_ = m.OpenBrowser(authURL)
	}

	return &OAuthLoginSession{
		AuthURL:  authURL,
		provider: provider,
		manager:  m,
		pkce:     pkce,
		codeCh:   codeCh,
		server:   server,
	}, nil
}

func (s *OAuthLoginSession) Wait(ctx context.Context) error {
	defer shutdownOAuthServer(s.server)

	select {
	case <-ctx.Done():
		return ctx.Err()
	case result := <-s.codeCh:
		if result.Err != nil {
			return result.Err
		}
		tokens, err := s.manager.exchangeCode(ctx, result.Code, s.pkce.Verifier)
		if err != nil {
			result.Respond(false, "Token exchange failed.")
			return err
		}
		cred := credentialFromTokenResponse(tokens)
		cred.AccountID = ExtractAccountID(tokens.IDToken)
		if cred.AccountID == "" {
			cred.AccountID = ExtractAccountID(tokens.AccessToken)
		}
		if err := s.manager.Store.Set(s.provider, cred); err != nil {
			result.Respond(false, "Credential storage failed.")
			return err
		}
		result.Respond(true, "Authentication successful. You can close this window and return to Keen Code.")
		return nil
	case <-time.After(5 * time.Minute):
		return fmt.Errorf("OAuth login timed out")
	}
}

func (m *OAuthManager) ValidAccessToken(ctx context.Context, provider string) (OAuthCredential, error) {
	cred, ok, err := m.Store.Get(provider)
	if err != nil {
		return OAuthCredential{}, err
	}
	if !ok || cred.RefreshToken == "" {
		return OAuthCredential{}, fmt.Errorf("not authenticated with OpenAI ChatGPT/Codex; run /model to sign in")
	}
	if time.Now().Before(cred.ExpiresAt.Add(-1*time.Minute)) && cred.AccessToken != "" {
		return cred, nil
	}

	m.refreshMu.Lock()
	defer m.refreshMu.Unlock()

	cred, ok, err = m.Store.Get(provider)
	if err != nil {
		return OAuthCredential{}, err
	}
	if !ok || cred.RefreshToken == "" {
		return OAuthCredential{}, fmt.Errorf("not authenticated with OpenAI ChatGPT/Codex; run /model to sign in")
	}
	if time.Now().Before(cred.ExpiresAt.Add(-1*time.Minute)) && cred.AccessToken != "" {
		return cred, nil
	}

	tokens, err := m.refreshToken(ctx, cred.RefreshToken)
	if err != nil {
		return OAuthCredential{}, err
	}
	if tokens.RefreshToken == "" {
		tokens.RefreshToken = cred.RefreshToken
	}
	next := credentialFromTokenResponse(tokens)
	if next.AccountID == "" {
		next.AccountID = cred.AccountID
	}
	if next.AccountID == "" {
		next.AccountID = ExtractAccountID(tokens.IDToken)
	}
	if next.AccountID == "" {
		next.AccountID = ExtractAccountID(tokens.AccessToken)
	}
	if err := m.Store.Set(provider, next); err != nil {
		return OAuthCredential{}, err
	}
	return next, nil
}

func (m *OAuthManager) AuthorizationURL(challenge, state string) string {
	params := url.Values{
		"response_type":              {"code"},
		"client_id":                  {m.ClientID},
		"redirect_uri":               {m.RedirectURI},
		"scope":                      {"openid profile email offline_access"},
		"code_challenge":             {challenge},
		"code_challenge_method":      {"S256"},
		"id_token_add_organizations": {"true"},
		"codex_cli_simplified_flow":  {"true"},
		"state":                      {state},
		"originator":                 {"keen-code"},
	}
	return strings.TrimRight(m.Issuer, "/") + "/oauth/authorize?" + params.Encode()
}

type PKCE struct {
	Verifier  string
	Challenge string
}

func GeneratePKCE() (PKCE, error) {
	verifier, err := randomBase64URL(32)
	if err != nil {
		return PKCE{}, err
	}
	sum := sha256.Sum256([]byte(verifier))
	return PKCE{
		Verifier:  verifier,
		Challenge: base64.RawURLEncoding.EncodeToString(sum[:]),
	}, nil
}

func GenerateState() (string, error) {
	return randomBase64URL(32)
}

func randomBase64URL(size int) (string, error) {
	b := make([]byte, size)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

type oauthCallbackResult struct {
	Code      string
	State     string
	Err       error
	respondCh chan callbackResponse
}

type callbackResponse struct {
	Success bool
	Message string
}

func (r oauthCallbackResult) Respond(success bool, message string) {
	if r.respondCh == nil {
		return
	}
	r.respondCh <- callbackResponse{Success: success, Message: message}
}

func (m *OAuthManager) startCallbackServer(ctx context.Context, state string, resultCh chan<- oauthCallbackResult) (*http.Server, error) {
	mux := newOAuthCallbackHandler(ctx, state, resultCh)
	server := &http.Server{
		Addr:              "localhost:" + oauthPort,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		BaseContext: func(net.Listener) context.Context {
			return ctx
		},
	}

	ln, err := net.Listen("tcp", server.Addr)
	if err != nil {
		return nil, fmt.Errorf("OAuth callback port 1455 is unavailable; close the app using it and try again: %w", err)
	}
	go func() {
		_ = server.Serve(ln)
	}()
	return server, nil
}

func newOAuthCallbackHandler(ctx context.Context, state string, resultCh chan<- oauthCallbackResult) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/auth/callback", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("state") != state {
			writeOAuthPage(w, false, "Authentication failed. Return to Keen Code and try again.")
			select {
			case resultCh <- oauthCallbackResult{Err: fmt.Errorf("OAuth state mismatch")}:
			default:
			}
			return
		}
		if errText := q.Get("error"); errText != "" {
			writeOAuthPage(w, false, "Authentication was not completed.")
			select {
			case resultCh <- oauthCallbackResult{Err: fmt.Errorf("OAuth authorization failed: %s", errText)}:
			default:
			}
			return
		}
		code := q.Get("code")
		if code == "" {
			writeOAuthPage(w, false, "Authentication failed. Return to Keen Code and try again.")
			select {
			case resultCh <- oauthCallbackResult{Err: fmt.Errorf("OAuth callback missing code")}:
			default:
			}
			return
		}

		respondCh := make(chan callbackResponse, 1)
		select {
		case resultCh <- oauthCallbackResult{Code: code, respondCh: respondCh}:
		default:
			writeOAuthPage(w, false, "Authentication already handled.")
			return
		}

		select {
		case response := <-respondCh:
			writeOAuthPage(w, response.Success, response.Message)
		case <-ctx.Done():
			writeOAuthPage(w, false, "Authentication timed out.")
		case <-time.After(30 * time.Second):
			writeOAuthPage(w, false, "Authentication timed out.")
		}
	})
	return mux
}

func writeOAuthPage(w http.ResponseWriter, success bool, message string) {
	title := "Authentication Failed"
	if success {
		title = "Authentication Successful"
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = fmt.Fprintf(w, `<!doctype html>
<html>
  <head>
    <title>Keen Code - %s</title>
    <style>
      body {
        font-family: system-ui, -apple-system, sans-serif;
        display: flex;
        align-items: center;
        justify-content: center;
        height: 100vh;
        margin: 0;
        background: #111;
        color: #f6f6f6;
      }
      .box {
        text-align: center;
        max-width: 36rem;
        padding: 2rem;
      }
      p {
        color: #c9c9c9;
      }
    </style>
  </head>
  <body>
    <main class="box">
      <h1>%s</h1>
      <p>%s</p>
    </main>
  </body>
</html>`, html.EscapeString(title), html.EscapeString(title), html.EscapeString(message))
}

func (m *OAuthManager) exchangeCode(ctx context.Context, code, verifier string) (tokenResponse, error) {
	return m.postToken(ctx, url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {m.RedirectURI},
		"client_id":     {m.ClientID},
		"code_verifier": {verifier},
	})
}

func (m *OAuthManager) refreshToken(ctx context.Context, refreshToken string) (tokenResponse, error) {
	return m.postToken(ctx, url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"client_id":     {m.ClientID},
	})
}

func (m *OAuthManager) postToken(ctx context.Context, values url.Values) (tokenResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(m.Issuer, "/")+"/oauth/token", strings.NewReader(values.Encode()))
	if err != nil {
		return tokenResponse{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	client := m.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return tokenResponse{}, err
	}
	defer resp.Body.Close()
	body, err := readTokenResponseBody(resp.Body)
	if err != nil {
		return tokenResponse{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return tokenResponse{}, fmt.Errorf("OpenAI OAuth token request failed: HTTP %d, content-type %q%s", resp.StatusCode, resp.Header.Get("Content-Type"), responseBodyDetail(body))
	}
	if len(trimTokenBody(body)) == 0 {
		return tokenResponse{}, fmt.Errorf("OpenAI OAuth token response was empty: HTTP %d, content-type %q, body-bytes %d", resp.StatusCode, resp.Header.Get("Content-Type"), len(body))
	}
	var tokens tokenResponse
	if err := json.Unmarshal(body, &tokens); err != nil {
		return tokenResponse{}, fmt.Errorf("parse OpenAI OAuth token response: HTTP %d, content-type %q, body-bytes %d: %w", resp.StatusCode, resp.Header.Get("Content-Type"), len(body), err)
	}
	if tokens.AccessToken == "" {
		return tokenResponse{}, fmt.Errorf("OpenAI OAuth token response missing access_token: HTTP %d, content-type %q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	return tokens, nil
}

func readTokenResponseBody(body io.Reader) ([]byte, error) {
	const maxTokenResponseBytes = 1 << 20
	data, err := io.ReadAll(io.LimitReader(body, maxTokenResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read OpenAI OAuth token response: %w", err)
	}
	if len(data) > maxTokenResponseBytes {
		return nil, fmt.Errorf("OpenAI OAuth token response exceeded %d bytes", maxTokenResponseBytes)
	}
	return data, nil
}

func shutdownOAuthServer(server *http.Server) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		_ = server.Close()
	}
}

func responseBodyDetail(body []byte) string {
	trimmed := string(trimTokenBody(body))
	if trimmed == "" {
		return ""
	}
	const maxLen = 300
	if len(trimmed) > maxLen {
		trimmed = trimmed[:maxLen] + "..."
	}
	return ": " + trimmed
}

func trimTokenBody(body []byte) []byte {
	return []byte(strings.Trim(string(body), "\x00 \n\r\t"))
}

func credentialFromTokenResponse(tokens tokenResponse) OAuthCredential {
	expiresIn := tokens.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = 3600
	}
	return OAuthCredential{
		Type:         "oauth",
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(expiresIn) * time.Second),
	}
}

func ExtractAccountID(token string) string {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		return ""
	}
	return accountIDFromClaims(claims)
}

func accountIDFromClaims(claims map[string]any) string {
	if v, ok := claims["chatgpt_account_id"].(string); ok && v != "" {
		return v
	}
	if authClaims, ok := claims["https://api.openai.com/auth"].(map[string]any); ok {
		if v, ok := authClaims["chatgpt_account_id"].(string); ok && v != "" {
			return v
		}
	}
	if orgs, ok := claims["organizations"].([]any); ok && len(orgs) > 0 {
		if org, ok := orgs[0].(map[string]any); ok {
			if v, ok := org["id"].(string); ok && v != "" {
				return v
			}
		}
	}
	return ""
}

func OpenDefaultBrowser(rawURL string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", rawURL)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", rawURL)
	default:
		cmd = exec.Command("xdg-open", rawURL)
	}
	return cmd.Start()
}

func FetchOAuthAuthorizationCode(ctx context.Context, authURL, redirectURL string, openBrowser BrowserOpener) (string, string, error) {
	u, err := url.Parse(redirectURL)
	if err != nil {
		return "", "", fmt.Errorf("parse OAuth redirect URL: %w", err)
	}
	if u.Host == "" {
		return "", "", fmt.Errorf("OAuth redirect URL must include host")
	}
	path := u.Path
	if path == "" {
		path = "/"
	}

	codeCh := make(chan oauthCallbackResult, 1)
	server, err := startOAuthCallbackServer(ctx, u.Host, path, codeCh)
	if err != nil {
		return "", "", err
	}
	defer shutdownOAuthServer(server)

	if openBrowser == nil {
		openBrowser = OpenDefaultBrowser
	}
	if err := openBrowser(authURL); err != nil {
		return "", "", err
	}

	select {
	case <-ctx.Done():
		return "", "", ctx.Err()
	case result := <-codeCh:
		if result.Err != nil {
			return "", "", result.Err
		}
		result.Respond(true, "Authentication successful. You can close this window and return to Keen Code.")
		return result.Code, result.State, nil
	case <-time.After(5 * time.Minute):
		return "", "", fmt.Errorf("OAuth login timed out")
	}
}

func newAuthorizationCallbackHandler(ctx context.Context, path string, resultCh chan<- oauthCallbackResult) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if errText := q.Get("error"); errText != "" {
			writeOAuthPage(w, false, "Authentication was not completed.")
			select {
			case resultCh <- oauthCallbackResult{Err: fmt.Errorf("OAuth authorization failed: %s", errText)}:
			default:
			}
			return
		}
		code := q.Get("code")
		if code == "" {
			writeOAuthPage(w, false, "Authentication failed. Return to Keen Code and try again.")
			select {
			case resultCh <- oauthCallbackResult{Err: fmt.Errorf("OAuth callback missing code")}:
			default:
			}
			return
		}

		respondCh := make(chan callbackResponse, 1)
		select {
		case resultCh <- oauthCallbackResult{Code: code, State: q.Get("state"), respondCh: respondCh}:
		default:
			writeOAuthPage(w, false, "Authentication already handled.")
			return
		}

		select {
		case response := <-respondCh:
			writeOAuthPage(w, response.Success, response.Message)
		case <-ctx.Done():
			writeOAuthPage(w, false, "Authentication timed out.")
		case <-time.After(30 * time.Second):
			writeOAuthPage(w, false, "Authentication timed out.")
		}
	})
	return mux
}

func startOAuthCallbackServer(ctx context.Context, host, path string, resultCh chan<- oauthCallbackResult) (*http.Server, error) {
	mux := newAuthorizationCallbackHandler(ctx, path, resultCh)
	server := &http.Server{
		Addr:              host,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		BaseContext: func(net.Listener) context.Context {
			return ctx
		},
	}

	ln, err := net.Listen("tcp", server.Addr)
	if err != nil {
		return nil, fmt.Errorf("OAuth callback address %s is unavailable; close the app using it and try again: %w", server.Addr, err)
	}
	go func() {
		_ = server.Serve(ln)
	}()
	return server, nil
}
