# REPL Refactoring Plan

## Current State

The `internal/cli/repl.go` file has grown to ~450 lines and is becoming difficult to maintain. It contains:

- Model struct and initialization
- State management (streaming, spinner, messages)
- View rendering logic
- Message handling (commands, LLM interactions)
- Helper functions

## Refactoring Goals

1. **Improve maintainability** - Smaller, focused files
2. **Improve testability** - Isolated components can be unit tested
3. **Improve readability** - Clear separation of concerns

---

## Proposed Changes

### 1. Extract Streaming Logic to `internal/cli/streaming.go`

The streaming logic is the most complex and self-contained part. Extract it into a dedicated handler.

```go
type StreamHandler struct {
    eventCh     <-chan llm.StreamEvent
    isActive    bool
    response    string
    loadingText string
}

func (sh *StreamHandler) Start(ctx context.Context, client llm.LLMClient, messages []llm.Message) tea.Cmd
func (sh *StreamHandler) HandleChunk(chunk string) tea.Cmd
func (sh *StreamHandler) HandleDone() ([]string, error)
func (sh *StreamHandler) HandleError(err error) string
func (sh *StreamHandler) View(width int) string
```

**Benefits:**
- Isolates the complex goroutine/channel handling
- Makes streaming logic testable independently
- Simplifies the main Update() switch statement

---

### 2. Split View Components to `internal/cli/views.go`

Extract view rendering logic into focused functions.

```go
func (m replModel) renderOutput() string
func (m replModel) renderStreaming() string
func (m replModel) renderInput() string
func (m replModel) renderSpinner() string
```

**Benefits:**
- View() method becomes a simple composition of render functions
- Each component can be tested individually
- Easier to modify UI without touching business logic

---

### 3. Extract Message Handlers to `internal/cli/handlers.go`

Move the Update() switch cases into dedicated handler functions.

```go
func (m replModel) handleCommand(input string) (replModel, tea.Cmd)
func (m replModel) handleLLMStart(input string) (replModel, tea.Cmd)
func (m replModel) handleLLMChunk(chunk string) (replModel, tea.Cmd)
func (m replModel) handleLLMDone() (replModel, tea.Cmd)
func (m replModel) handleLLMError(err error) (replModel, tea.Cmd)
func (m replModel) handleKeyMsg(msg tea.KeyMsg) (replModel, tea.Cmd)
```

**Benefits:**
- Reduces the giant Update() switch statement
- Each handler has a single responsibility
- Easier to add new commands/handlers

---

### 4. Move Model Selection to `internal/cli/modelselection/`

The model selection UI is already partially extracted. Complete the separation.

**New structure:**
```
internal/cli/modelselection/
├── model.go      # Model struct, Update, View
├── styles.go     # Selection-specific styles
├── commands.go   # Selection commands
└── keys.go       # Key bindings
```

**Benefits:**
- Model selection is a distinct feature
- Can be reused or tested independently
- Cleaner main repl.go file

---

### 5. Create Output Builder `internal/cli/output.go`

Encapsulate output line management with a builder pattern.

```go
type OutputBuilder struct {
    lines []string
    width int
}

func (ob *OutputBuilder) AddUserInput(input string)
func (ob *OutputBuilder) AddAssistantResponse(response string, style lipgloss.Style)
func (ob *OutputBuilder) AddError(err string)
func (ob *OutputBuilder) AddEmptyLine()
func (ob *OutputBuilder) Strings() []string
```

**Benefits:**
- Centralizes text wrapping and styling logic
- Eliminates repetitive string concatenation
- Makes output formatting consistent

---

### 6. Separate State from Model `internal/cli/state.go`

Extract the application state from the UI model.

```go
type AppState struct {
    messages  []llm.Message
    cfg       *config.ResolvedConfig
    llmClient llm.LLMClient
    // ... other state fields
}

func (s *AppState) AddMessage(role llm.Role, content string)
func (s *AppState) ClearMessages()
func (s *AppState) GetMessages() []llm.Message
```

**Benefits:**
- Clear separation between UI state and application state
- State can be persisted/loaded independently
- Easier to implement features like conversation history

---

## Recommended Implementation Order

1. **Phase 1: Streaming Handler (#1)**
   - Most impactful - reduces complexity significantly
   - Self-contained - low risk of breaking changes

2. **Phase 2: Message Handlers (#3)**
   - Cleans up the Update() method
   - Depends on streaming handler being done first

3. **Phase 3: Output Builder (#5)**
   - Refactors existing code patterns
   - Can be done independently

4. **Phase 4: View Components (#2)**
   - Organizes the View() method
   - Can be done independently

5. **Phase 5: Model Selection Package (#4)**
   - Move existing code to new package
   - Update imports in main file

6. **Phase 6: State Separation (#6)**
   - Largest refactoring - save for last
   - Requires understanding of all state interactions

---

## Expected Outcome

After refactoring, `internal/cli/repl.go` should be ~150-200 lines and contain only:

- Model struct definition
- Update() method (delegating to handlers)
- View() method (composing view functions)
- Init() method
- High-level command handlers (delegating to specific handlers)

Each new file should be under 150 lines and focused on a single responsibility.
