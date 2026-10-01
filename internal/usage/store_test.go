package usage

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	return NewStore(filepath.Join(t.TempDir(), "usage.jsonl"))
}

func TestAppendLoadRoundtrip(t *testing.T) {
	s := newTestStore(t)
	ts := time.Date(2026, 9, 10, 18, 22, 4, 0, time.UTC)
	raw := Record{TS: ts, Provider: "anthropic", Model: "claude-sonnet-4-5",
		Input: 18432, Output: 512, CacheRead: 16000, CacheWrite: 1024}
	rollup := Record{Date: "2026-08-14", Provider: "anthropic", Model: "claude-sonnet-4-5",
		Input: 90210, Output: 4410, CacheRead: 77120, CacheWrite: 3072, Reasoning: 5, Rollup: true}
	if err := s.Append(raw); err != nil {
		t.Fatalf("append raw: %v", err)
	}
	if err := s.Append(rollup); err != nil {
		t.Fatalf("append rollup: %v", err)
	}
	got, err := s.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d records, want 2", len(got))
	}
	if !got[0].TS.Equal(ts) || got[0].Provider != "anthropic" || got[0].Input != 18432 ||
		got[0].CacheRead != 16000 || got[0].CacheWrite != 1024 || got[0].Rollup {
		t.Fatalf("raw mismatch: %+v", got[0])
	}
	if got[1].Date != "2026-08-14" || !got[1].Rollup || got[1].Reasoning != 5 {
		t.Fatalf("rollup mismatch: %+v", got[1])
	}
}

func TestLoadMissingFile(t *testing.T) {
	s := newTestStore(t)
	records, err := s.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("got %d records, want 0", len(records))
	}
}

func TestLoadCorruptTolerance(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "usage.jsonl")
	ts := time.Date(2026, 9, 10, 18, 22, 4, 0, time.UTC)
	content := "{\"ts\":\"" + ts.Format(time.RFC3339) + "\",\"provider\":\"anthropic\",\"model\":\"m1\",\"input\":10,\"output\":1}\n" +
		"\n" +
		"not json at all\n" +
		"{\"ts\":\"bad-time\",\"provider\":\"anthropic\",\"model\":\"m2\",\"input\":5}\n" +
		"{\"provider\":\"anthropic\",\"input\":5}\n" +
		"{\"ts\":\"2026-09-10T18:22:04Z\",\"provider\":\"\",\"model\":\"m3\"}\n" +
		"{\"ts\":\"2026-09-10T18:22:04Z\",\"provider\":\"anthropic\",\"model\":\"m4\",\"input\":7}\n" +
		"{\"ts\":\"2026-09-10T18:22:04Z\",\"provider\":\"anthropic\",\"model\":\"truncated\",\"input\":"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	s := NewStore(path)
	got, err := s.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d records, want 2: %+v", len(got), got)
	}
	if got[0].Model != "m1" || got[1].Model != "m4" {
		t.Fatalf("unexpected records: %+v", got)
	}
}

func TestLoadSkipsBadRollupDate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "usage.jsonl")
	content := "{\"date\":\"not-a-date\",\"provider\":\"a\",\"model\":\"m\",\"input\":1,\"rollup\":true}\n" +
		"{\"date\":\"2026-08-14\",\"provider\":\"a\",\"model\":\"m\",\"input\":2,\"rollup\":true}\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := NewStore(path).Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(got) != 1 || got[0].Input != 2 || !got[0].Rollup {
		t.Fatalf("unexpected records: %+v", got)
	}
}
