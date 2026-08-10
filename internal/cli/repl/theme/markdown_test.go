package theme

import "testing"

func TestMarkdownStyleConfig(t *testing.T) {
	cfg := MarkdownStyleConfig(80)
	if cfg.Document.Margin == nil || *cfg.Document.Margin != markdownMargin {
		t.Fatalf("unexpected document margin %#v", cfg.Document.Margin)
	}
	if cfg.HorizontalRule.Format == "" || cfg.CodeBlock.Chroma == nil {
		t.Fatal("expected horizontal rule and chroma configuration")
	}
}

func TestMarkdownContentWidth(t *testing.T) {
	if got := markdownContentWidth(80); got != 76 {
		t.Fatalf("markdownContentWidth(80) = %d, want 76", got)
	}
	if got := markdownContentWidth(2); got != 1 {
		t.Fatalf("markdownContentWidth(2) = %d, want 1", got)
	}
}

func TestPointerHelpers(t *testing.T) {
	if *stringPtr("value") != "value" || !*boolPtr(true) || *uintPtr(3) != 3 {
		t.Fatal("pointer helper returned unexpected value")
	}
}
