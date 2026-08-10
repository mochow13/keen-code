package cmd

import (
	"fmt"

	"github.com/mochow13/keen-code/internal/session"
)

func loadResumeSession(workingDir, sessionID string) (*session.LoadedSession, error) {
	store, err := session.NewStore(workingDir)
	if err != nil {
		return nil, fmt.Errorf("failed to open session store: %w", err)
	}
	summaries, err := store.List()
	if err != nil {
		return nil, fmt.Errorf("failed to list sessions: %w", err)
	}
	for _, summary := range summaries {
		if summary.ID == sessionID {
			return store.Load(summary)
		}
	}
	return nil, fmt.Errorf("session %q not found", sessionID)
}
