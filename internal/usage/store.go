package usage

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const dateLayout = "2006-01-02"

// staleLockAge bounds how long a leftover compaction lock is honored before it is treated as crashed.
const staleLockAge = 5 * time.Minute

// ErrLocked is returned by Compact when another compaction holds a fresh lock.
var ErrLocked = errors.New("usage compaction locked")

// Record is one ledger entry: either a raw per-response entry (TS set)
// or a rolled-up per-day entry (Date set, Rollup true).
type Record struct {
	TS                                              time.Time
	Date                                            string
	Provider, Model                                 string
	Input, Output, CacheRead, CacheWrite, Reasoning int
	Rollup                                          bool
}

type recordJSON struct {
	TS         string `json:"ts,omitempty"`
	Date       string `json:"date,omitempty"`
	Provider   string `json:"provider"`
	Model      string `json:"model"`
	Input      int    `json:"input"`
	Output     int    `json:"output"`
	CacheRead  int    `json:"cache_read"`
	CacheWrite int    `json:"cache_write"`
	Reasoning  int    `json:"reasoning"`
	Rollup     bool   `json:"rollup,omitempty"`
}

func NewRecord(provider, model string, input, output, cacheRead, cacheWrite, reasoning int) (Record, bool) {
	if provider == "" || model == "" {
		return Record{}, false
	}
	return Record{
		TS:         time.Now().UTC(),
		Provider:   provider,
		Model:      model,
		Input:      input,
		Output:     output,
		CacheRead:  cacheRead,
		CacheWrite: cacheWrite,
		Reasoning:  reasoning,
	}, true
}

type Store struct {
	path string
}

func NewStore(path string) *Store {
	return &Store{path: path}
}

func DefaultStore() (*Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home directory: %w", err)
	}
	return NewStore(filepath.Join(home, ".keen", "usage", "usage.jsonl")), nil
}

func (s *Store) Path() string {
	return s.path
}

func (s *Store) lockPath() string {
	return filepath.Join(filepath.Dir(s.path), "usage.lock")
}

func (s *Store) markerPath() string {
	return filepath.Join(filepath.Dir(s.path), ".compacted")
}

func (s *Store) Append(rec Record) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create usage directory: %w", err)
	}
	recJSON := toJSON(rec)
	data, err := json.Marshal(recJSON)
	if err != nil {
		return fmt.Errorf("encode usage record: %w", err)
	}
	data = append(data, '\n')
	f, err := os.OpenFile(s.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open usage ledger: %w", err)
	}
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("append usage record: %w", err)
	}
	return nil
}

// Load reads all records, skipping blank and corrupt lines.
func (s *Store) Load() ([]Record, error) {
	f, err := os.Open(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open usage ledger: %w", err)
	}
	defer f.Close()
	var out []Record
	r := bufio.NewReader(f)
	for {
		line, err := r.ReadString('\n')
		if line != "" {
			if rec, ok := parseLine(line); ok {
				out = append(out, rec)
			}
		}
		if err != nil {
			break
		}
	}
	return out, nil
}

func parseLine(line string) (Record, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return Record{}, false
	}
	var raw recordJSON
	if err := json.Unmarshal([]byte(trimmed), &raw); err != nil {
		return Record{}, false
	}
	if raw.Provider == "" || raw.Model == "" {
		return Record{}, false
	}
	if raw.Date != "" {
		day, err := time.Parse(dateLayout, raw.Date)
		if err != nil {
			return Record{}, false
		}
		return Record{
			Date:       day.Format(dateLayout),
			Provider:   raw.Provider,
			Model:      raw.Model,
			Input:      raw.Input,
			Output:     raw.Output,
			CacheRead:  raw.CacheRead,
			CacheWrite: raw.CacheWrite,
			Reasoning:  raw.Reasoning,
			Rollup:     true,
		}, true
	}
	if raw.TS == "" {
		return Record{}, false
	}
	ts, err := time.Parse(time.RFC3339Nano, raw.TS)
	if err != nil {
		return Record{}, false
	}
	return Record{
		TS:         ts,
		Provider:   raw.Provider,
		Model:      raw.Model,
		Input:      raw.Input,
		Output:     raw.Output,
		CacheRead:  raw.CacheRead,
		CacheWrite: raw.CacheWrite,
		Reasoning:  raw.Reasoning,
	}, true
}

func toJSON(rec Record) recordJSON {
	if rec.Rollup || rec.Date != "" {
		date := rec.Date
		if date == "" && !rec.TS.IsZero() {
			date = rec.TS.UTC().Format(dateLayout)
		}
		return recordJSON{
			Date: date, Provider: rec.Provider, Model: rec.Model,
			Input: rec.Input, Output: rec.Output,
			CacheRead: rec.CacheRead, CacheWrite: rec.CacheWrite,
			Reasoning: rec.Reasoning, Rollup: true,
		}
	}
	ts := rec.TS
	if ts.IsZero() {
		ts = time.Now().UTC()
	}
	return recordJSON{
		TS: ts.UTC().Format(time.RFC3339Nano), Provider: rec.Provider, Model: rec.Model,
		Input: rec.Input, Output: rec.Output,
		CacheRead: rec.CacheRead, CacheWrite: rec.CacheWrite,
		Reasoning: rec.Reasoning,
	}
}

func recordDay(rec Record) (time.Time, bool) {
	ts, ok := recordTime(rec)
	if !ok {
		return time.Time{}, false
	}
	return utcDay(ts), true
}

func utcDay(t time.Time) time.Time {
	y, m, d := t.UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

type rollupKey struct {
	date     string
	provider string
	model    string
}

// Compact collapses every record into one rollup per (UTC day, provider, model)
// and drops rollups older than a year, rewriting the ledger atomically. Records
// without a usable timestamp are left untouched.
func (s *Store) Compact(now time.Time) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create usage directory: %w", err)
	}
	release, err := s.acquireLock()
	if err != nil {
		return err
	}
	defer release()

	records, err := s.Load()
	if err != nil {
		return err
	}
	retention := utcDay(now).AddDate(-1, 0, 0)
	var keep []Record
	agg := make(map[rollupKey]*Record)
	var keys []rollupKey
	for _, rec := range records {
		day, ok := recordDay(rec)
		if !ok {
			keep = append(keep, rec)
			continue
		}
		if day.Before(retention) {
			continue
		}
		key := rollupKey{date: day.Format(dateLayout), provider: rec.Provider, model: rec.Model}
		slot, exists := agg[key]
		if !exists {
			slot = &Record{Date: key.date, Provider: key.provider, Model: key.model, Rollup: true}
			agg[key] = slot
			keys = append(keys, key)
		}
		slot.Input += rec.Input
		slot.Output += rec.Output
		slot.CacheRead += rec.CacheRead
		slot.CacheWrite += rec.CacheWrite
		slot.Reasoning += rec.Reasoning
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].date != keys[j].date {
			return keys[i].date < keys[j].date
		}
		if keys[i].provider != keys[j].provider {
			return keys[i].provider < keys[j].provider
		}
		return keys[i].model < keys[j].model
	})
	return s.rewrite(appendRollupsKeys(agg, keys, keep))
}

func appendRollupsKeys(agg map[rollupKey]*Record, keys []rollupKey, keep []Record) []Record {
	out := make([]Record, 0, len(keys)+len(keep))
	for _, k := range keys {
		out = append(out, *agg[k])
	}
	return append(out, keep...)
}

func (s *Store) rewrite(records []Record) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create usage directory: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), "usage-*.tmp")
	if err != nil {
		return fmt.Errorf("create usage temp file: %w", err)
	}
	tmpName := tmp.Name()
	w := bufio.NewWriter(tmp)
	var writeErr error
	for _, rec := range records {
		data, err := json.Marshal(toJSON(rec))
		if err != nil {
			writeErr = err
			break
		}
		if _, err := w.Write(append(data, '\n')); err != nil {
			writeErr = err
			break
		}
	}
	if writeErr == nil {
		writeErr = w.Flush()
	}
	if writeErr == nil {
		writeErr = tmp.Close()
	} else {
		tmp.Close()
	}
	if writeErr != nil {
		os.Remove(tmpName)
		return fmt.Errorf("write usage temp file: %w", writeErr)
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("chmod usage temp file: %w", err)
	}
	if err := os.Rename(tmpName, s.path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("replace usage ledger: %w", err)
	}
	return nil
}

func (s *Store) acquireLock() (func(), error) {
	lock := s.lockPath()
	for i := 0; i < 2; i++ {
		f, err := os.OpenFile(lock, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			f.Close()
			return func() { os.Remove(lock) }, nil
		}
		if !os.IsExist(err) {
			return nil, fmt.Errorf("acquire usage lock: %w", err)
		}
		info, statErr := os.Stat(lock)
		if statErr != nil {
			if os.IsNotExist(statErr) {
				continue
			}
			return nil, fmt.Errorf("stat usage lock: %w", statErr)
		}
		if time.Since(info.ModTime()) > staleLockAge {
			os.Remove(lock)
			continue
		}
		return nil, fmt.Errorf("%w: %s", ErrLocked, lock)
	}
	return nil, fmt.Errorf("%w: %s", ErrLocked, lock)
}

// MaybeCompact runs Compact at most once per UTC calendar day, tracking the
// last run in a marker file next to the ledger. It is a no-op when the ledger
// does not exist yet, so callers can invoke it on every startup cheaply.
func (s *Store) MaybeCompact(now time.Time) error {
	if _, err := os.Stat(s.path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat usage ledger: %w", err)
	}
	today := now.UTC().Format(dateLayout)
	if data, err := os.ReadFile(s.markerPath()); err == nil {
		if strings.TrimSpace(string(data)) == today {
			return nil
		}
	}
	if err := s.Compact(now); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create usage directory: %w", err)
	}
	if err := os.WriteFile(s.markerPath(), []byte(today+"\n"), 0o644); err != nil {
		return fmt.Errorf("write compaction marker: %w", err)
	}
	return nil
}
