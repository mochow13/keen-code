# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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

[Unreleased]: https://github.com/mochow13/keen-code/compare/v0.1.7...HEAD
[0.1.7]: https://github.com/mochow13/keen-code/compare/v0.1.6...v0.1.7
[0.1.6]: https://github.com/mochow13/keen-code/compare/v0.1.5...v0.1.6
[0.1.5]: https://github.com/mochow13/keen-code/compare/v0.1.4...v0.1.5
[0.1.4]: https://github.com/mochow13/keen-code/compare/v0.1.3...v0.1.4
[0.1.3]: https://github.com/mochow13/keen-code/compare/v0.1.2...v0.1.3
[0.1.2]: https://github.com/mochow13/keen-code/compare/v0.1.1...v0.1.2
[0.1.1]: https://github.com/mochow13/keen-code/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/mochow13/keen-code/releases/tag/v0.1.0
