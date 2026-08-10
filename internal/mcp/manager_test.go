package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	keenauth "github.com/mochow13/keen-code/internal/auth"
	mcpauth "github.com/modelcontextprotocol/go-sdk/auth"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/oauth2"
)

type echoInput struct {
	Message string `json:"message" jsonschema:"message to echo"`
}

type echoOutput struct {
	Message string `json:"message"`
}

func TestManagerStreamableHTTPListAndCallTool(t *testing.T) {
	server := newTestMCPServer()
	httpServer := httptest.NewServer(mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server {
		return server
	}, nil))
	t.Cleanup(httpServer.Close)

	manager := newTestManager(t, map[string]ServerConfig{
		"test": {
			URL:  httpServer.URL,
			Auth: AuthConfig{Type: AuthNone},
		},
	})
	startAndWait(t, manager, "test", StateConnected)

	tools, err := manager.ListTools(context.Background(), "test")
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	if len(tools) != 2 {
		t.Fatalf("ListTools() length = %d, want 2", len(tools))
	}
	if status := manager.Status("test"); status.Description != testServerInstructions {
		t.Fatalf("Status().Description = %q, want %q", status.Description, testServerInstructions)
	}

	result, err := manager.CallTool(context.Background(), "test", "echo", map[string]any{"message": "hello"})
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if result.IsError {
		t.Fatalf("CallTool() IsError = true")
	}
	if got := result.StructuredContent.(map[string]any)["message"]; got != "hello" {
		t.Fatalf("structured message = %v, want hello", got)
	}
}

func TestManagerAPIKeyHeaderAndRedaction(t *testing.T) {
	const secret = "ctx7-secret"
	var sawHeader bool
	server := newTestMCPServer()
	handler := mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server {
		return server
	}, nil)
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("CONTEXT7_API_KEY") == secret {
			sawHeader = true
		} else {
			http.Error(w, "missing api key "+secret, http.StatusUnauthorized)
			return
		}
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(httpServer.Close)

	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	restoreDefaultLogger(t, logger)
	manager := newTestManagerWithOptions(t, map[string]ServerConfig{
		"context7": {
			URL: httpServer.URL,
			Auth: AuthConfig{
				Type:   AuthAPIKey,
				Header: "CONTEXT7_API_KEY",
				Scheme: "",
				Key:    secret,
			},
		},
	})
	startAndWait(t, manager, "context7", StateConnected)
	if !sawHeader {
		t.Fatal("server did not receive API key header")
	}
	if strings.Contains(logs.String(), secret) {
		t.Fatal("logs contain API key")
	}

	redacted := manager.redactError("context7", errors.New("bad "+secret))
	if strings.Contains(redacted, secret) || !strings.Contains(redacted, "[redacted]") {
		t.Fatalf("redacted error = %q", redacted)
	}
}

func TestManagerFailureLogsRedactedState(t *testing.T) {
	const secret = "ctx7-secret"
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	restoreDefaultLogger(t, logger)
	manager := newTestManagerWithOptions(t, map[string]ServerConfig{
		"context7": {
			URL: "https://example.com/mcp",
			Auth: AuthConfig{
				Type:   AuthAPIKey,
				Header: "CONTEXT7_API_KEY",
				Scheme: "",
				Key:    secret,
			},
		},
	})

	manager.setFailure("context7", errors.New("bad "+secret))

	got := logs.String()
	if !strings.Contains(got, "mcp server unavailable") {
		t.Fatalf("logs = %q, want unavailable message", got)
	}
	if !strings.Contains(got, "state=disconnected") {
		t.Fatalf("logs = %q, want disconnected state", got)
	}
	if strings.Contains(got, secret) {
		t.Fatalf("logs contain secret: %q", got)
	}
	if !strings.Contains(got, "[redacted]") {
		t.Fatalf("logs = %q, want redacted marker", got)
	}
}

func TestManagerCallToolErrors(t *testing.T) {
	server := newTestMCPServer()
	httpServer := httptest.NewServer(mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server {
		return server
	}, nil))
	t.Cleanup(httpServer.Close)

	manager := newTestManager(t, map[string]ServerConfig{
		"test": {URL: httpServer.URL, Auth: AuthConfig{Type: AuthNone}},
	})
	startAndWait(t, manager, "test", StateConnected)

	if _, err := manager.CallTool(context.Background(), "missing", "echo", nil); !errors.Is(err, ErrServerNotConfigured) {
		t.Fatalf("unknown server error = %v, want ErrServerNotConfigured", err)
	}
	if _, err := manager.CallTool(context.Background(), "test", "missing", nil); !errors.Is(err, ErrToolNotFound) {
		t.Fatalf("unknown tool error = %v, want ErrToolNotFound", err)
	}
	result, err := manager.CallTool(context.Background(), "test", "fail", nil)
	if !errors.Is(err, ErrRemoteTool) {
		t.Fatalf("remote tool error = %v, want ErrRemoteTool", err)
	}
	if result == nil || !result.IsError {
		t.Fatalf("remote tool result = %#v, want IsError", result)
	}
}

func TestManagerOAuthWithoutHooksMarksAuthRequired(t *testing.T) {
	server := newTestMCPServer()
	handler := mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server {
		return server
	}, nil)
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+r.Host+`"`)
			http.Error(w, "missing token", http.StatusUnauthorized)
			return
		}
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(httpServer.Close)

	manager := newTestManager(t, map[string]ServerConfig{
		"posthog": {URL: httpServer.URL, Auth: AuthConfig{Type: AuthOAuth}},
	})
	startAndWait(t, manager, "posthog", StateAuthRequired)
	status := manager.Status("posthog")
	if !strings.Contains(status.LastError, ErrAuthRequired.Error()) {
		t.Fatalf("LastError = %q, want auth required", status.LastError)
	}
}

func TestManagerOAuthStoredTokenConnectsWithoutFetcher(t *testing.T) {
	const token = "stored-token"
	server := newTestMCPServer()
	handler := mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server {
		return server
	}, nil)
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			http.Error(w, "missing token", http.StatusUnauthorized)
			return
		}
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(httpServer.Close)

	authStore := keenauth.NewStoreAt(filepath.Join(t.TempDir(), "auth.json"))
	if err := saveOAuthToken(authStore, "posthog", &oauth2.Token{
		AccessToken: token,
		TokenType:   "Bearer",
		Expiry:      time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	manager := newTestManagerWithOptions(t, map[string]ServerConfig{
		"posthog": {URL: httpServer.URL, Auth: AuthConfig{Type: AuthOAuth}},
	}, WithAuthStore(authStore))
	startAndWait(t, manager, "posthog", StateConnected)
}

func TestManagerRefreshOAuthForceReauthClearsStoredToken(t *testing.T) {
	const token = "stored-token"
	server := newTestMCPServer()
	handler := mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server {
		return server
	}, nil)
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+r.Host+`"`)
			http.Error(w, "missing token", http.StatusUnauthorized)
			return
		}
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(httpServer.Close)

	authStore := keenauth.NewStoreAt(filepath.Join(t.TempDir(), "auth.json"))
	if err := saveOAuthToken(authStore, "posthog", &oauth2.Token{
		AccessToken: token,
		TokenType:   "Bearer",
		Expiry:      time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	manager := newTestManagerWithOptions(t, map[string]ServerConfig{
		"posthog": {URL: httpServer.URL, Auth: AuthConfig{Type: AuthOAuth}},
	}, WithAuthStore(authStore))
	startAndWait(t, manager, "posthog", StateConnected)

	err := manager.Refresh(context.Background(), "posthog", WithRefreshOAuthForceReauth(true))
	if !errors.Is(err, ErrAuthRequired) {
		t.Fatalf("Refresh() error = %v, want ErrAuthRequired", err)
	}
	if status := manager.Status("posthog"); status.State != StateAuthRequired {
		t.Fatalf("status = %s, want %s", status.State, StateAuthRequired)
	}
	if _, ok, err := authStore.Get(mcpAuthProvider("posthog")); err != nil || ok {
		t.Fatalf("auth store credential after refresh ok=%v err=%v, want missing", ok, err)
	}
}

func TestManagerCloseSkipsStreamableHTTPDelete(t *testing.T) {
	var sawDelete bool
	server := newTestMCPServer()
	handler := mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server {
		return server
	}, nil)
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			sawDelete = true
			http.Error(w, "delete should not be called", http.StatusInternalServerError)
			return
		}
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(httpServer.Close)

	manager := newTestManager(t, map[string]ServerConfig{
		"test": {
			URL:  httpServer.URL,
			Auth: AuthConfig{Type: AuthNone},
		},
	})
	startAndWait(t, manager, "test", StateConnected)

	start := time.Now()
	if err := manager.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("Close() took %s, want under 1s", elapsed)
	}
	if sawDelete {
		t.Fatal("Close() attempted streamable HTTP DELETE")
	}
}

func TestManagerOAuthTokenCachePersistsMCPToken(t *testing.T) {
	authStore := keenauth.NewStoreAt(filepath.Join(t.TempDir(), "auth.json"))
	manager := newTestManagerWithOptions(t, map[string]ServerConfig{
		"posthog": {URL: "https://example.com/mcp", Auth: AuthConfig{Type: AuthOAuth}},
	}, WithAuthStore(authStore))
	token := &oauth2.Token{
		AccessToken:  "access",
		RefreshToken: "refresh",
		TokenType:    "Bearer",
		Expiry:       time.Now().Add(time.Hour).Truncate(time.Second),
	}
	if err := manager.saveOAuthToken(context.Background(), "posthog", token); err != nil {
		t.Fatalf("saveOAuthToken() error = %v", err)
	}
	got := manager.oauthToken("posthog", false)
	if got == nil {
		t.Fatal("oauthToken() = nil")
	}
	if got.AccessToken != token.AccessToken || got.RefreshToken != token.RefreshToken || !got.Expiry.Equal(token.Expiry) {
		t.Fatalf("oauthToken() = %#v, want %#v", got, token)
	}
	if got := manager.oauthToken("posthog", true); got != nil {
		t.Fatalf("oauthToken(forceReauth) = %#v, want nil", got)
	}
	cred, ok, err := authStore.Get(mcpAuthProvider("posthog"))
	if err != nil || !ok {
		t.Fatalf("auth store credential ok=%v err=%v", ok, err)
	}
	if cred.AccessToken != token.AccessToken || cred.RefreshToken != token.RefreshToken || !cred.ExpiresAt.Equal(token.Expiry) {
		t.Fatalf("stored credential = %#v, want token %#v", cred, token)
	}
	if err := manager.clearOAuthToken("posthog"); err != nil {
		t.Fatalf("clearOAuthToken() error = %v", err)
	}
	if got := manager.oauthToken("posthog", false); got != nil {
		t.Fatalf("oauthToken() after clear = %#v, want nil", got)
	}
	if _, ok, err := authStore.Get(mcpAuthProvider("posthog")); err != nil || ok {
		t.Fatalf("auth store credential after clear ok=%v err=%v, want missing", ok, err)
	}
}

func TestLoadOAuthTokensOnlyLoadsMCPProviders(t *testing.T) {
	authStore := keenauth.NewStoreAt(filepath.Join(t.TempDir(), "auth.json"))
	if err := authStore.Set("openai-codex", keenauth.OAuthCredential{
		Type:        "oauth",
		AccessToken: "provider-token",
		ExpiresAt:   time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	if err := authStore.Set(mcpAuthProvider("posthog"), keenauth.OAuthCredential{
		Type:        "oauth",
		AccessToken: "mcp-token",
		ExpiresAt:   time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}

	runtime := map[string]*serverRuntime{
		"posthog": {config: ServerConfig{Auth: AuthConfig{Type: AuthOAuth}}},
	}
	tokens, err := loadOAuthTokens(authStore, runtime)
	if err != nil {
		t.Fatalf("loadOAuthTokens() error = %v", err)
	}
	if len(tokens) != 1 || tokens["posthog"] == nil || tokens["posthog"].AccessToken != "mcp-token" {
		t.Fatalf("tokens = %#v, want only posthog MCP token", tokens)
	}
}

func TestManagerStdioListAndCallTool(t *testing.T) {
	if os.Getenv("KEEN_MCP_STDIO_HELPER") == "1" {
		runStdioHelperServer()
		return
	}

	manager := newTestManager(t, map[string]ServerConfig{
		"local": {
			Command: os.Args[0],
			Args:    []string{"-test.run=TestManagerStdioListAndCallTool"},
			Env:     map[string]string{"KEEN_MCP_STDIO_HELPER": "1"},
		},
	})
	startAndWait(t, manager, "local", StateConnected)

	result, err := manager.CallTool(context.Background(), "local", "echo", map[string]any{"message": "stdio"})
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if got := result.StructuredContent.(map[string]any)["message"]; got != "stdio" {
		t.Fatalf("structured message = %v, want stdio", got)
	}
}

func TestListToolsDisconnectedServer(t *testing.T) {
	manager := newTestManager(t, map[string]ServerConfig{
		"test": {URL: "https://example.com/mcp", Auth: AuthConfig{Type: AuthNone}},
	})
	if _, err := manager.ListTools(context.Background(), "test"); !errors.Is(err, ErrServerDisconnected) {
		t.Fatalf("ListTools() error = %v, want ErrServerDisconnected", err)
	}
}

const testServerInstructions = "Use fake to echo messages and test errors."

func newTestMCPServer() *mcpsdk.Server {
	server := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "fake", Version: "1.0.0"}, &mcpsdk.ServerOptions{Instructions: testServerInstructions})
	mcpsdk.AddTool(server, &mcpsdk.Tool{Name: "echo", Description: "echo a message"}, func(_ context.Context, _ *mcpsdk.CallToolRequest, input echoInput) (*mcpsdk.CallToolResult, echoOutput, error) {
		return nil, echoOutput{Message: input.Message}, nil
	})
	mcpsdk.AddTool(server, &mcpsdk.Tool{Name: "fail", Description: "return a tool error"}, func(_ context.Context, _ *mcpsdk.CallToolRequest, _ struct{}) (*mcpsdk.CallToolResult, any, error) {
		return nil, nil, fmt.Errorf("tool failed")
	})
	return server
}

func runStdioHelperServer() {
	server := newTestMCPServer()
	if err := server.Run(context.Background(), &mcpsdk.StdioTransport{}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(0)
}

func newTestManager(t *testing.T, servers map[string]ServerConfig) *Manager {
	t.Helper()
	return newTestManagerWithOptions(t, servers)
}

func restoreDefaultLogger(t *testing.T, logger *slog.Logger) {
	t.Helper()
	previous := slog.Default()
	slog.SetDefault(logger)
	t.Cleanup(func() {
		slog.SetDefault(previous)
	})
}

func newTestManagerWithOptions(t *testing.T, servers map[string]ServerConfig, opts ...Option) *Manager {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := DefaultConfigPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	data := Config{Servers: servers}
	encoded, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	defaults := []Option{
		WithAuthStore(keenauth.NewStoreAt(filepath.Join(t.TempDir(), "auth.json"))),
		WithTimeouts(2*time.Second, 2*time.Second, 2*time.Second),
		WithStandaloneSSEDisabled(true),
		WithStreamableMaxRetries(-1),
	}
	opts = append(defaults, opts...)
	manager, err := NewManager(opts...)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	t.Cleanup(func() {
		if err := manager.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})
	return manager
}

func startAndWait(t *testing.T, manager *Manager, server string, want ServerState) {
	t.Helper()
	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	waitForState(t, manager, server, want)
	_ = manager.WaitInitialScan(context.Background())
}

func waitForState(t *testing.T, manager *Manager, server string, want ServerState) ServerStatus {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		status := manager.Status(server)
		if status.State == want {
			return status
		}
		if status.State == StateDisconnected || status.State == StateAuthFailed {
			t.Fatalf("server state = %s, want %s, error: %s", status.State, want, status.LastError)
		}
		time.Sleep(10 * time.Millisecond)
	}
	status := manager.Status(server)
	t.Fatalf("server state = %s, want %s, error: %s", status.State, want, status.LastError)
	return ServerStatus{}
}

func TestManagerServersReturnsSortedStatuses(t *testing.T) {
	manager := &Manager{runtime: map[string]*serverRuntime{
		"zeta":  {status: ServerStatus{Name: "zeta", State: StateConnected}},
		"alpha": {status: ServerStatus{Name: "alpha", State: StateConfigured}},
	}}
	statuses := manager.Servers()
	if len(statuses) != 2 || statuses[0].Name != "alpha" || statuses[1].Name != "zeta" {
		t.Fatalf("Servers() = %#v", statuses)
	}
}

func TestManagerNormalizeError(t *testing.T) {
	manager := &Manager{runtime: map[string]*serverRuntime{
		"secure": {config: ServerConfig{Auth: AuthConfig{Type: AuthAPIKey, Key: "secret"}}},
	}}
	tests := []struct {
		name string
		err  error
		kind error
	}{
		{name: "deadline", err: context.DeadlineExceeded, kind: ErrTimeout},
		{name: "cancelled", err: context.Canceled, kind: ErrTimeout},
		{name: "auth required", err: ErrAuthRequired, kind: ErrAuthRequired},
		{name: "auth failed", err: errors.New("401 secret"), kind: ErrAuthFailed},
		{name: "protocol", err: errors.New("bad secret response"), kind: ErrProtocol},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := manager.normalizeError("secure", "tool", tt.err)
			if !errors.Is(err, tt.kind) {
				t.Fatalf("normalizeError() = %v, want %v", err, tt.kind)
			}
			if strings.Contains(err.Error(), "secret") {
				t.Fatalf("normalizeError leaked secret: %v", err)
			}
		})
	}
}

func TestManagerFormattingHelpers(t *testing.T) {
	if got := truncate("short", 10); got != "short" {
		t.Fatalf("truncate(short) = %q", got)
	}
	if got := truncate("long value", 4); got != "long..." {
		t.Fatalf("truncate(long) = %q", got)
	}
	if got := schemaSummary(nil); got != "" {
		t.Fatalf("schemaSummary(nil) = %q", got)
	}
	if got := schemaSummary(map[string]any{"type": "object"}); len(got) != 16 {
		t.Fatalf("schemaSummary() = %q", got)
	}
	if got := schemaJSON(make(chan int)); got != nil {
		t.Fatalf("schemaJSON(channel) = %q", got)
	}
	writer := &stderrLogger{server: "test"}
	if n, err := writer.Write([]byte(" log line \n")); err != nil || n != len(" log line \n") {
		t.Fatalf("Write() = %d, %v", n, err)
	}
	if n, err := writer.Write(nil); err != nil || n != 0 {
		t.Fatalf("Write(nil) = %d, %v", n, err)
	}
}

func TestMCPErrorFormattingAndStates(t *testing.T) {
	err := &Error{Kind: ErrProtocol, Server: "server", Tool: "tool", Err: errors.New("detail")}
	if got := err.Error(); got != "mcp protocol error: server/tool: detail" {
		t.Fatalf("Error() = %q", got)
	}
	for _, tt := range []struct {
		state ServerState
		kind  error
	}{
		{StateAuthRequired, ErrAuthRequired},
		{StateAuthFailed, ErrAuthFailed},
		{StateDisconnected, ErrServerDisconnected},
	} {
		if got := stateError("server", tt.state, "detail"); !errors.Is(got, tt.kind) {
			t.Fatalf("stateError(%s) = %v", tt.state, got)
		}
	}
}

func TestManagerOptionSetters(t *testing.T) {
	opts := defaultOptions()
	client := &http.Client{}
	store := keenauth.NewStoreAt(filepath.Join(t.TempDir(), "auth.json"))
	fetcher := func(context.Context, *mcpauth.AuthorizationArgs) (*mcpauth.AuthorizationResult, error) {
		return nil, nil
	}
	for _, option := range []Option{
		WithHTTPClient(client),
		WithHTTPClient(nil),
		WithAuthStore(store),
		WithOAuthRedirectURL("http://localhost/callback"),
		WithOAuthAuthorizationCodeFetcher(fetcher),
		WithOAuthClientName("Client"),
		WithOAuthClientName(""),
		WithOAuthClientIDMetadataDocument("https://example.invalid/metadata"),
		WithOAuthPreregisteredClient("id", "secret"),
		WithStdioTerminateTimeout(time.Second),
		WithStdioTerminateTimeout(0),
	} {
		option(&opts)
	}
	if opts.httpClient != client || opts.authStore != store || opts.oauthRedirectURL == "" || opts.oauthCodeFetcher == nil {
		t.Fatalf("options not applied: %#v", opts)
	}
	if opts.oauthClientName != "Client" || opts.oauthPreregisteredID != "id" || opts.oauthPreregisteredSecret != "secret" {
		t.Fatalf("OAuth options not applied: %#v", opts)
	}
	if opts.stdioTerminateTimeout != time.Second {
		t.Fatalf("stdio timeout = %s", opts.stdioTerminateTimeout)
	}
}
