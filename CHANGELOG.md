# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.9.0] - 2026-04-27

### Added
- Input history navigation with up/down arrows in the input textarea

### Changed
- Multiline input key changed from Ctrl+Enter to Shift+Enter

### Fixed
- Skip empty assistant messages that cause Bedrock blank text errors

### Added
- Retry transient LLM stream errors with backoff for OpenAI-compatible clients

## [0.7.0] - 2026-04-25

### Added
- Startup update checker that notifies REPL users when a newer version is available

## [0.6.1] - 2026-04-25

### Fixed
- Improved assistant markdown colors on light terminals while preserving inline code color and syntax highlighting

## [0.6.0] - 2025-07-18

### Added
- Z.ai (GLM) as an OpenAI-compatible provider (#3)
- File suggestion with `@` prefix in the input textarea
- `filesearch` package with gitignore-aware file indexing and glob-escaped query matching

### Fixed
- Materialize partial message and TurnMemory on LLM error

### Changed
- Bumped Anthropic max output tokens

## [0.5.0] - 2026-04-23

### Added
- Configurable base URL per provider
- CONTRIBUTING.md for contributors
- Public-facing ROADMAP.md
- Turn-memory documentation
- Project tour, issue templates, and pull request template
- Demo GIF in README

### Changed
- Wrapped REPL diff output in a viewport
- Updated LLM configuration
- Refreshed the demo GIF rendering with Monaspace Argon NF

## [0.4.1] - 2026-04-22

### Fixed
- Retain tool memory on stream interrupt

## [0.4.0] - 2026-04-22

### Added
- Provider-backed context status replacing the local word-count heuristic
- Token usage events emitted from all provider clients (OpenAI Responses, Anthropic, Genkit/Google AI, DeepSeek/Moonshot)
- Cache-aware token accounting for Anthropic (includes cache creation and read tokens)
- Anthropic adaptive effort display in the status bar
- `N/A` display when context window is unknown instead of a misleading percentage

### Changed
- Context status now reports actual provider-counted token usage against the model context window
- Compaction suggestions are grounded in real tokenization rather than local estimates
- `/clear` and `/new` reset context metrics for new sessions
- Updated provider registry with new models and context windows

### Removed
- Local word-count token estimation helpers (`estimateTokensFromWordCount`, `countWords`, `estimateToolDefinitionTokens`, `buildConversationForEstimation`)

## [0.3.0] - 2026-04-21

### Added
- Configurable thinking effort selection in model setup and via the `/thinking` runtime command
- Direct Anthropic SDK client with expanded Anthropic streaming and tool-loop test coverage
- Refactored REPL helper packages for app state, output, permissions, theme, tooling, widgets, and streaming

### Changed
- Thread thinking effort configuration through OpenAI Responses, Anthropic, and Genkit clients
- Added Anthropic prompt caching support in the REPL streaming path
- Refreshed phase-5 design notes and removed stale scratch artifacts

## [0.2.3] - 2026-04-19

### Added
- Structured `TurnMemory` system to replace in-band XML tags for tracking durable tool outcomes
- `turnMemoryAccumulator` in REPL to automatically capture file changes and failed bash commands

### Changed
- Refactored LLM provider interface to deterministically append tool memory metadata
- Simplified system and compaction prompts by removing manual memory tag instructions
- Improved session persistence to support structured tool outcomes

### Removed
- Legacy `<keen_memory>` tag parsing and stripping logic

## [0.2.2] - 2026-04-17

### Added
- Hidden `keen_memory` blocks to preserve durable tool outcomes across turns without showing them in the REPL transcript

### Changed
- Session picker now constrains its visible list to the current viewport height

### Fixed
- Only extract trailing dedicated `keen_memory` blocks for logging and compaction-aware handling

## [0.2.1] - 2026-04-16

### Changed
- Simplified REPL context status display and metadata emphasis

## [0.2.0] - 2026-04-16

### Added
- Conversation session management with transcript persistence
- `/sessions` command to list recent sessions with metadata
- `/resume` command with interactive picker to restore conversations
- `/compact` command to summarize conversation history via LLM
- Event-sourced storage (session_started, user_message, assistant_turn, compaction_applied)
- Store tool outputs, bash results, and file diffs in transcript for full replay

## [0.1.7] - 2026-03-24

### Added
- REPL context status indicator with progress bar and percentage based on model context window
- Slash command autosuggestion dropdown for `/help`, `/model`, and `/exit`

### Changed
- Consolidated REPL styling for context status and suggestion UI

## [0.1.6] - 2026-03-22

### Changed
- Improved spinner UX with smoother feedback during LLM streaming
- Refined tool descriptions for better LLM tool selection
- Improved Genkit streaming reliability

## [0.1.5] - 2026-03-22

### Added
- Install script for easier local setup
- npm wrapper package documentation

## [0.1.4] - 2026-03-22

### Changed
- Switched npm publishing to trusted publishing (removes need for legacy token)

## [0.1.3] - 2026-03-22

### Fixed
- Release pipeline corrections from v0.1.2

## [0.1.2] - 2026-03-22

### Fixed
- Improved release flow and startup behavior

## [0.1.1] - 2026-03-22

### Fixed
- npm wrapper publish and install flow

## [0.1.0] - 2026-03-22

### Added
- Interactive REPL powered by Bubble Tea with streaming LLM responses
- Multi-turn tool calling with Genkit integration
- `read_file` tool with interactive permission system
- `write_file` tool with inline diff rendering
- `edit_file` tool with inline diff rendering
- `bash` tool with permission gating
- `glob` tool for file pattern searching
- `grep` tool for content search
- File guard with `.gitignore` awareness and permission levels (granted/pending/denied)
- Inline permission card UI (replaces full-screen modal)
- Dynamic system prompt generation with project context
- OpenAI-compatible client supporting DeepSeek (including reasoning/chain-of-thought)
- MoonshotAI provider via OpenAI-compatible client
- Dedicated OpenAI Responses API client
- GoReleaser config for cross-platform binary distribution
- npm wrapper package for installation via `npm install -g keen-code`

[Unreleased]: https://github.com/mochow13/keen-code/compare/v0.9.0...HEAD
[0.9.0]: https://github.com/mochow13/keen-code/compare/v0.8.0...v0.9.0
[0.7.0]: https://github.com/mochow13/keen-code/compare/v0.6.1...v0.7.0
[0.6.1]: https://github.com/mochow13/keen-code/compare/v0.6.0...v0.6.1
[0.6.0]: https://github.com/mochow13/keen-code/compare/v0.5.0...v0.6.0
[0.5.0]: https://github.com/mochow13/keen-code/compare/v0.4.1...v0.5.0
[0.4.1]: https://github.com/mochow13/keen-code/compare/v0.4.0...v0.4.1
[0.4.0]: https://github.com/mochow13/keen-code/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/mochow13/keen-code/compare/v0.2.3...v0.3.0
[0.2.3]: https://github.com/mochow13/keen-code/compare/v0.2.2...v0.2.3
[0.2.2]: https://github.com/mochow13/keen-code/compare/v0.2.1...v0.2.2
[0.2.1]: https://github.com/mochow13/keen-code/compare/v0.2.0...v0.2.1
[0.2.0]: https://github.com/mochow13/keen-code/compare/v0.1.7...v0.2.0
[0.1.7]: https://github.com/mochow13/keen-code/compare/v0.1.6...v0.1.7
[0.1.6]: https://github.com/mochow13/keen-code/compare/v0.1.5...v0.1.6
[0.1.5]: https://github.com/mochow13/keen-code/compare/v0.1.4...v0.1.5
[0.1.4]: https://github.com/mochow13/keen-code/compare/v0.1.3...v0.1.4
[0.1.3]: https://github.com/mochow13/keen-code/compare/v0.1.2...v0.1.3
[0.1.2]: https://github.com/mochow13/keen-code/compare/v0.1.1...v0.1.2
[0.1.1]: https://github.com/mochow13/keen-code/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/mochow13/keen-code/releases/tag/v0.1.0
