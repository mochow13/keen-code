package markdown

import (
	"strings"
	"testing"
)

func TestMakeURLsClickableWrapsBareURL(t *testing.T) {
	got := makeURLsClickable("see https://example.com for details")
	want := "see " + osc8Open + "https://example.com" + osc8ST + "\x1b[4mhttps://example.com\x1b[24m" + osc8Close + " for details"
	if got != want {
		t.Fatalf("makeURLsClickable() = %q, want %q", got, want)
	}
}

func TestMakeURLsClickablePreservesTrailingPunctuation(t *testing.T) {
	got := makeURLsClickable("visit https://example.com.")
	want := "visit " + osc8Open + "https://example.com" + osc8ST + "\x1b[4mhttps://example.com\x1b[24m" + osc8Close + "."
	if got != want {
		t.Fatalf("makeURLsClickable() = %q, want %q", got, want)
	}
}

func TestMakeURLsClickableNoURL(t *testing.T) {
	in := "no links here"
	if got := makeURLsClickable(in); got != in {
		t.Fatalf("makeURLsClickable() = %q, want %q", got, in)
	}
}

func TestMakeURLsClickableSkipsURLInsideEscape(t *testing.T) {
	// An OSC 8 sequence that already contains a URL must not be re-wrapped.
	in := osc8Open + "https://example.com" + osc8ST + "label" + osc8Close
	got := makeURLsClickable(in)
	if got != in {
		t.Fatalf("expected URL inside existing hyperlink to be untouched, got %q", got)
	}
}

func TestMakeURLsClickableUnderlinesBareURL(t *testing.T) {
	got := makeURLsClickable("https://example.com")
	if !strings.Contains(got, "\x1b[4mhttps://example.com\x1b[24m") {
		t.Fatalf("expected bare URL to be underlined, got %q", got)
	}
}

func TestMakeURLsClickableDoesNotDoubleUnderline(t *testing.T) {
	// Underline already active before the URL: do not add another underline.
	got := makeURLsClickable("\x1b[4mhttps://example.com\x1b[0m")
	want := "\x1b[4m" + osc8Open + "https://example.com" + osc8ST + "https://example.com" + osc8Close + "\x1b[0m"
	if got != want {
		t.Fatalf("makeURLsClickable() = %q, want %q", got, want)
	}
}

func TestMakeURLsClickableHandlesStyledText(t *testing.T) {
	in := "\x1b[34mhttps://example.com\x1b[0m"
	got := makeURLsClickable(in)
	if !strings.Contains(got, osc8Open+"https://example.com"+osc8ST) {
		t.Fatalf("expected styled URL to be wrapped, got %q", got)
	}
	if !strings.Contains(got, "\x1b[34m") || !strings.Contains(got, "\x1b[0m") {
		t.Fatalf("expected styling escapes to be preserved, got %q", got)
	}
}

func TestRendererRenderLinkIsClickable(t *testing.T) {
	renderer, err := New(80)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rendered := renderer.Render("Read https://example.com/docs now.")
	if !strings.Contains(rendered, osc8Open) {
		t.Fatalf("expected OSC 8 hyperlink escape in %q", rendered)
	}
}

func TestRendererRenderRelativeMarkdownLinkShowsLabelOnly(t *testing.T) {
	renderer, err := New(80)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rendered := stripANSI(renderer.Render("[internal/cli/repl/repl.go:413](/internal/cli/repl/repl.go#L413)"))
	if !strings.Contains(rendered, "internal/cli/repl/repl.go:413") {
		t.Fatalf("expected link label in %q", rendered)
	}
	if strings.Contains(rendered, "/internal/cli/repl/repl.go#L413") {
		t.Fatalf("expected relative destination to be hidden, got %q", rendered)
	}
}

func TestRendererRenderExternalMarkdownLinkUsesOSC8Label(t *testing.T) {
	renderer, err := New(80)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rendered := renderer.Render("[docs](https://example.com/docs)")
	if !strings.Contains(rendered, osc8Open+"https://example.com/docs"+osc8ST) {
		t.Fatalf("expected markdown link destination as OSC 8 target in %q", rendered)
	}
	if strings.Contains(stripEscapes(rendered), "https://example.com/docs") {
		t.Fatalf("expected external destination to be hidden from visible text, got %q", rendered)
	}
}

func stripEscapes(value string) string {
	return ansiEscape.ReplaceAllString(stripANSI(value), "")
}
