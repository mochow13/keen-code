package output

import (
	"strings"
	"testing"
)

func TestCompactionStatuses(t *testing.T) {
	builder := NewOutputBuilder(80, "")
	AddCompactionSuccessStatus(builder, "compacted")
	AddCompactionErrorStatus(builder, "failed")
	AddCompactionCancelledStatus(builder, "cancelled")

	got := builder.Join()
	for _, want := range []string{"✓ compacted", "✗ failed", "cancelled"} {
		if !strings.Contains(got, want) {
			t.Fatalf("status output %q missing %q", got, want)
		}
	}
}

func TestCompactionStatusIgnoresMissingOutput(t *testing.T) {
	builder := NewOutputBuilder(80, "")
	AddCompactionSuccessStatus(nil, "ignored")
	AddCompactionSuccessStatus(builder, "")
	if !builder.IsEmpty() {
		t.Fatal("missing output added a compaction status")
	}
}
