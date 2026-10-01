package usage

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func mustAppend(t *testing.T, s *Store, rec Record) {
	t.Helper()
	if err := s.Append(rec); err != nil {
		t.Fatalf("append: %v", err)
	}
}

func sumInputs(records []Record) (in, out int) {
	for _, r := range records {
		in += r.Input
		out += r.Output
	}
	return in, out
}

func TestCompactRollsUpEveryDay(t *testing.T) {
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	s := newTestStore(t)
	today := time.Date(2026, 9, 28, 9, 0, 0, 0, time.UTC)
	yesterday := time.Date(2026, 9, 27, 9, 0, 0, 0, time.UTC)
	mustAppend(t, s, Record{TS: today, Provider: "a", Model: "m", Input: 10, Output: 1, CacheRead: 4, CacheWrite: 2})
	mustAppend(t, s, Record{TS: today.Add(time.Hour), Provider: "a", Model: "m", Input: 20, Output: 2})
	mustAppend(t, s, Record{TS: yesterday, Provider: "a", Model: "m", Input: 30, Output: 3})
	before, _ := s.Load()
	beforeIn, beforeOut := sumInputs(before)

	if err := s.Compact(now); err != nil {
		t.Fatalf("compact: %v", err)
	}
	after, err := s.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	afterIn, afterOut := sumInputs(after)
	if beforeIn != afterIn || beforeOut != afterOut {
		t.Fatalf("compaction must preserve totals: before (%d,%d) after (%d,%d)", beforeIn, beforeOut, afterIn, afterOut)
	}
	if len(after) != 2 {
		t.Fatalf("want one row per day, got %+v", after)
	}
	byDate := map[string]Record{}
	for _, r := range after {
		if !r.Rollup {
			t.Fatalf("every record, including today, should roll up: %+v", r)
		}
		byDate[r.Date] = r
	}
	if got := byDate["2026-09-28"]; got.Input != 30 || got.Output != 3 || got.CacheRead != 4 || got.CacheWrite != 2 {
		t.Fatalf("today rollup wrong: %+v", got)
	}
	if got := byDate["2026-09-27"]; got.Input != 30 || got.Output != 3 {
		t.Fatalf("yesterday rollup wrong: %+v", got)
	}
}

func TestCompactDropsRecordsOlderThanOneYear(t *testing.T) {
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	s := newTestStore(t)
	// retention = 2025-09-28; days strictly before it are dropped.
	tooOld := time.Date(2025, 9, 27, 8, 0, 0, 0, time.UTC)
	boundary := time.Date(2025, 9, 28, 8, 0, 0, 0, time.UTC)
	recent := time.Date(2026, 9, 28, 8, 0, 0, 0, time.UTC)
	mustAppend(t, s, Record{Date: "2025-01-01", Provider: "a", Model: "m", Input: 5000, Output: 500, Rollup: true})
	mustAppend(t, s, Record{TS: tooOld, Provider: "a", Model: "m", Input: 999, Output: 99})
	mustAppend(t, s, Record{TS: boundary, Provider: "a", Model: "m", Input: 100, Output: 10})
	mustAppend(t, s, Record{TS: recent, Provider: "a", Model: "m", Input: 50, Output: 5})

	if err := s.Compact(now); err != nil {
		t.Fatalf("compact: %v", err)
	}
	got, err := s.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	in, out := sumInputs(got)
	if in != 150 || out != 15 {
		t.Fatalf("records older than a year must be dropped: in=%d out=%d %+v", in, out, got)
	}
	for _, r := range got {
		if r.Date == "2025-01-01" || r.Date == "2025-09-27" {
			t.Fatalf("expired record survived: %+v", r)
		}
	}
	if len(got) != 2 {
		t.Fatalf("want boundary and recent days only, got %+v", got)
	}
}

func TestCompactGroupsByDayProviderModel(t *testing.T) {
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	s := newTestStore(t)
	day1a := time.Date(2026, 8, 5, 1, 0, 0, 0, time.UTC)
	day1b := time.Date(2026, 8, 5, 23, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	mustAppend(t, s, Record{TS: day1a, Provider: "a", Model: "m1", Input: 10, Output: 1})
	mustAppend(t, s, Record{TS: day1b, Provider: "a", Model: "m1", Input: 20, Output: 2})
	mustAppend(t, s, Record{TS: day1a, Provider: "a", Model: "m2", Input: 30, Output: 3})
	mustAppend(t, s, Record{TS: day2, Provider: "a", Model: "m1", Input: 40, Output: 4})
	if err := s.Compact(now); err != nil {
		t.Fatalf("compact: %v", err)
	}
	got, _ := s.Load()
	if len(got) != 3 {
		t.Fatalf("want 3 rollups, got %+v", got)
	}
	byDateModel := map[string]int{}
	for _, r := range got {
		if !r.Rollup {
			t.Fatalf("all old records should roll up: %+v", r)
		}
		byDateModel[r.Date+"/"+r.Model] = r.Input
	}
	if byDateModel["2026-08-05/m1"] != 30 || byDateModel["2026-08-05/m2"] != 30 || byDateModel["2026-08-06/m1"] != 40 {
		t.Fatalf("wrong grouping: %+v", byDateModel)
	}
}

func TestCompactIdempotentAndMergesPartialDay(t *testing.T) {
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	s := newTestStore(t)
	oldTS := time.Date(2026, 8, 10, 8, 0, 0, 0, time.UTC)
	mustAppend(t, s, Record{Date: "2026-08-10", Provider: "a", Model: "m", Input: 100, Output: 10, Rollup: true})
	mustAppend(t, s, Record{TS: oldTS.Add(2 * time.Hour), Provider: "a", Model: "m", Input: 25, Output: 3})
	if err := s.Compact(now); err != nil {
		t.Fatalf("compact: %v", err)
	}
	first, _ := s.Load()
	if len(first) != 1 || !first[0].Rollup || first[0].Input != 125 || first[0].Output != 13 {
		t.Fatalf("partial day should merge: %+v", first)
	}
	if err := s.Compact(now); err != nil {
		t.Fatalf("second compact: %v", err)
	}
	second, _ := s.Load()
	if len(second) != 1 || second[0].Input != 125 || second[0].Output != 13 || second[0].Date != "2026-08-10" {
		t.Fatalf("re-run must be idempotent: %+v", second)
	}
}

func TestCompactLockContention(t *testing.T) {
	s := newTestStore(t)
	mustAppend(t, s, Record{TS: time.Now().UTC(), Provider: "a", Model: "m", Input: 1})
	lockPath := filepath.Join(filepath.Dir(s.Path()), "usage.lock")
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lockPath, []byte("holder"), 0o644); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err := s.Compact(now); !errors.Is(err, ErrLocked) {
		t.Fatalf("want ErrLocked, got %v", err)
	}
	// Stale lock (mtime > 5min) must be recovered.
	stale := now.Add(-10 * time.Minute)
	if err := os.Chtimes(lockPath, stale, stale); err != nil {
		t.Fatal(err)
	}
	if err := s.Compact(now); err != nil {
		t.Fatalf("stale lock recovery: %v", err)
	}
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Fatalf("lock should be removed, stat err: %v", err)
	}
}

func TestMaybeCompactThrottle(t *testing.T) {
	day1 := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	s := newTestStore(t)
	oldTS := time.Date(2026, 8, 1, 8, 0, 0, 0, time.UTC)
	mustAppend(t, s, Record{TS: oldTS, Provider: "a", Model: "m", Input: 100, Output: 10})
	if err := s.MaybeCompact(day1); err != nil {
		t.Fatalf("maybe compact: %v", err)
	}
	got, _ := s.Load()
	if len(got) != 1 || !got[0].Rollup {
		t.Fatalf("first run should compact: %+v", got)
	}
	// New old data added the same day must wait for the next day.
	mustAppend(t, s, Record{TS: oldTS.Add(time.Hour), Provider: "a", Model: "m", Input: 50, Output: 5})
	if err := s.MaybeCompact(day1.Add(2 * time.Hour)); err != nil {
		t.Fatalf("throttled run: %v", err)
	}
	got, _ = s.Load()
	raws := 0
	for _, r := range got {
		if !r.Rollup {
			raws++
		}
	}
	if raws != 1 {
		t.Fatalf("throttled run must not compact: %+v", got)
	}
	if err := s.MaybeCompact(day1.Add(24 * time.Hour)); err != nil {
		t.Fatalf("next-day run: %v", err)
	}
	got, _ = s.Load()
	for _, r := range got {
		if !r.Rollup {
			t.Fatalf("next-day run should compact: %+v", got)
		}
	}
	marker, err := os.ReadFile(filepath.Join(filepath.Dir(s.Path()), ".compacted"))
	if err != nil {
		t.Fatal(err)
	}
	if string(marker) != "2026-09-29\n" {
		t.Fatalf("wrong marker: %q", marker)
	}
}

func TestCompactUTCDateGrouping(t *testing.T) {
	// Two instants on opposite sides of a UTC midnight boundary belong to different rollups.
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	s := newTestStore(t)
	justBefore := time.Date(2026, 8, 10, 23, 30, 0, 0, time.UTC)
	justAfter := time.Date(2026, 8, 11, 0, 30, 0, 0, time.UTC)
	mustAppend(t, s, Record{TS: justBefore, Provider: "a", Model: "m", Input: 10, Output: 1})
	mustAppend(t, s, Record{TS: justAfter, Provider: "a", Model: "m", Input: 20, Output: 2})
	if err := s.Compact(now); err != nil {
		t.Fatalf("compact: %v", err)
	}
	got, _ := s.Load()
	if len(got) != 2 {
		t.Fatalf("UTC-day split wrong: %+v", got)
	}
}

func TestMaybeCompactSkipsMissingLedger(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(filepath.Join(dir, "usage.jsonl"))
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	if err := s.MaybeCompact(now); err != nil {
		t.Fatalf("maybe compact: %v", err)
	}
	if _, err := os.Stat(s.Path()); !os.IsNotExist(err) {
		t.Fatalf("missing ledger should not be created, stat err: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".compacted")); !os.IsNotExist(err) {
		t.Fatalf("missing ledger should not write a marker, stat err: %v", err)
	}
}

func TestCompactCreatesMissingDirectory(t *testing.T) {
	s := NewStore(filepath.Join(t.TempDir(), "nested", "usage", "usage.jsonl"))
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	if err := s.Compact(now); err != nil {
		t.Fatalf("compact: %v", err)
	}
	if _, err := os.Stat(s.Path()); err != nil {
		t.Fatalf("expected ledger to be created: %v", err)
	}
}
