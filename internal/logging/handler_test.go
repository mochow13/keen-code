package logging

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestPrettyHandlerEnabled(t *testing.T) {
	h := &prettyHandler{level: slog.LevelInfo}
	if h.Enabled(context.Background(), slog.LevelDebug) || !h.Enabled(context.Background(), slog.LevelInfo) {
		t.Fatal("unexpected level filtering")
	}
}

func TestPrettyHandlerHandle(t *testing.T) {
	for _, level := range []slog.Level{slog.LevelDebug, slog.LevelInfo, slog.LevelWarn, slog.LevelError} {
		var output bytes.Buffer
		h := &prettyHandler{w: &output, level: slog.LevelDebug}
		record := slog.NewRecord(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), level, "message", 0)
		record.Add("value", map[string]any{"bytes": []byte("text")})
		if err := h.Handle(context.Background(), record); err != nil {
			t.Fatalf("Handle() error = %v", err)
		}
		if !strings.Contains(output.String(), "message") || !strings.Contains(output.String(), "text") {
			t.Fatalf("unexpected output %q", output.String())
		}
	}
}

func TestFormatValue(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  string
	}{
		{"nil", nil, "null"},
		{"plain", "hello", "hello"},
		{"json", `{"key":"value"}`, `"key"`},
		{"bytes", []byte("bytes"), "bytes"},
		{"map", map[string]any{"nested": map[string]any{"bytes": []byte("value")}}, "value"},
		{"slice", []any{[]byte("value"), map[string]any{"key": []byte("nested")}}, "nested"},
		{"number", 42, "42"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatValue(tt.value, "  "); !strings.Contains(got, tt.want) {
				t.Fatalf("formatValue() = %q, want substring %q", got, tt.want)
			}
		})
	}
}

func TestPrettyHandlerWithMethods(t *testing.T) {
	h := &prettyHandler{}
	if h.WithAttrs(nil) != h || h.WithGroup("group") != h {
		t.Fatal("handler With methods should preserve handler")
	}
}
