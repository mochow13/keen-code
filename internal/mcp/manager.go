package mcp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"

	mcpauth "github.com/modelcontextprotocol/go-sdk/auth"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/oauth2"
)

type Manager struct {
	opts managerOptions

	runtime     map[string]*serverRuntime
	oauthTokens map[string]*oauth2.Token
	started     bool
	closing     bool
	ctx         context.Context
	cancel      context.CancelFunc
	clientID    *mcpsdk.Implementation
	initialDone chan struct{}
}

type serverRuntime struct {
	mu          sync.RWMutex
	config      ServerConfig
	status      ServerStatus
	session     *mcpsdk.ClientSession
	tools       []Tool
	toolsByName map[string]Tool
}

func NewManager(opts ...Option) (*Manager, error) {
	options := defaultOptions()
	for _, opt := range opts {
		opt(&options)
	}

	cfg, err := LoadConfigWithPath(options.configPath)
	if err != nil {
		return nil, err
	}

	m := &Manager{
		opts:    options,
		runtime: make(map[string]*serverRuntime, len(cfg.Servers)),
		clientID: &mcpsdk.Implementation{
			Name:    "keen",
			Title:   "Keen Code",
			Version: "0.0.0",
		},
	}
	for name, server := range cfg.Servers {
		server = normalizedServerConfig(server)
		m.runtime[name] = &serverRuntime{
			config: server,
			status: ServerStatus{
				Name:         name,
				Transport:    inferredTransport(server),
				AuthType:     server.Auth.withDefaults().Type,
				State:        StateConfigured,
				Endpoint:     server.URL,
				StdioCommand: server.Command,
			},
			toolsByName: map[string]Tool{},
		}
	}
	tokens, err := loadOAuthTokens(options.authStore, m.runtime)
	if err != nil {
		return nil, err
	}
	m.oauthTokens = tokens
	return m, nil
}

func normalizedServerConfig(server ServerConfig) ServerConfig {
	server.Auth = server.Auth.withDefaults()
	if server.Env != nil {
		server.Env = cloneMapString(server.Env)
	}
	server.Args = append([]string(nil), server.Args...)
	return server
}

func inferredTransport(server ServerConfig) string {
	if server.Command != "" {
		return TransportStdio
	}
	return TransportStreamableHTTP
}

func (m *Manager) Start(ctx context.Context) error {
	if m.started {
		return ErrAlreadyStarted
	}
	if ctx == nil {
		ctx = context.Background()
	}
	m.started = true
	m.ctx, m.cancel = context.WithCancel(ctx)
	m.initialDone = make(chan struct{})

	servers := make([]string, 0, len(m.runtime))
	for name := range m.runtime {
		servers = append(servers, name)
	}
	sort.Strings(servers)

	var wg sync.WaitGroup
	wg.Add(len(servers))
	for _, name := range servers {
		go func() {
			defer wg.Done()
			m.discoverServer(m.ctx, name, m.opts)
		}()
	}
	go func() {
		wg.Wait()
		close(m.initialDone)
	}()
	return nil
}

func (m *Manager) Close() error {
	if m.closing {
		return nil
	}
	m.closing = true
	if m.cancel != nil {
		m.cancel()
	}

	var err error
	for name, rt := range m.runtime {
		rt.mu.RLock()
		session := rt.session
		rt.mu.RUnlock()
		if session == nil {
			continue
		}
		if inferredTransport(rt.config) == TransportStreamableHTTP {
			slog.Default().Debug("mcp streamable http close skipped", "server", name)
			continue
		}
		if closeErr := closeSession(session); closeErr != nil {
			err = errors.Join(err, closeErr)
		}
	}
	return err
}

func (m *Manager) Servers() []ServerStatus {
	statuses := make([]ServerStatus, 0, len(m.runtime))
	for _, rt := range m.runtime {
		rt.mu.RLock()
		statuses = append(statuses, rt.status)
		rt.mu.RUnlock()
	}
	sort.Slice(statuses, func(i, j int) bool {
		return statuses[i].Name < statuses[j].Name
	})
	return statuses
}

func (m *Manager) WaitInitialScan(ctx context.Context) error {
	if !m.started || m.initialDone == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-m.initialDone:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m *Manager) Status(server string) ServerStatus {
	rt := m.runtime[server]
	if rt == nil {
		return ServerStatus{Name: server, State: StateDisconnected, LastError: ErrServerNotConfigured.Error()}
	}
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	return rt.status
}

func (m *Manager) ListTools(ctx context.Context, server string) ([]Tool, error) {
	rt := m.runtime[server]
	if rt == nil {
		return nil, newError(ErrServerNotConfigured, server, "", nil)
	}
	rt.mu.RLock()
	status := rt.status
	tools := copyTools(rt.tools)
	rt.mu.RUnlock()

	if status.State != StateConnected {
		return nil, stateError(server, status.State, status.LastError)
	}
	return tools, nil
}

func (m *Manager) CallTool(ctx context.Context, server, tool string, arguments map[string]any) (*ToolResult, error) {
	rt := m.runtime[server]
	if rt == nil {
		return nil, newError(ErrServerNotConfigured, server, tool, nil)
	}
	rt.mu.RLock()
	status := rt.status
	session := rt.session
	_, ok := rt.toolsByName[tool]
	rt.mu.RUnlock()

	if status.State != StateConnected || session == nil {
		return nil, stateError(server, status.State, status.LastError)
	}
	if !ok {
		return nil, newError(ErrToolNotFound, server, tool, nil)
	}

	callCtx, cancel := context.WithTimeout(ctx, m.opts.callToolTimeout)
	defer cancel()
	res, err := session.CallTool(callCtx, &mcpsdk.CallToolParams{
		Name:      tool,
		Arguments: arguments,
	})
	if err != nil {
		return nil, m.normalizeError(server, tool, err)
	}
	result := convertToolResult(res)
	if result.IsError {
		return result, newError(ErrRemoteTool, server, tool, nil)
	}
	return result, nil
}

func (m *Manager) Refresh(ctx context.Context, server string, opts ...RefreshOption) error {
	if ctx == nil {
		ctx = context.Background()
	}
	refreshOpts := m.opts
	for _, opt := range opts {
		opt(&refreshOpts)
	}

	rt := m.runtime[server]
	if rt == nil {
		return newError(ErrServerNotConfigured, server, "", nil)
	}
	rt.mu.Lock()
	oldSession := rt.session
	rt.session = nil
	rt.tools = nil
	rt.toolsByName = map[string]Tool{}
	rt.status.ToolCount = 0
	if oldSession != nil {
		rt.status.State = StateConnecting
	}
	rt.mu.Unlock()

	if oldSession != nil {
		_ = closeSession(oldSession)
	}
	m.discoverServer(ctx, server, refreshOpts)
	status := m.Status(server)
	if status.State != StateConnected {
		return stateError(server, status.State, status.LastError)
	}
	return nil
}

func closeSession(session *mcpsdk.ClientSession) error {
	if session == nil {
		return nil
	}
	return session.Close()
}

func (m *Manager) discoverServer(ctx context.Context, name string, opts managerOptions) {
	m.setState(name, StateConnecting, "")

	rt := m.runtime[name]
	if rt == nil {
		return
	}

	connectCtx, cancel := context.WithTimeout(ctx, opts.connectTimeout)
	defer cancel()

	transport, err := m.transportFor(name, rt.config, opts)
	if err != nil {
		m.setFailure(name, err)
		return
	}

	client := mcpsdk.NewClient(m.clientID, &mcpsdk.ClientOptions{
		Logger: slog.Default(),
		ToolListChangedHandler: func(_ context.Context, _ *mcpsdk.ToolListChangedRequest) {
			slog.Default().Debug("mcp tools list changed", "server", name)
		},
		LoggingMessageHandler: func(_ context.Context, req *mcpsdk.LoggingMessageRequest) {
			slog.Default().Debug("mcp server log", "server", name, "level", req.Params.Level, "logger", req.Params.Logger)
		},
	})
	session, err := client.Connect(connectCtx, transport, nil)
	if err != nil {
		m.setFailure(name, err)
		return
	}

	tools, err := m.fetchTools(ctx, name, session, opts)
	if err != nil {
		_ = closeSession(session)
		m.setFailure(name, err)
		return
	}

	now := time.Now()
	toolsByName := make(map[string]Tool, len(tools))
	for _, tool := range tools {
		toolsByName[tool.Name] = tool
	}

	rt.mu.Lock()
	if m.closing {
		rt.mu.Unlock()
		_ = closeSession(session)
		return
	}
	status := rt.status
	status.State = StateConnected
	status.LastError = ""
	status.LastConnectedAt = now
	status.LastToolRefreshAt = now
	status.ToolCount = len(tools)
	if init := session.InitializeResult(); init != nil {
		status.NegotiatedProtocol = init.ProtocolVersion
		status.Description = strings.TrimSpace(init.Instructions)
		if init.ServerInfo != nil {
			status.NegotiatedServerName = init.ServerInfo.Name
			status.NegotiatedServerVersion = init.ServerInfo.Version
		}
	}
	rt.session = session
	rt.tools = tools
	rt.toolsByName = toolsByName
	rt.status = status
	rt.mu.Unlock()
	slog.Default().Debug("mcp server connected", "server", name, "transport", status.Transport, "auth", status.AuthType, "tools", status.ToolCount)

	go func() {
		err := session.Wait()
		m.handleSessionClosed(name, err)
	}()
}

func (m *Manager) transportFor(name string, server ServerConfig, opts managerOptions) (mcpsdk.Transport, error) {
	if server.Command != "" {
		cmd := exec.Command(server.Command, server.Args...)
		cmd.Env = mergeEnv(os.Environ(), server.Env)
		cmd.Stderr = &stderrLogger{server: name}
		return &mcpsdk.CommandTransport{Command: cmd, TerminateDuration: opts.stdioTerminateTimeout}, nil
	}

	client := opts.httpClient
	var oauthHandler mcpauth.OAuthHandler
	if server.Auth.Type == AuthAPIKey {
		client = newAPIKeyClient(client, server.Auth)
	}
	if server.Auth.Type == AuthOAuth {
		if opts.oauthForceReauth {
			if err := m.clearOAuthToken(name); err != nil {
				return nil, err
			}
		}
		handler, err := newOAuthHandler(name, server, opts, m.oauthToken(name, opts.oauthForceReauth), m.saveOAuthToken)
		if err != nil {
			return nil, err
		}
		oauthHandler = handler
	}
	transport := &mcpsdk.StreamableClientTransport{
		Endpoint:             server.URL,
		HTTPClient:           client,
		MaxRetries:           opts.streamableMaxRetries,
		DisableStandaloneSSE: opts.disableStandaloneSSE,
	}
	if oauthHandler != nil {
		transport.OAuthHandler = oauthHandler
	}
	return transport, nil
}

func (m *Manager) fetchTools(ctx context.Context, server string, session *mcpsdk.ClientSession, opts managerOptions) ([]Tool, error) {
	listCtx, cancel := context.WithTimeout(ctx, opts.listToolsTimeout)
	defer cancel()

	var tools []Tool
	cursor := ""
	for {
		res, err := session.ListTools(listCtx, &mcpsdk.ListToolsParams{Cursor: cursor})
		if err != nil {
			return nil, m.normalizeError(server, "", err)
		}
		for _, tool := range res.Tools {
			converted := convertTool(tool)
			tools = append(tools, converted)
		}
		if res.NextCursor == "" {
			break
		}
		cursor = res.NextCursor
	}
	return tools, nil
}

func (m *Manager) setState(name string, state ServerState, lastErr string) {
	rt := m.runtime[name]
	if rt == nil {
		return
	}
	rt.mu.Lock()
	rt.status.State = state
	rt.status.LastError = lastErr
	rt.mu.Unlock()
}

func (m *Manager) setFailure(name string, err error) {
	state := StateDisconnected
	if errors.Is(err, ErrAuthRequired) {
		state = StateAuthRequired
	} else if isAuthError(err) {
		state = StateAuthFailed
	}
	redacted := m.redactError(name, err)
	m.setState(name, state, redacted)
	slog.Default().Debug("mcp server unavailable", "server", name, "state", state, "error", redacted)
}

func (m *Manager) handleSessionClosed(name string, err error) {
	if m.closing {
		return
	}
	rt := m.runtime[name]
	if rt == nil {
		return
	}
	rt.mu.Lock()
	if rt.status.State != StateConnected {
		rt.mu.Unlock()
		return
	}
	rt.status.State = StateDisconnected
	rt.status.LastError = m.redactErrorLocked(name, err)
	rt.session = nil
	lastErr := rt.status.LastError
	rt.mu.Unlock()
	slog.Default().Debug("mcp server disconnected", "server", name, "error", lastErr)
}

func (m *Manager) normalizeError(server, tool string, err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return newError(ErrTimeout, server, tool, err)
	}
	if errors.Is(err, context.Canceled) {
		return newError(ErrTimeout, server, tool, err)
	}
	if errors.Is(err, ErrAuthRequired) {
		return newError(ErrAuthRequired, server, tool, err)
	}
	if isAuthError(err) {
		return newError(ErrAuthFailed, server, tool, errors.New(m.redactError(server, err)))
	}
	return newError(ErrProtocol, server, tool, errors.New(m.redactError(server, err)))
}

func (m *Manager) redactError(server string, err error) string {
	return m.redactErrorLocked(server, err)
}

func (m *Manager) redactErrorLocked(server string, err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	rt := m.runtime[server]
	if rt != nil && rt.config.Auth.Type == AuthAPIKey && rt.config.Auth.Key != "" {
		msg = strings.ReplaceAll(msg, rt.config.Auth.Key, "[redacted]")
	}
	return msg
}

func isAuthError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unauthorized") ||
		strings.Contains(msg, "forbidden") ||
		strings.Contains(msg, "401") ||
		strings.Contains(msg, "403") ||
		strings.Contains(msg, "oauth")
}

func convertTool(tool *mcpsdk.Tool) Tool {
	if tool == nil {
		return Tool{}
	}
	return Tool{
		Name:         tool.Name,
		Title:        tool.Title,
		Description:  tool.Description,
		InputSchema:  tool.InputSchema,
		OutputSchema: tool.OutputSchema,
	}
}

func convertToolResult(res *mcpsdk.CallToolResult) *ToolResult {
	if res == nil {
		return &ToolResult{}
	}
	return &ToolResult{
		Content:           res.Content,
		StructuredContent: res.StructuredContent,
		IsError:           res.IsError,
		Meta:              map[string]any(res.Meta),
	}
}

func mergeEnv(base []string, extra map[string]string) []string {
	env := append([]string(nil), base...)
	for key, value := range extra {
		prefix := key + "="
		replaced := false
		for i, existing := range env {
			if strings.HasPrefix(existing, prefix) {
				env[i] = prefix + value
				replaced = true
				break
			}
		}
		if !replaced {
			env = append(env, prefix+value)
		}
	}
	return env
}

type stderrLogger struct {
	server string
}

func (w *stderrLogger) Write(p []byte) (int, error) {
	msg := strings.TrimSpace(string(p))
	if msg != "" {
		slog.Default().Debug("mcp stdio stderr", "server", w.server, "message", truncate(msg, 500))
	}
	return len(p), nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func schemaSummary(schema any) string {
	data := schemaJSON(schema)
	if len(data) == 0 {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:8])
}

var _ io.Writer = (*stderrLogger)(nil)
