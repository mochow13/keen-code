package mcp

import (
	"context"
	"net/http"
	"strings"

	keenauth "github.com/mochow13/keen-code/internal/auth"
	mcpauth "github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
	"golang.org/x/oauth2"
)

const DefaultOAuthRedirectURL = "http://localhost:1456/auth/mcp/callback"

func mcpAuthProvider(server string) string {
	return "mcp:" + server
}

func NewBrowserOAuthCodeFetcher(redirectURL string) mcpauth.AuthorizationCodeFetcher {
	if redirectURL == "" {
		redirectURL = DefaultOAuthRedirectURL
	}
	return func(ctx context.Context, args *mcpauth.AuthorizationArgs) (*mcpauth.AuthorizationResult, error) {
		code, state, err := keenauth.FetchOAuthAuthorizationCode(ctx, args.URL, redirectURL, nil)
		if err != nil {
			return nil, err
		}
		return &mcpauth.AuthorizationResult{Code: code, State: state}, nil
	}
}

func (m *Manager) oauthToken(server string, forceReauth bool) *oauth2.Token {
	if forceReauth {
		return nil
	}
	token := m.oauthTokens[server]
	if token == nil {
		return nil
	}
	copy := *token
	return &copy
}

func (m *Manager) clearOAuthToken(server string) error {
	return m.saveOAuthToken(context.Background(), server, nil)
}

func (m *Manager) saveOAuthToken(_ context.Context, server string, token *oauth2.Token) error {
	if err := saveOAuthToken(m.opts.authStore, server, token); err != nil {
		return err
	}
	if token == nil {
		delete(m.oauthTokens, server)
		return nil
	}
	copy := *token
	m.oauthTokens[server] = &copy
	return nil
}

type cachingOAuthHandler struct {
	server    string
	token     *oauth2.Token
	saveToken func(context.Context, string, *oauth2.Token) error
	inner     mcpauth.OAuthHandler
	source    oauth2.TokenSource
	hadToken  bool
}

func (h *cachingOAuthHandler) TokenSource(ctx context.Context) (oauth2.TokenSource, error) {
	if h.source != nil {
		return h.source, nil
	}
	if h.token != nil && h.token.AccessToken != "" {
		h.hadToken = true
		if h.token.Valid() {
			h.source = oauth2.StaticTokenSource(h.token)
			return h.source, nil
		}
	}
	if h.inner == nil {
		return nil, nil
	}
	source, err := h.inner.TokenSource(ctx)
	if err != nil {
		return nil, err
	}
	h.source = source
	return source, nil
}

func (h *cachingOAuthHandler) Authorize(ctx context.Context, req *http.Request, resp *http.Response) error {
	if h.inner == nil {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		if h.hadToken {
			return ErrAuthFailed
		}
		return ErrAuthRequired
	}
	if err := h.inner.Authorize(ctx, req, resp); err != nil {
		return err
	}
	source, err := h.inner.TokenSource(ctx)
	if err != nil {
		return err
	}
	h.source = source
	if h.saveToken == nil || source == nil {
		return nil
	}
	token, err := source.Token()
	if err != nil {
		return nil
	}
	if err := h.saveToken(ctx, h.server, token); err != nil {
		return err
	}
	h.token = token
	return nil
}

func newOAuthHandler(server string, cfg ServerConfig, opts managerOptions, token *oauth2.Token, saveToken func(context.Context, string, *oauth2.Token) error) (mcpauth.OAuthHandler, error) {
	if opts.oauthCodeFetcher == nil {
		return &cachingOAuthHandler{server: server, token: token}, nil
	}
	if opts.oauthRedirectURL == "" {
		opts.oauthRedirectURL = DefaultOAuthRedirectURL
	}

	handlerCfg := &mcpauth.AuthorizationCodeHandlerConfig{
		RedirectURL:              opts.oauthRedirectURL,
		AuthorizationCodeFetcher: opts.oauthCodeFetcher,
		Client:                   opts.httpClient,
	}
	if opts.oauthClientMetadataURL != "" {
		handlerCfg.ClientIDMetadataDocumentConfig = &mcpauth.ClientIDMetadataDocumentConfig{URL: opts.oauthClientMetadataURL}
	}
	if opts.oauthPreregisteredID != "" {
		client := &oauthex.ClientCredentials{ClientID: opts.oauthPreregisteredID}
		if opts.oauthPreregisteredSecret != "" {
			client.ClientSecretAuth = &oauthex.ClientSecretAuth{ClientSecret: opts.oauthPreregisteredSecret}
		}
		handlerCfg.PreregisteredClient = client
	}
	if handlerCfg.ClientIDMetadataDocumentConfig == nil && handlerCfg.PreregisteredClient == nil {
		scopes := strings.Join(cfg.Auth.Scopes, " ")
		handlerCfg.DynamicClientRegistrationConfig = &mcpauth.DynamicClientRegistrationConfig{
			Metadata: &oauthex.ClientRegistrationMetadata{
				ClientName:              opts.oauthClientName,
				RedirectURIs:            []string{opts.oauthRedirectURL},
				GrantTypes:              []string{"authorization_code"},
				ResponseTypes:           []string{"code"},
				TokenEndpointAuthMethod: "client_secret_post",
				Scope:                   scopes,
			},
		}
	}

	handler, err := mcpauth.NewAuthorizationCodeHandler(handlerCfg)
	if err != nil {
		return nil, err
	}
	return &cachingOAuthHandler{server: server, token: token, saveToken: saveToken, inner: handler}, nil
}

func loadOAuthTokens(store *keenauth.Store, runtime map[string]*serverRuntime) (map[string]*oauth2.Token, error) {
	tokens := map[string]*oauth2.Token{}
	if store == nil {
		return tokens, nil
	}
	creds, err := store.All()
	if err != nil {
		return nil, err
	}
	for provider, cred := range creds {
		if !strings.HasPrefix(provider, "mcp:") || cred.AccessToken == "" {
			continue
		}
		server := strings.TrimPrefix(provider, "mcp:")
		rt := runtime[server]
		if rt == nil || rt.config.Auth.Type != AuthOAuth {
			continue
		}
		tokens[server] = &oauth2.Token{
			AccessToken:  cred.AccessToken,
			RefreshToken: cred.RefreshToken,
			Expiry:       cred.ExpiresAt,
			TokenType:    "Bearer",
		}
	}
	return tokens, nil
}

func saveOAuthToken(store *keenauth.Store, server string, token *oauth2.Token) error {
	if store == nil {
		return nil
	}
	provider := mcpAuthProvider(server)
	if token == nil {
		return store.Remove(provider)
	}
	return store.Set(provider, keenauth.OAuthCredential{
		Type:         "oauth",
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		ExpiresAt:    token.Expiry,
	})
}
