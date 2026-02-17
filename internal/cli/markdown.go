package cli

import (
	"github.com/charmbracelet/glamour"
)

type MarkdownRenderer struct {
	renderer *glamour.TermRenderer
	width    int
}

func NewMarkdownRenderer(width int) (*MarkdownRenderer, error) {
	renderer, err := glamour.NewTermRenderer(
		glamour.WithStylePath("dark"),
		glamour.WithWordWrap(width-4),
	)
	if err != nil {
		return nil, err
	}

	return &MarkdownRenderer{
		renderer: renderer,
		width:    width,
	}, nil
}

func (mr *MarkdownRenderer) Render(markdown string) string {
	if markdown == "" {
		return ""
	}

	rendered, err := mr.renderer.Render(markdown)
	if err != nil {
		return markdown
	}
	return rendered
}

func (mr *MarkdownRenderer) UpdateWidth(width int) error {
	if mr.width == width {
		return nil
	}

	renderer, err := glamour.NewTermRenderer(
		glamour.WithStylePath("dark"),
		glamour.WithWordWrap(width-4),
	)
	if err != nil {
		return err
	}

	mr.renderer = renderer
	mr.width = width
	return nil
}
