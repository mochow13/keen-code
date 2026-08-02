package session

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Store struct {
	workingDir   string
	rootDir      string
	namespaceDir string
}

type Session struct {
	ID             string
	CreatedAt      time.Time
	Directory      string
	TranscriptPath string
	nextSeq        uint64
}

type LoadedSession struct {
	Summary Summary
	Events  []Event
	Session *Session
}

func NewStore(workingDir string) (*Store, error) {
	abs, err := filepath.Abs(workingDir)
	if err != nil {
		return nil, fmt.Errorf("resolve working directory: %w", err)
	}

	rootDir, err := sessionsRootDir()
	if err != nil {
		return nil, err
	}

	return &Store{
		workingDir:   filepath.Clean(abs),
		rootDir:      rootDir,
		namespaceDir: filepath.Join(rootDir, namespaceDirName(abs)),
	}, nil
}

func (s *Store) List() ([]Summary, error) {
	entries, err := os.ReadDir(s.namespaceDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read session namespace: %w", err)
	}

	summaries := make([]Summary, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		dir := filepath.Join(s.namespaceDir, entry.Name())
		transcriptPath := filepath.Join(dir, transcriptFileName)
		info, err := os.Stat(transcriptPath)
		if err != nil {
			continue
		}

		events, err := loadEvents(transcriptPath)
		if err != nil {
			return nil, err
		}

		summary := summarize(entry.Name(), dir, transcriptPath, info.ModTime(), events)
		if summary.ID == "" && len(events) == 0 {
			continue
		}
		summaries = append(summaries, summary)
	}

	sort.Slice(summaries, func(i, j int) bool {
		if summaries[i].UpdatedAt.Equal(summaries[j].UpdatedAt) {
			return summaries[i].LastSeq > summaries[j].LastSeq
		}
		return summaries[i].UpdatedAt.After(summaries[j].UpdatedAt)
	})

	return summaries, nil
}

func PruneExpired(root string, cutoff time.Time) error {
	rootInfo, err := os.Lstat(root)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect sessions directory: %w", err)
	}
	if !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("sessions directory %q is not a directory", root)
	}

	namespaces, err := os.ReadDir(root)
	if err != nil {
		return fmt.Errorf("read sessions directory: %w", err)
	}
	for _, namespace := range namespaces {
		if !namespace.IsDir() || namespace.Type()&os.ModeSymlink != 0 {
			continue
		}
		namespaceDir := filepath.Join(root, namespace.Name())
		entries, err := os.ReadDir(namespaceDir)
		if err != nil {
			return fmt.Errorf("read session namespace: %w", err)
		}
		for _, entry := range entries {
			if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
				continue
			}
			dir := filepath.Join(namespaceDir, entry.Name())
			info, err := os.Stat(filepath.Join(dir, transcriptFileName))
			if err != nil || !info.Mode().IsRegular() || !info.ModTime().Before(cutoff) {
				continue
			}
			if err := os.RemoveAll(dir); err != nil {
				return fmt.Errorf("remove expired session %q: %w", dir, err)
			}
		}
	}
	return nil
}

func (s *Store) Create() (*Session, error) {
	createdAt := time.Now().UTC()
	sessionID, err := generateSessionID()
	if err != nil {
		return nil, err
	}

	dir := filepath.Join(s.namespaceDir, sessionDirName(sessionID))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create session directory: %w", err)
	}

	session := &Session{
		ID:             sessionID,
		CreatedAt:      createdAt,
		Directory:      dir,
		TranscriptPath: filepath.Join(dir, transcriptFileName),
		nextSeq:        1,
	}

	started := Event{
		Kind: KindSessionStarted,
		SessionStarted: &SessionStartedPayload{
			SessionID: sessionID,
			CreatedAt: createdAt,
			CWD:       s.workingDir,
		},
	}

	if err := s.Append(session, started); err != nil {
		return nil, err
	}

	return session, nil
}

func (s *Store) Append(session *Session, event Event) error {
	if session == nil {
		return fmt.Errorf("session is nil")
	}

	event.Seq = session.nextSeq
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal session event: %w", err)
	}

	file, err := os.OpenFile(session.TranscriptPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return fmt.Errorf("open transcript file: %w", err)
	}
	defer file.Close()

	if _, err := file.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("append transcript event: %w", err)
	}

	session.nextSeq++
	return nil
}

// AppendBatch atomically persists events as one JSONL transcript update.
func (s *Store) AppendBatch(session *Session, events []Event) error {
	if session == nil {
		return fmt.Errorf("session is nil")
	}
	if len(events) == 0 {
		return nil
	}

	data := make([]byte, 0)
	for i := range events {
		events[i].Seq = session.nextSeq + uint64(i)
		encoded, err := json.Marshal(events[i])
		if err != nil {
			return fmt.Errorf("marshal session event: %w", err)
		}
		data = append(data, encoded...)
		data = append(data, '\n')
	}

	existing, err := os.ReadFile(session.TranscriptPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read transcript file: %w", err)
	}

	temp, err := os.CreateTemp(session.Directory, ".transcript-*")
	if err != nil {
		return fmt.Errorf("create transcript transaction: %w", err)
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0600); err != nil {
		temp.Close()
		return fmt.Errorf("set transcript transaction permissions: %w", err)
	}
	if _, err := temp.Write(existing); err != nil {
		temp.Close()
		return fmt.Errorf("write transcript transaction: %w", err)
	}
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return fmt.Errorf("write transcript transaction: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close transcript transaction: %w", err)
	}
	if err := os.Rename(tempPath, session.TranscriptPath); err != nil {
		return fmt.Errorf("commit transcript transaction: %w", err)
	}

	session.nextSeq += uint64(len(events))
	return nil
}

func (s *Store) Load(summary Summary) (*LoadedSession, error) {
	events, err := loadEvents(summary.TranscriptPath)
	if err != nil {
		return nil, err
	}

	session := &Session{
		ID:             summary.ID,
		CreatedAt:      summary.CreatedAt,
		Directory:      summary.Directory,
		TranscriptPath: summary.TranscriptPath,
		nextSeq:        maxSeq(events) + 1,
	}

	return &LoadedSession{
		Summary: summary,
		Events:  events,
		Session: session,
	}, nil
}

func loadEvents(path string) ([]Event, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open transcript file: %w", err)
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	events := make([]Event, 0)

	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			line = []byte(strings.TrimSpace(string(line)))
			if len(line) > 0 {
				var event Event
				if decodeErr := json.Unmarshal(line, &event); decodeErr == nil {
					events = append(events, event)
				}
			}
		}

		if err == nil {
			continue
		}
		if err == io.EOF {
			break
		}
		return nil, fmt.Errorf("read transcript events: %w", err)
	}

	return events, nil
}

func summarize(name, directory, transcriptPath string, updatedAt time.Time, events []Event) Summary {
	summary := Summary{
		Directory:      directory,
		TranscriptPath: transcriptPath,
		UpdatedAt:      updatedAt,
		LastSeq:        maxSeq(events),
	}

	for _, event := range events {
		switch event.Kind {
		case KindSessionStarted:
			if event.SessionStarted != nil {
				summary.ID = event.SessionStarted.SessionID
				summary.CreatedAt = event.SessionStarted.CreatedAt
			}
		case KindUserMessage:
			if event.UserMessage != nil {
				summary.LastUserMessage = strings.Join(strings.Fields(event.UserMessage.Content), " ")
			}
		}
	}

	if summary.CreatedAt.IsZero() {
		summary.CreatedAt = updatedAt
	}

	if summary.ID == "" {
		summary.ID = name
	}

	return summary
}
func maxSeq(events []Event) uint64 {
	var max uint64
	for _, event := range events {
		if event.Seq > max {
			max = event.Seq
		}
	}
	return max
}
