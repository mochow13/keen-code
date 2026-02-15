# StreamChat Implementation - Todo List

## 1. Add Genkit Dependency
- [x] Add `github.com/firebase/genkit/go` to go.mod
- [x] Run `go mod tidy` to fetch dependencies

## 2. Create `internal/llm` Package Structure
- [x] Create `internal/llm/client.go` with `LLMClient` interface
- [x] Create `internal/llm/message.go` with message types
- [x] Create `internal/llm/models.go` with provider/model configuration
- [x] Create `internal/llm/genkit.go` with Genkit client implementation

## 3. Define Core Interfaces and Types
- [x] Define `LLMClient` interface with `StreamChat(messages []Message) (<-chan StreamEvent, error)` method
- [x] Define `Message` struct with Role and Content fields
- [x] Define `StreamEvent` struct with Type (chunk/done/error) and Content fields
- [x] Define `Role` type constants (System, User, Assistant)

## 4. Implement Genkit Client Wrapper
- [x] Create `GenkitClient` struct implementing `LLMClient`
- [x] Implement provider-specific initialization (Anthropic, OpenAI, Gemini)
- [x] Implement `StreamChat` method using genkit's streaming API
- [x] Handle streaming response chunks
- [x] **Test:** Write unit tests for `StreamChat` with mock genkit responses

## 5. Create LLM Client Factory
- [x] Create `NewClient(config *config.ResolvedConfig) (LLMClient, error)` factory function
- [x] Support Anthropic provider with API key and model
- [x] Support OpenAI provider with API key and model
- [x] Support Gemini provider with API key and model
- [x] **Test:** Write unit tests for factory with different provider configurations

## 6. Update REPL for Multi-line Input
- [x] Replace bufio.Scanner with bubbletea textarea
- [x] Implement Ctrl+J to insert new line in message
- [x] Implement Enter to send message to LLM
- [x] Update tips to show keybindings

## 7. Integrate LLM Client with REPL
- [ ] Add `llmClient` field to `replState` struct
- [ ] Initialize LLM client in `RunREPL` with current config
- [ ] Update `handleInput` to send non-command input to LLM
- [ ] Create method to handle LLM streaming response

## 8. Implement Streaming Response Display
- [ ] Create styled output area for LLM responses using lipgloss
- [ ] Stream response chunks to the output in real-time
- [ ] Add visual indicator while streaming (e.g., spinner or "...")
- [ ] Handle final formatting of complete response

## 9. Handle LLM Client Updates
- [ ] Create `updateLLMClient` method on `replState`
- [ ] Call `updateLLMClient` when `/model` command changes config
- [ ] Ensure old client connections are properly closed

## 10. Error Handling
- [ ] Handle API key errors with clear user message
- [ ] Handle network errors gracefully
- [ ] Handle rate limit errors with retry suggestion
- [ ] Display error messages in REPL without breaking the loop
- [ ] Log errors for debugging
- [ ] **Test:** Write unit tests for error scenarios (invalid API key, network failure, rate limits)
