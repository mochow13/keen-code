package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewOAuthManagerSetsDefaults(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	manager := NewOAuthManager(nil)
	if manager.Store == nil || manager.HTTPClient == nil || manager.OpenBrowser == nil {
		t.Fatal("NewOAuthManager did not set defaults")
	}
}

func TestGenerateState(t *testing.T) {
	state, err := GenerateState()
	if err != nil {
		t.Fatalf("GenerateState() error = %v", err)
	}
	if len(state) < 40 {
		t.Fatalf("GenerateState() returned short state %q", state)
	}
}

func TestAuthorizationURL(t *testing.T) {
	manager := NewOAuthManager(NewStoreAt(filepath.Join(t.TempDir(), "auth.json")))
	manager.Issuer = "https://issuer.example/"
	manager.ClientID = "client"
	manager.RedirectURI = "http://localhost/callback"

	authURL, err := url.Parse(manager.AuthorizationURL("challenge", "state"))
	if err != nil {
		t.Fatalf("parse authorization URL: %v", err)
	}
	if authURL.Path != "/oauth/authorize" || authURL.Query().Get("code_challenge") != "challenge" || authURL.Query().Get("state") != "state" {
		t.Fatalf("unexpected authorization URL %s", authURL)
	}
}

func TestHasCredentialAndValidAccessToken(t *testing.T) {
	store := NewStoreAt(filepath.Join(t.TempDir(), "auth.json"))
	manager := NewOAuthManager(store)
	if manager.HasCredential("provider") {
		t.Fatal("HasCredential returned true for an empty store")
	}

	cred := OAuthCredential{AccessToken: "access", RefreshToken: "refresh", ExpiresAt: time.Now().Add(time.Hour)}
	if err := store.Set("provider", cred); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if !manager.HasCredential("provider") {
		t.Fatal("HasCredential returned false for a stored credential")
	}
	got, err := manager.ValidAccessToken(context.Background(), "provider")
	if err != nil || got.AccessToken != "access" {
		t.Fatalf("ValidAccessToken() = %#v, %v", got, err)
	}
	if _, err := manager.ValidAccessToken(context.Background(), "missing"); err == nil {
		t.Fatal("ValidAccessToken succeeded without a credential")
	}
}

func TestPostTokenRejectsInvalidResponses(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{name: "HTTP error", status: http.StatusUnauthorized, body: "denied", want: "HTTP 401"},
		{name: "missing access token", status: http.StatusOK, body: `{}`, want: "missing access_token"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := NewOAuthManager(NewStoreAt(filepath.Join(t.TempDir(), "auth.json")))
			manager.Issuer = "https://issuer.example/"
			manager.HTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: tt.status,
					Header:     http.Header{"Content-Type": {"application/json"}},
					Body:       io.NopCloser(strings.NewReader(tt.body)),
					Request:    req,
				}, nil
			})}
			_, err := manager.postToken(context.Background(), url.Values{"grant_type": {"refresh_token"}})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("postToken() error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestReadTokenResponseBodyRejectsInvalidBodies(t *testing.T) {
	if _, err := readTokenResponseBody(errorReader{}); err == nil {
		t.Fatal("readTokenResponseBody accepted a reader error")
	}
	if _, err := readTokenResponseBody(strings.NewReader(strings.Repeat("x", (1<<20)+1))); err == nil {
		t.Fatal("readTokenResponseBody accepted an oversized response")
	}
}

func TestExtractAccountIDFallbacks(t *testing.T) {
	claims := []map[string]any{
		{"chatgpt_account_id": "direct"},
		{"organizations": []any{map[string]any{"id": "org"}}},
	}
	for _, claim := range claims {
		if got := ExtractAccountID(jwtForClaims(t, claim)); got == "" {
			t.Fatalf("ExtractAccountID(%#v) returned empty ID", claim)
		}
	}
	for _, token := range []string{"", "a.b", "a.!.c", "a." + base64.RawURLEncoding.EncodeToString([]byte("not-json")) + ".c"} {
		if got := ExtractAccountID(token); got != "" {
			t.Fatalf("ExtractAccountID(%q) = %q", token, got)
		}
	}
}

func TestOAuthPageEscapesContent(t *testing.T) {
	recorder := httptest.NewRecorder()
	writeOAuthPage(recorder, true, `<script>alert(1)</script>`)
	body := recorder.Body.String()
	if !strings.Contains(body, "Authentication Successful") || strings.Contains(body, "<script>") {
		t.Fatalf("unexpected OAuth page %q", body)
	}
}

func TestGeneratePKCEProducesMatchingChallenge(t *testing.T) {
	pkce, err := GeneratePKCE()
	if err != nil {
		t.Fatalf("GeneratePKCE() error = %v", err)
	}
	if pkce.Verifier == "" || pkce.Challenge == "" {
		t.Fatalf("GeneratePKCE() = %#v, want non-empty values", pkce)
	}
	if pkce.Verifier == pkce.Challenge {
		t.Fatal("PKCE challenge must be derived from, not equal to, verifier")
	}
}

func TestOAuthCallbackResultRespond(t *testing.T) {
	result := oauthCallbackResult{}
	result.Respond(true, "ignored")

	responses := make(chan callbackResponse, 1)
	result.respondCh = responses
	result.Respond(false, "failed")
	got := <-responses
	if got.Success || got.Message != "failed" {
		t.Fatalf("Respond() sent %#v", got)
	}
}

func TestPostTokenRequestAndResponseBehavior(t *testing.T) {
	tests := []struct {
		name        string
		status      int
		contentType string
		body        string
		wantToken   string
		wantErr     string
	}{
		{name: "success", status: http.StatusOK, contentType: "application/json", body: `{"access_token":"access","refresh_token":"refresh","expires_in":120}`, wantToken: "access"},
		{name: "empty", status: http.StatusOK, contentType: "application/json", body: " \x00\n", wantErr: "response was empty"},
		{name: "invalid JSON", status: http.StatusOK, contentType: "text/plain", body: "not-json", wantErr: "parse OpenAI OAuth token response"},
		{name: "long HTTP error", status: http.StatusBadRequest, contentType: "text/plain", body: strings.Repeat("x", 400), wantErr: strings.Repeat("x", 300) + "..."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var request *http.Request
			manager := NewOAuthManager(NewStoreAt(filepath.Join(t.TempDir(), "auth.json")))
			manager.Issuer = "https://issuer.example/"
			manager.HTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				request = req
				return &http.Response{
					StatusCode: tt.status,
					Header:     http.Header{"Content-Type": {tt.contentType}},
					Body:       io.NopCloser(strings.NewReader(tt.body)),
					Request:    req,
				}, nil
			})}

			tokens, err := manager.refreshToken(context.Background(), "refresh value")
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("refreshToken() error = %v, want %q", err, tt.wantErr)
				}
			} else if err != nil || tokens.AccessToken != tt.wantToken {
				t.Fatalf("refreshToken() = %#v, %v", tokens, err)
			}
			if request == nil || request.URL.String() != "https://issuer.example/oauth/token" {
				t.Fatalf("request URL = %v", request)
			}
			if err := request.ParseForm(); err != nil {
				t.Fatalf("ParseForm() error = %v", err)
			}
			if request.Form.Get("grant_type") != "refresh_token" || request.Form.Get("refresh_token") != "refresh value" {
				t.Fatalf("request form = %v", request.Form)
			}
		})
	}
}

func TestPostTokenTransportAndRequestErrors(t *testing.T) {
	manager := NewOAuthManager(NewStoreAt(filepath.Join(t.TempDir(), "auth.json")))
	manager.HTTPClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("transport failed")
	})}
	if _, err := manager.postToken(context.Background(), url.Values{}); err == nil || !strings.Contains(err.Error(), "transport failed") {
		t.Fatalf("postToken() transport error = %v", err)
	}

	manager.Issuer = "://invalid"
	if _, err := manager.postToken(context.Background(), url.Values{}); err == nil {
		t.Fatal("postToken() accepted an invalid issuer URL")
	}
}

func TestValidAccessTokenRefreshesExpiredCredential(t *testing.T) {
	store := NewStoreAt(filepath.Join(t.TempDir(), "auth.json"))
	if err := store.Set("provider", OAuthCredential{
		AccessToken:  "expired",
		RefreshToken: "old-refresh",
		AccountID:    "existing-account",
		ExpiresAt:    time.Now().Add(-time.Hour),
	}); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	manager := NewOAuthManager(store)
	manager.HTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"access_token":"fresh","expires_in":60}`)),
			Request:    req,
		}, nil
	})}

	got, err := manager.ValidAccessToken(context.Background(), "provider")
	if err != nil {
		t.Fatalf("ValidAccessToken() error = %v", err)
	}
	if got.AccessToken != "fresh" || got.RefreshToken != "old-refresh" || got.AccountID != "existing-account" {
		t.Fatalf("ValidAccessToken() = %#v", got)
	}
	stored, ok, err := store.Get("provider")
	if err != nil || !ok || stored.AccessToken != "fresh" {
		t.Fatalf("stored credential = %#v, %t, %v", stored, ok, err)
	}
}

func TestTokenHelpers(t *testing.T) {
	credential := credentialFromTokenResponse(tokenResponse{AccessToken: "access", RefreshToken: "refresh"})
	if credential.Type != "oauth" || credential.ExpiresAt.Before(time.Now().Add(59*time.Minute)) {
		t.Fatalf("credentialFromTokenResponse() = %#v", credential)
	}

	if got := responseBodyDetail([]byte(" \x00\n")); got != "" {
		t.Fatalf("responseBodyDetail(empty) = %q", got)
	}
	if got := responseBodyDetail([]byte(" denied \n")); got != ": denied" {
		t.Fatalf("responseBodyDetail() = %q", got)
	}

	nested := map[string]any{"https://api.openai.com/auth": map[string]any{"chatgpt_account_id": "nested"}}
	if got := accountIDFromClaims(nested); got != "nested" {
		t.Fatalf("accountIDFromClaims(nested) = %q", got)
	}
	if got := accountIDFromClaims(map[string]any{}); got != "" {
		t.Fatalf("accountIDFromClaims(empty) = %q", got)
	}
}

func TestFetchOAuthAuthorizationCodeRejectsInvalidRedirect(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, redirect := range []string{"http://[::1", "/callback"} {
		if _, _, err := FetchOAuthAuthorizationCode(ctx, "https://auth.example", redirect, func(string) error { return nil }); err == nil {
			t.Fatalf("FetchOAuthAuthorizationCode() accepted redirect %q", redirect)
		}
	}
}

func TestExchangeCodeSendsAuthorizationFields(t *testing.T) {
	manager := NewOAuthManager(NewStoreAt(filepath.Join(t.TempDir(), "auth.json")))
	manager.ClientID = "client-id"
	manager.RedirectURI = "http://localhost/callback"
	manager.HTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if err := req.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error = %v", err)
		}
		for key, want := range map[string]string{
			"grant_type":    "authorization_code",
			"code":          "auth-code",
			"redirect_uri":  "http://localhost/callback",
			"client_id":     "client-id",
			"code_verifier": "verifier",
		} {
			if got := req.Form.Get(key); got != want {
				t.Fatalf("form[%q] = %q, want %q", key, got, want)
			}
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"access_token":"access"}`)),
			Request:    req,
		}, nil
	})}
	if _, err := manager.exchangeCode(context.Background(), "auth-code", "verifier"); err != nil {
		t.Fatalf("exchangeCode() error = %v", err)
	}
}

func TestStartLoginUsesInjectedServerAndBrowser(t *testing.T) {
	manager := NewOAuthManager(NewStoreAt(filepath.Join(t.TempDir(), "auth.json")))
	var gotState, openedURL string
	manager.startServer = func(_ context.Context, state string, _ chan<- oauthCallbackResult) (*http.Server, error) {
		gotState = state
		return &http.Server{}, nil
	}
	manager.OpenBrowser = func(rawURL string) error {
		openedURL = rawURL
		return nil
	}

	session, err := manager.StartLogin(context.Background(), "provider")
	if err != nil {
		t.Fatalf("StartLogin() error = %v", err)
	}
	if session.provider != "provider" || session.pkce.Verifier == "" || gotState == "" || openedURL != session.AuthURL {
		t.Fatalf("StartLogin() session = %#v, state = %q, opened URL = %q", session, gotState, openedURL)
	}
}

func TestStartLoginReturnsServerError(t *testing.T) {
	manager := NewOAuthManager(NewStoreAt(filepath.Join(t.TempDir(), "auth.json")))
	manager.startServer = func(context.Context, string, chan<- oauthCallbackResult) (*http.Server, error) {
		return nil, errors.New("server failed")
	}
	if _, err := manager.StartLogin(context.Background(), "provider"); err == nil || !strings.Contains(err.Error(), "server failed") {
		t.Fatalf("StartLogin() error = %v", err)
	}
}

func TestOAuthLoginSessionWaitOutcomes(t *testing.T) {
	t.Run("context canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		session := &OAuthLoginSession{codeCh: make(chan oauthCallbackResult), server: &http.Server{}}
		if err := session.Wait(ctx); !errors.Is(err, context.Canceled) {
			t.Fatalf("Wait() error = %v", err)
		}
	})

	t.Run("callback error", func(t *testing.T) {
		codeCh := make(chan oauthCallbackResult, 1)
		codeCh <- oauthCallbackResult{Err: errors.New("callback failed")}
		session := &OAuthLoginSession{codeCh: codeCh, server: &http.Server{}}
		if err := session.Wait(context.Background()); err == nil || !strings.Contains(err.Error(), "callback failed") {
			t.Fatalf("Wait() error = %v", err)
		}
	})

	t.Run("successful exchange", func(t *testing.T) {
		store := NewStoreAt(filepath.Join(t.TempDir(), "auth.json"))
		manager := NewOAuthManager(store)
		manager.HTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"access_token":"access","refresh_token":"refresh"}`)), Request: req}, nil
		})}
		responses := make(chan callbackResponse, 1)
		codeCh := make(chan oauthCallbackResult, 1)
		codeCh <- oauthCallbackResult{Code: "code", respondCh: responses}
		session := &OAuthLoginSession{provider: "provider", manager: manager, pkce: PKCE{Verifier: "verifier"}, codeCh: codeCh, server: &http.Server{}}
		if err := session.Wait(context.Background()); err != nil {
			t.Fatalf("Wait() error = %v", err)
		}
		if response := <-responses; !response.Success {
			t.Fatalf("callback response = %#v", response)
		}
		if cred, ok, err := store.Get("provider"); err != nil || !ok || cred.AccessToken != "access" {
			t.Fatalf("stored credential = %#v, %t, %v", cred, ok, err)
		}
	})
}

func TestOAuthCallbackHandlersRejectInvalidCallbacks(t *testing.T) {
	tests := []struct {
		name    string
		handler func(context.Context, chan<- oauthCallbackResult) http.Handler
		target  string
		wantErr string
	}{
		{name: "state mismatch", handler: func(ctx context.Context, ch chan<- oauthCallbackResult) http.Handler {
			return newOAuthCallbackHandler(ctx, "expected", ch)
		}, target: "/auth/callback?state=wrong&code=code", wantErr: "state mismatch"},
		{name: "authorization error", handler: func(ctx context.Context, ch chan<- oauthCallbackResult) http.Handler {
			return newOAuthCallbackHandler(ctx, "expected", ch)
		}, target: "/auth/callback?state=expected&error=denied", wantErr: "authorization failed"},
		{name: "missing code", handler: func(ctx context.Context, ch chan<- oauthCallbackResult) http.Handler {
			return newOAuthCallbackHandler(ctx, "expected", ch)
		}, target: "/auth/callback?state=expected", wantErr: "missing code"},
		{name: "generic authorization error", handler: func(ctx context.Context, ch chan<- oauthCallbackResult) http.Handler {
			return newAuthorizationCallbackHandler(ctx, "/callback", ch)
		}, target: "/callback?error=denied", wantErr: "authorization failed"},
		{name: "generic missing code", handler: func(ctx context.Context, ch chan<- oauthCallbackResult) http.Handler {
			return newAuthorizationCallbackHandler(ctx, "/callback", ch)
		}, target: "/callback", wantErr: "missing code"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := make(chan oauthCallbackResult, 1)
			recorder := httptest.NewRecorder()
			tt.handler(context.Background(), results).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, tt.target, nil))
			result := <-results
			if result.Err == nil || !strings.Contains(result.Err.Error(), tt.wantErr) || recorder.Code != http.StatusOK {
				t.Fatalf("callback result = %#v, status = %d", result, recorder.Code)
			}
		})
	}
}

func TestAuthorizationCallbackHandlerCompletesAndCancels(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		results := make(chan oauthCallbackResult)
		done := make(chan *httptest.ResponseRecorder, 1)
		handler := newAuthorizationCallbackHandler(context.Background(), "/callback", results)
		go func() {
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/callback?code=code&state=state", nil))
			done <- recorder
		}()
		result := <-results
		if result.Code != "code" || result.State != "state" {
			t.Fatalf("callback result = %#v", result)
		}
		result.Respond(true, "done")
		if recorder := <-done; !strings.Contains(recorder.Body.String(), "done") {
			t.Fatalf("callback body = %q", recorder.Body.String())
		}
	})

	t.Run("canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		results := make(chan oauthCallbackResult)
		done := make(chan *httptest.ResponseRecorder, 1)
		handler := newAuthorizationCallbackHandler(ctx, "/callback", results)
		go func() {
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/callback?code=code", nil))
			done <- recorder
		}()
		<-results
		cancel()
		if recorder := <-done; !strings.Contains(recorder.Body.String(), "timed out") {
			t.Fatalf("callback body = %q", recorder.Body.String())
		}
	})
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) { return 0, errors.New("boom") }

func jwtForClaims(t *testing.T, claims map[string]any) string {
	t.Helper()
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	return "header." + base64.RawURLEncoding.EncodeToString(payload) + ".signature"
}
