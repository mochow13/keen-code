package mcp

import (
	"net/http"
	"time"

	keenauth "github.com/mochow13/keen-code/internal/auth"
	mcpauth "github.com/modelcontextprotocol/go-sdk/auth"
)

type Option func(*managerOptions)
type RefreshOption func(*managerOptions)

type managerOptions struct {
	httpClient               *http.Client
	authStore                *keenauth.Store
	oauthRedirectURL         string
	oauthCodeFetcher         mcpauth.AuthorizationCodeFetcher
	oauthForceReauth         bool
	connectTimeout           time.Duration
	listToolsTimeout         time.Duration
	callToolTimeout          time.Duration
	stdioTerminateTimeout    time.Duration
	streamableMaxRetries     int
	disableStandaloneSSE     bool
	oauthClientName          string
	oauthClientMetadataURL   string
	oauthPreregisteredID     string
	oauthPreregisteredSecret string
}

func defaultOptions() managerOptions {
	return managerOptions{
		httpClient:            http.DefaultClient,
		authStore:             keenauth.NewStore(),
		connectTimeout:        30 * time.Second,
		listToolsTimeout:      15 * time.Second,
		callToolTimeout:       2 * time.Minute,
		stdioTerminateTimeout: 5 * time.Second,
		streamableMaxRetries:  5,
		oauthClientName:       "Keen Code",
	}
}

func WithHTTPClient(client *http.Client) Option {
	return func(o *managerOptions) {
		if client != nil {
			o.httpClient = client
		}
	}
}

func WithAuthStore(store *keenauth.Store) Option {
	return func(o *managerOptions) {
		if store != nil {
			o.authStore = store
		}
	}
}

func WithOAuthRedirectURL(redirectURL string) Option {
	return func(o *managerOptions) {
		o.oauthRedirectURL = redirectURL
	}
}

func WithOAuthAuthorizationCodeFetcher(fetcher mcpauth.AuthorizationCodeFetcher) Option {
	return func(o *managerOptions) {
		o.oauthCodeFetcher = fetcher
	}
}

func WithOAuthClientName(name string) Option {
	return func(o *managerOptions) {
		if name != "" {
			o.oauthClientName = name
		}
	}
}

func WithOAuthClientIDMetadataDocument(url string) Option {
	return func(o *managerOptions) {
		o.oauthClientMetadataURL = url
	}
}

func WithOAuthPreregisteredClient(clientID, clientSecret string) Option {
	return func(o *managerOptions) {
		o.oauthPreregisteredID = clientID
		o.oauthPreregisteredSecret = clientSecret
	}
}

func WithTimeouts(connect, listTools, callTool time.Duration) Option {
	return func(o *managerOptions) {
		if connect > 0 {
			o.connectTimeout = connect
		}
		if listTools > 0 {
			o.listToolsTimeout = listTools
		}
		if callTool > 0 {
			o.callToolTimeout = callTool
		}
	}
}

func WithStdioTerminateTimeout(timeout time.Duration) Option {
	return func(o *managerOptions) {
		if timeout > 0 {
			o.stdioTerminateTimeout = timeout
		}
	}
}

func WithStreamableMaxRetries(maxRetries int) Option {
	return func(o *managerOptions) {
		o.streamableMaxRetries = maxRetries
	}
}

func WithStandaloneSSEDisabled(disabled bool) Option {
	return func(o *managerOptions) {
		o.disableStandaloneSSE = disabled
	}
}

func WithRefreshOAuthAuthorizationCodeFetcher(fetcher mcpauth.AuthorizationCodeFetcher) RefreshOption {
	return func(o *managerOptions) {
		o.oauthCodeFetcher = fetcher
	}
}

func WithRefreshOAuthRedirectURL(redirectURL string) RefreshOption {
	return func(o *managerOptions) {
		o.oauthRedirectURL = redirectURL
	}
}

func WithRefreshOAuthForceReauth(force bool) RefreshOption {
	return func(o *managerOptions) {
		o.oauthForceReauth = force
	}
}

func WithRefreshConnectTimeout(timeout time.Duration) RefreshOption {
	return func(o *managerOptions) {
		if timeout > 0 {
			o.connectTimeout = timeout
		}
	}
}
