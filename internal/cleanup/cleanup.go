package cleanup

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/user/keen-code/internal/session"
)

const (
	artifactRetention = 7 * 24 * time.Hour
	logRetention      = 14 * 24 * time.Hour
	sessionRetention  = 14 * 24 * time.Hour
	historyLimit      = 1000
)

func Run() error {
	for _, name := range []string{"bash", "mcp-artifacts", "web-fetch-artifacts"} {
		dir, err := keenPath(name)
		if err != nil {
			return err
		}
		if _, err := PruneExpiredFiles(dir, artifactRetention); err != nil {
			return err
		}
	}

	logDir, err := keenPath("logs")
	if err != nil {
		return err
	}
	if _, err := PruneExpiredFiles(logDir, logRetention); err != nil {
		return err
	}

	sessionsDir, err := keenPath("sessions")
	if err != nil {
		return err
	}
	if err := session.PruneExpired(sessionsDir, time.Now().Add(-sessionRetention)); err != nil {
		return err
	}

	historyPath, err := keenPath("input-history")
	if err != nil {
		return err
	}
	return TrimHistory(historyPath, historyLimit)
}

func keenPath(name string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve Keen directory: %w", err)
	}
	if home == "" {
		return "", fmt.Errorf("resolve Keen directory: home directory is empty")
	}
	return filepath.Join(home, ".keen", name), nil
}

func PruneExpiredFiles(dir string, maxAge time.Duration) (int, error) {
	return pruneExpiredFilesBefore(dir, time.Now().Add(-maxAge))
}

func pruneExpiredFilesBefore(dir string, cutoff time.Time) (int, error) {
	if err := validateRoot(dir); err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, fmt.Errorf("read cleanup directory: %w", err)
	}

	removed := 0
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() || !info.ModTime().Before(cutoff) {
			continue
		}
		if err := os.Remove(filepath.Join(dir, entry.Name())); err != nil {
			return removed, fmt.Errorf("remove expired file: %w", err)
		}
		removed++
	}
	return removed, nil
}

func TrimHistory(path string, limit int) error {
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open input history: %w", err)
	}

	var entries []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		entries = append(entries, scanner.Text())
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close input history: %w", err)
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read input history: %w", err)
	}
	if len(entries) <= limit {
		return nil
	}

	temp, err := os.CreateTemp(filepath.Dir(path), ".input-history-*")
	if err != nil {
		return fmt.Errorf("create input history temp file: %w", err)
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0600); err != nil {
		temp.Close()
		return fmt.Errorf("secure input history temp file: %w", err)
	}

	writer := bufio.NewWriter(temp)
	for _, entry := range entries[len(entries)-limit:] {
		if _, err := writer.WriteString(strings.ReplaceAll(entry, "\n", `\n`) + "\n"); err != nil {
			temp.Close()
			return fmt.Errorf("write input history: %w", err)
		}
	}
	if err := writer.Flush(); err != nil {
		temp.Close()
		return fmt.Errorf("flush input history: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close input history temp file: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("replace input history: %w", err)
	}
	return nil
}

func validateRoot(root string) error {
	info, err := os.Lstat(root)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("cleanup root %q is not a directory", root)
	}
	return nil
}
