package llm

import (
	"github.com/firebase/genkit/go/ai"
	"github.com/user/keen-code/internal/tools"
)

func ToGenkitTool(t tools.Tool) *ai.ToolDef[any, any] {
	return ai.NewTool(
		t.Name(),
		t.Description(),
		func(ctx *ai.ToolContext, input any) (any, error) {
			return t.Execute(ctx, input)
		},
		ai.WithInputSchema(t.InputSchema()),
	)
}

func ToGenkitTools(registry *tools.Registry) []ai.ToolRef {
	var genkitTools []ai.ToolRef
	for _, t := range registry.All() {
		genkitTools = append(genkitTools, ToGenkitTool(t))
	}
	return genkitTools
}
