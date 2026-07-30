User prompt: Continue the OSS PR sprint by implementing a focused fix for mochow13/keen-code issue #76, "Render Markdown links without duplicate destinations".

Requirements from the issue:
- Relative repository source references and anchors should display their label only.
- External http/https Markdown links should render the label as an OSC 8 terminal hyperlink without printing a duplicate destination.
- Bare URLs should retain existing behavior.
- Add focused tests for relative source references and external URLs.

Repository instructions followed:
- Minimal comments only when strictly necessary.
- Run gofmt on modified Go files.
- Run go mod tidy after the change.
- Run go test -race ./... after finalising the change.
- Include interaction files under .ai-interactions/tasks/.
