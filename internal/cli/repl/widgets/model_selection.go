package widgets

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	keenauth "github.com/mochow13/keen-code/internal/auth"
	repltheme "github.com/mochow13/keen-code/internal/cli/repl/theme"
	"github.com/mochow13/keen-code/internal/config"
	"github.com/mochow13/keen-code/internal/providers"
)

func supportsBaseURL(providerID string) bool {
	return providerID != config.ProviderGoogleAI && config.RequiresAPIKey(providerID)
}

func promptsForAPIKey(providerID string) bool {
	return config.SupportsAPIKey(providerID)
}

type Step int

const (
	StepProvider Step = iota
	StepModel
	StepThinking
	StepUpdateProviderConfigs
	StepBaseURL
	StepAPIKey
	StepAPIKeyHelper
	StepOAuth
)

const maxVisibleListItems = 6

type modelSelectionCompleteMsg struct{}
type modelSelectionCancelMsg struct{}
type modelSelectionOAuthCompleteMsg struct {
	err error
}
type modelSelectionAPIKeyHelperResultMsg struct {
	apiKey      string
	err         error
	providerCfg config.ProviderConfig
}

type Model struct {
	Step                       Step
	SelectedProvider           string
	SelectedModel              string
	APIKeyInput                string
	BaseURLInput               string
	BaseURLError               string
	ProviderCursor             int
	ModelCursor                int
	ThinkingCursor             int
	ThinkingOptions            []string
	SelectedThinking           string
	UpdateProviderConfigCursor int
	updatingProviderConfig     bool
	OAuthStatus                string
	OAuthURL                   string
	ProviderList               []providers.Provider
	ModelList                  []providers.Model
	ErrorMessage               string
	oauthCancel                context.CancelFunc
	authManager                *keenauth.OAuthManager
	registry                   *providers.Registry
	globalCfg                  *config.GlobalConfig
	loader                     *config.Loader
	resolvedCfg                *config.ResolvedConfig
	onComplete                 func(provider, model, apiKey string) error
}

func New(registry *providers.Registry, globalCfg *config.GlobalConfig, loader *config.Loader, resolvedCfg *config.ResolvedConfig, onComplete func(provider, model, apiKey string) error) *Model {
	providerList := make([]providers.Provider, 0, len(registry.Providers))
	for _, provider := range registry.Providers {
		if provider.ID != config.ProviderOpenAICompatible {
			providerList = append(providerList, provider)
		}
	}
	return &Model{
		Step:         StepProvider,
		ProviderList: providerList,
		authManager:  keenauth.NewOAuthManager(nil),
		registry:     registry,
		globalCfg:    globalCfg,
		loader:       loader,
		resolvedCfg:  resolvedCfg,
		onComplete:   onComplete,
	}
}

func NewWithAuthManager(registry *providers.Registry, globalCfg *config.GlobalConfig, loader *config.Loader, resolvedCfg *config.ResolvedConfig, authManager *keenauth.OAuthManager, onComplete func(provider, model, apiKey string) error) *Model {
	m := New(registry, globalCfg, loader, resolvedCfg, onComplete)
	m.authManager = authManager
	return m
}

func (m *Model) Update(msg tea.Msg) (*Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		return m.handleKeyMsg(msg)
	case tea.PasteMsg:
		return m.handlePasteMsg(msg)
	case modelSelectionOAuthCompleteMsg:
		return m.handleOAuthComplete(msg)
	case modelSelectionAPIKeyHelperResultMsg:
		return m.handleAPIKeyHelperResult(msg)
	}
	return m, nil
}

func (m *Model) handleKeyMsg(msg tea.KeyPressMsg) (*Model, tea.Cmd) {
	switch m.Step {
	case StepProvider:
		switch msg.String() {
		case "up":
			m.ProviderCursor = (m.ProviderCursor - 1 + len(m.ProviderList)) % len(m.ProviderList)
		case "down":
			m.ProviderCursor = (m.ProviderCursor + 1) % len(m.ProviderList)
		case "enter":
			m.SelectedProvider = m.ProviderList[m.ProviderCursor].ID
			m.APIKeyInput = ""
			m.BaseURLInput = ""
			m.BaseURLError = ""
			m.updatingProviderConfig = false
			provider, _ := m.registry.GetProvider(m.SelectedProvider)
			m.ModelList = provider.Models
			m.ModelCursor = 0
			m.ErrorMessage = ""
			if config.AuthModeForProvider(m.SelectedProvider) == config.AuthModeOAuth && !m.authManager.HasCredential(m.SelectedProvider) {
				return m.startOAuth()
			}
			m.Step = StepModel
		case "esc":
			return m, func() tea.Msg { return modelSelectionCancelMsg{} }
		}

	case StepModel:
		switch msg.String() {
		case "up":
			m.ModelCursor = (m.ModelCursor - 1 + len(m.ModelList)) % len(m.ModelList)
		case "down":
			m.ModelCursor = (m.ModelCursor + 1) % len(m.ModelList)
		case "enter":
			m.SelectedModel = m.ModelList[m.ModelCursor].ID
			modelMeta, ok := m.registry.GetModel(m.SelectedProvider, m.SelectedModel)
			if ok && modelMeta.SupportsThinkingEffort() {
				m.ThinkingOptions = modelMeta.ThinkingEfforts
				m.ThinkingCursor = m.resolveThinkingCursor(m.ThinkingOptions)
				m.Step = StepThinking
			} else {
				return m.continueAfterModelSelection()
			}
		case "esc":
			return m, func() tea.Msg { return modelSelectionCancelMsg{} }
		}

	case StepThinking:
		switch msg.String() {
		case "up":
			m.ThinkingCursor = (m.ThinkingCursor - 1 + len(m.ThinkingOptions)) % len(m.ThinkingOptions)
		case "down":
			m.ThinkingCursor = (m.ThinkingCursor + 1) % len(m.ThinkingOptions)
		case "enter":
			m.SelectedThinking = m.ThinkingOptions[m.ThinkingCursor]
			return m.continueAfterModelSelection()
		case "esc":
			return m, func() tea.Msg { return modelSelectionCancelMsg{} }
		}

	case StepUpdateProviderConfigs:
		switch msg.String() {
		case "up", "down":
			m.UpdateProviderConfigCursor = 1 - m.UpdateProviderConfigCursor
		case "enter":
			if m.UpdateProviderConfigCursor == 0 {
				return m.complete()
			}
			m.updatingProviderConfig = true
			m.beginProviderConfigUpdate()
			return m, nil
		case "esc":
			return m, func() tea.Msg { return modelSelectionCancelMsg{} }
		}

	case StepBaseURL:
		switch msg.String() {
		case "esc":
			return m, func() tea.Msg { return modelSelectionCancelMsg{} }
		case "enter":
			if err := isValidBaseURL(m.BaseURLInput); err != nil {
				m.BaseURLError = err.Error()
				return m, nil
			}
			m.BaseURLError = ""
			m.Step = StepAPIKey
		case "backspace":
			if len(m.BaseURLInput) > 0 {
				m.BaseURLInput = m.BaseURLInput[:len(m.BaseURLInput)-1]
			}
			m.BaseURLError = ""
		default:
			if len(msg.Text) > 0 {
				m.BaseURLInput += msg.Text
				m.BaseURLError = ""
			}
		}

	case StepAPIKey:
		switch msg.String() {
		case "esc":
			return m, func() tea.Msg { return modelSelectionCancelMsg{} }
		case "enter":
			return m.complete()
		case "backspace":
			if len(m.APIKeyInput) > 0 {
				m.APIKeyInput = m.APIKeyInput[:len(m.APIKeyInput)-1]
			}
		default:
			if len(msg.Text) > 0 {
				m.APIKeyInput += msg.Text
			}
		}

	case StepAPIKeyHelper:
		switch msg.String() {
		case "esc":
			return m, func() tea.Msg { return modelSelectionCancelMsg{} }
		}

	case StepOAuth:
		switch msg.String() {
		case "esc":
			if m.oauthCancel != nil {
				m.oauthCancel()
			}
			return m, func() tea.Msg { return modelSelectionCancelMsg{} }
		}
	}

	return m, nil
}

func (m *Model) startOAuth() (*Model, tea.Cmd) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	session, err := m.authManager.StartLogin(ctx, m.SelectedProvider)
	if err != nil {
		cancel()
		m.ErrorMessage = err.Error()
		m.OAuthStatus = "Authentication failed."
		m.Step = StepOAuth
		return m, nil
	}

	m.Step = StepOAuth
	m.OAuthStatus = "Complete authentication in your browser."
	m.OAuthURL = session.AuthURL
	m.oauthCancel = cancel
	return m, m.waitOAuthCmd(ctx, cancel, session)
}

func (m *Model) waitOAuthCmd(ctx context.Context, cancel context.CancelFunc, session *keenauth.OAuthLoginSession) tea.Cmd {
	return func() tea.Msg {
		defer cancel()
		err := session.Wait(ctx)
		return modelSelectionOAuthCompleteMsg{err: err}
	}
}

func (m *Model) handleOAuthComplete(msg modelSelectionOAuthCompleteMsg) (*Model, tea.Cmd) {
	m.oauthCancel = nil
	if msg.err != nil {
		m.ErrorMessage = msg.err.Error()
		m.OAuthStatus = "Authentication failed."
		return m, nil
	}
	m.ErrorMessage = ""
	m.OAuthStatus = ""
	m.OAuthURL = ""
	if m.SelectedModel != "" {
		return m.complete()
	}
	provider, _ := m.registry.GetProvider(m.SelectedProvider)
	m.ModelList = provider.Models
	m.ModelCursor = 0
	m.Step = StepModel
	return m, nil
}

func (m *Model) Select(providerID, modelID string) (*Model, tea.Cmd, error) {
	modelMeta, ok := m.registry.GetModel(providerID, modelID)
	if !ok {
		return m, nil, fmt.Errorf("unknown model: %s/%s", providerID, modelID)
	}

	m.SelectedProvider = providerID
	m.SelectedModel = modelID
	m.APIKeyInput = ""
	m.BaseURLInput = ""
	m.BaseURLError = ""
	m.ErrorMessage = ""
	m.updatingProviderConfig = false

	if modelMeta.SupportsThinkingEffort() {
		m.ThinkingOptions = modelMeta.ThinkingEfforts
		m.ThinkingCursor = m.resolveThinkingCursor(m.ThinkingOptions)
		m.Step = StepThinking
		return m, nil, nil
	}

	if config.AuthModeForProvider(m.SelectedProvider) == config.AuthModeOAuth && !m.authManager.HasCredential(m.SelectedProvider) {
		updated, cmd := m.startOAuth()
		return updated, cmd, nil
	}
	updated, cmd := m.continueAfterModelSelection()
	return updated, cmd, nil
}

func (m *Model) continueAfterModelSelection() (*Model, tea.Cmd) {
	if config.AuthModeForProvider(m.SelectedProvider) == config.AuthModeOAuth && !m.authManager.HasCredential(m.SelectedProvider) {
		return m.startOAuth()
	}
	if !promptsForAPIKey(m.SelectedProvider) {
		return m.complete()
	}
	if m.getExistingAPIKey(m.SelectedProvider) != "" {
		m.UpdateProviderConfigCursor = 0
		m.Step = StepUpdateProviderConfigs
		return m, nil
	}
	m.beginProviderConfigUpdate()
	return m, nil
}

func (m *Model) beginProviderConfigUpdate() {
	if supportsBaseURL(m.SelectedProvider) {
		m.BaseURLInput = m.getExistingBaseURL(m.SelectedProvider)
		m.Step = StepBaseURL
		return
	}
	m.Step = StepAPIKey
}

func (m *Model) resolveThinkingCursor(options []string) int {
	currentEffort := ""
	if m.resolvedCfg != nil {
		currentEffort = m.resolvedCfg.ThinkingEffort
	}
	if currentEffort == "" {
		if idx := slices.Index(options, "medium"); idx >= 0 {
			return idx
		}
		return 0
	}
	if idx := slices.Index(options, currentEffort); idx >= 0 {
		return idx
	}
	if idx := slices.Index(options, "medium"); idx >= 0 {
		return idx
	}
	return 0
}

func (m *Model) handlePasteMsg(msg tea.PasteMsg) (*Model, tea.Cmd) {
	switch m.Step {
	case StepBaseURL:
		if msg.Content != "" {
			m.BaseURLInput += msg.Content
			m.BaseURLError = ""
		}
	case StepAPIKey:
		if msg.Content != "" {
			m.APIKeyInput += msg.Content
		}
	}
	return m, nil
}

func (m *Model) complete() (*Model, tea.Cmd) {
	existing, exists := m.globalCfg.GetProviderConfig(m.SelectedProvider)

	apiKey := m.APIKeyInput
	if apiKey == "" && exists {
		apiKey = existing.APIKey
	}

	storedEffort := m.SelectedThinking

	// If model doesn't support configurable effort, clear any stale value
	modelMeta, ok := m.registry.GetModel(m.SelectedProvider, m.SelectedModel)
	if !ok || !modelMeta.SupportsThinkingEffort() {
		storedEffort = ""
	}

	m.globalCfg.ActiveProvider = m.SelectedProvider
	m.globalCfg.ActiveModel = m.SelectedModel
	m.globalCfg.ThinkingEffort = storedEffort

	providerCfg := config.ProviderConfig{
		APIKey:       apiKey,
		Models:       []string{m.SelectedModel},
		BaseURL:      existing.BaseURL,
		Headers:      existing.Headers,
		APIKeyHelper: existing.APIKeyHelper,
	}
	if supportsBaseURL(m.SelectedProvider) && (m.updatingProviderConfig || !exists) {
		providerCfg.BaseURL = m.BaseURLInput
	}

	if providerCfg.APIKeyHelper != "" {
		m.Step = StepAPIKeyHelper
		return m, func() tea.Msg {
			apiKey, err := config.ResolveProviderAPIKey(m.SelectedProvider, providerCfg)
			return modelSelectionAPIKeyHelperResultMsg{
				apiKey:      apiKey,
				err:         err,
				providerCfg: providerCfg,
			}
		}
	}

	resolvedAPIKey, err := config.ResolveProviderAPIKey(m.SelectedProvider, providerCfg)
	if err != nil {
		m.ErrorMessage = err.Error()
		return m, nil
	}

	return m.finishComplete(resolvedAPIKey, providerCfg)
}

func (m *Model) finishComplete(resolvedAPIKey string, providerCfg config.ProviderConfig) (*Model, tea.Cmd) {
	storedEffort := m.SelectedThinking
	modelMeta, ok := m.registry.GetModel(m.SelectedProvider, m.SelectedModel)
	if !ok || !modelMeta.SupportsThinkingEffort() {
		storedEffort = ""
	}

	m.globalCfg.SetProviderConfig(m.SelectedProvider, providerCfg)

	if err := m.loader.Save(m.globalCfg); err != nil {
		m.ErrorMessage = fmt.Sprintf("Failed to save config: %v", err)
		return m, nil
	}

	m.resolvedCfg.Provider = m.SelectedProvider
	m.resolvedCfg.Model = m.SelectedModel
	m.resolvedCfg.APIKey = resolvedAPIKey
	m.resolvedCfg.APIKeyHelper = providerCfg.APIKeyHelper
	m.resolvedCfg.ThinkingEffort = storedEffort
	m.resolvedCfg.BaseURL = providerCfg.BaseURL
	m.resolvedCfg.AuthMode = config.AuthModeForProvider(m.SelectedProvider)
	m.resolvedCfg.Headers = providerCfg.Headers

	if err := m.onComplete(m.SelectedProvider, m.SelectedModel, resolvedAPIKey); err != nil {
		m.ErrorMessage = fmt.Sprintf("Failed to initialize LLM client: %v", err)
		return m, nil
	}

	return m, func() tea.Msg { return modelSelectionCompleteMsg{} }
}

func (m *Model) handleAPIKeyHelperResult(msg modelSelectionAPIKeyHelperResultMsg) (*Model, tea.Cmd) {
	if m.Step != StepAPIKeyHelper {
		return m, nil
	}
	if msg.err != nil {
		m.ErrorMessage = msg.err.Error()
		m.Step = StepAPIKey
		return m, nil
	}
	return m.finishComplete(msg.apiKey, msg.providerCfg)
}

func (m *Model) ViewString() string {
	switch m.Step {
	case StepProvider:
		return m.renderProviderSelection()
	case StepModel:
		return m.renderModelSelection()
	case StepThinking:
		return m.renderThinkingSelection()
	case StepUpdateProviderConfigs:
		return m.renderUpdateProviderConfigs()
	case StepBaseURL:
		return m.renderBaseURLInput()
	case StepAPIKey:
		return m.renderAPIKeyInput()
	case StepAPIKeyHelper:
		return m.renderAPIKeyHelperStatus()
	case StepOAuth:
		return m.renderOAuthStatus()
	}
	return ""
}

func (m *Model) renderProviderSelection() string {
	var view strings.Builder
	view.WriteString(repltheme.ModelSelectionTitleStyle.Render("Select a provider:"))
	view.WriteString("\n\n")
	view.WriteString(m.renderList(m.ProviderCursor, func(i int) string { return m.ProviderList[i].Name }, len(m.ProviderList)))
	view.WriteString("\n")
	view.WriteString(repltheme.ModelSelectionTextStyle.Render("[↑/↓ to navigate, Enter to select, Esc to cancel]"))
	return view.String()
}

func (m *Model) renderModelSelection() string {
	var view strings.Builder
	providerName := m.getProviderName(m.SelectedProvider)
	view.WriteString(repltheme.ModelSelectionTitleStyle.Render(fmt.Sprintf("Select a model for %s:", providerName)))
	view.WriteString("\n\n")
	view.WriteString(m.renderList(m.ModelCursor, func(i int) string { return m.ModelList[i].Name }, len(m.ModelList)))
	view.WriteString("\n")
	view.WriteString(repltheme.ModelSelectionTextStyle.Render("[↑/↓ to navigate, Enter to select, Esc to cancel]"))
	return view.String()
}

func (m *Model) renderThinkingSelection() string {
	var view strings.Builder
	view.WriteString(repltheme.ModelSelectionThinkingStyle.Render("Select thinking effort:"))
	view.WriteString("\n\n")
	view.WriteString(m.renderList(m.ThinkingCursor, func(i int) string { return m.ThinkingOptions[i] }, len(m.ThinkingOptions)))
	view.WriteString("\n")
	view.WriteString(repltheme.ModelSelectionTextStyle.Render("[↑/↓ to navigate, Enter to select, Esc to cancel]"))
	return view.String()
}

func (m *Model) renderUpdateProviderConfigs() string {
	var view strings.Builder
	view.WriteString(repltheme.ModelSelectionTitleStyle.Render("Update provider configs?"))
	view.WriteString("\n\n")
	view.WriteString(m.renderList(m.UpdateProviderConfigCursor, func(i int) string {
		return []string{"No", "Yes"}[i]
	}, 2))
	view.WriteString("\n")
	view.WriteString(repltheme.ModelSelectionTextStyle.Render("[↑/↓ to navigate, Enter to select, Esc to cancel]"))
	return view.String()
}

func (m *Model) renderBaseURLInput() string {
	var view strings.Builder
	providerName := m.getProviderName(m.SelectedProvider)
	existingURL := m.getExistingBaseURL(m.SelectedProvider)

	title := fmt.Sprintf("Base URL for %s (optional), empty means use default", providerName)
	if existingURL != "" {
		title += "\n" + repltheme.ModelSelectionTextStyle.Render("current: "+existingURL)
	}
	view.WriteString(repltheme.ModelSelectionTextStyle.Render(title))
	view.WriteString("\n\n")

	view.WriteString(repltheme.ModelSelectionTextStyle.Render(" ▶ "))
	view.WriteString(m.BaseURLInput)
	view.WriteString("\n\n")
	view.WriteString(repltheme.ModelSelectionTextStyle.Render("[Enter to confirm, Esc to cancel]"))

	if m.BaseURLError != "" {
		view.WriteString("\n")
		view.WriteString(repltheme.ErrorStyle.Render(m.BaseURLError))
	}
	return view.String()
}

func (m *Model) renderAPIKeyInput() string {
	var view strings.Builder
	providerName := m.getProviderName(m.SelectedProvider)
	existingKey := m.getExistingAPIKey(m.SelectedProvider)

	title := fmt.Sprintf("Enter API key for %s", providerName)
	hint := ""
	if existingKey != "" {
		hint = "Press enter to keep existing key"
	} else if !config.RequiresAPIKey(m.SelectedProvider) {
		title = fmt.Sprintf("API key for %s (optional)", providerName)
		hint = "Press enter to skip and use locally configured AWS credentials"
	}
	if hint != "" {
		title += "\n" + repltheme.ModelSelectionTextStyle.Render("("+hint+")")
	}
	view.WriteString(repltheme.ModelSelectionTextStyle.Render(title))
	view.WriteString("\n\n")

	maskedKey := strings.Repeat("•", len(m.APIKeyInput))
	view.WriteString(repltheme.ModelSelectionTextStyle.Render(" ▶ "))
	view.WriteString(maskedKey)
	view.WriteString("\n\n")
	view.WriteString(repltheme.ModelSelectionTextStyle.Render("[Enter to confirm, Esc to cancel]"))

	if m.ErrorMessage != "" {
		view.WriteString("\n")
		view.WriteString(repltheme.ErrorStyle.Render(m.ErrorMessage))
	}
	return view.String()
}

func (m *Model) renderAPIKeyHelperStatus() string {
	var view strings.Builder
	view.WriteString(repltheme.ModelSelectionTextStyle.Render(fmt.Sprintf("Fetching credentials for %s", m.getProviderName(m.SelectedProvider))))
	view.WriteString("\n\n")
	view.WriteString(repltheme.ModelSelectionTextStyle.Render("Fetching credentials..."))
	view.WriteString("\n\n")
	view.WriteString(repltheme.ModelSelectionTextStyle.Render("[Please wait, Esc to cancel]"))
	return view.String()
}

func (m *Model) renderOAuthStatus() string {
	var view strings.Builder
	view.WriteString(repltheme.ModelSelectionTextStyle.Render("Sign in with OpenAI"))
	view.WriteString("\n\n")
	status := m.OAuthStatus
	if status == "" {
		status = "Waiting for authentication..."
	}
	view.WriteString(repltheme.ModelSelectionTextStyle.Render(status))
	view.WriteString("\n")
	if m.OAuthURL != "" {
		view.WriteString("\n")
		view.WriteString(repltheme.ModelSelectionTextStyle.Render("URL:"))
		view.WriteString("\n")
		view.WriteString(repltheme.ModelSelectionTextStyle.Render(m.OAuthURL))
		view.WriteString("\n")
	}
	view.WriteString(repltheme.ModelSelectionTextStyle.Render("After authentication succeeds, return to Keen Code. Press Esc to cancel."))
	if m.ErrorMessage != "" {
		view.WriteString("\n\n")
		view.WriteString(repltheme.ErrorStyle.Render(m.ErrorMessage))
	}
	return view.String()
}

func (m *Model) renderList(cursor int, getName func(int) string, count int) string {
	var view strings.Builder
	start, end := visibleListRange(cursor, count, maxVisibleListItems)
	if start > 0 {
		view.WriteString(repltheme.ModelSelectionTextStyle.Render("  ↑"))
		view.WriteString("\n")
	}
	for i := start; i < end; i++ {
		if i == cursor {
			view.WriteString(repltheme.ModelSelectionCursorStyle.Render("▶ "))
			view.WriteString(repltheme.ModelSelectionSelectedTextStyle.Render(getName(i)))
			view.WriteString("\n")
			continue
		}
		view.WriteString("  ")
		view.WriteString(repltheme.ModelSelectionTextStyle.Render(getName(i)))
		view.WriteString("\n")
	}
	if end < count {
		view.WriteString(repltheme.ModelSelectionTextStyle.Render("  ↓"))
		view.WriteString("\n")
	}
	return view.String()
}

func visibleListRange(cursor, count, maxVisible int) (int, int) {
	if count <= 0 {
		return 0, 0
	}
	if maxVisible <= 0 || count <= maxVisible {
		return 0, count
	}

	half := maxVisible / 2
	start := cursor - half
	if start < 0 {
		start = 0
	}
	if start+maxVisible > count {
		start = count - maxVisible
	}
	return start, start + maxVisible
}

func (m *Model) getProviderName(providerID string) string {
	if m.registry == nil {
		return ""
	}
	if provider, ok := m.registry.GetProvider(providerID); ok {
		return provider.Name
	}
	return ""
}

func (m *Model) getExistingAPIKey(providerID string) string {
	if m.globalCfg == nil {
		return ""
	}
	if providerCfg, exists := m.globalCfg.GetProviderConfig(providerID); exists {
		return providerCfg.APIKey
	}
	return ""
}

func (m *Model) getExistingBaseURL(providerID string) string {
	if m.globalCfg == nil {
		return ""
	}
	if providerCfg, exists := m.globalCfg.GetProviderConfig(providerID); exists {
		return providerCfg.BaseURL
	}
	return ""
}

func IsComplete(msg tea.Msg) bool {
	_, ok := msg.(modelSelectionCompleteMsg)
	return ok
}

func IsCancel(msg tea.Msg) bool {
	_, ok := msg.(modelSelectionCancelMsg)
	return ok
}

func isValidBaseURL(raw string) error {
	if raw == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("URL must start with http:// or https://")
	}
	if u.Host == "" {
		return fmt.Errorf("URL must include a host")
	}
	return nil
}
