Plan and implementation summary:

1. Reproduced the issue with failing tests in internal/cli/repl/markdown/hyperlink_test.go:
   - A relative Markdown link like [internal/cli/repl/repl.go:413](/internal/cli/repl/repl.go#L413) should not show the destination.
   - An external Markdown link like [docs](https://example.com/docs) should keep the URL as the OSC 8 target while hiding it from visible text.

2. Added a small Markdown link rewrite layer in internal/cli/repl/markdown/links.go:
   - Relative destinations and anchors are rewritten to label-only Markdown before rendering.
   - External http/https Markdown links are rewritten to label-only Markdown and recorded.
   - After Glamour renders the label, the rendered label is wrapped once with OSC 8 escapes.

3. Preserved existing bare URL behavior by leaving bare URLs untouched and continuing to run makeURLsClickable after Markdown link processing.

4. Verified with targeted markdown tests, broader REPL tests, gofmt, go mod tidy, and the repository-required race test command.
