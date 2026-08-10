package filesearch

import (
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/mochow13/keen-code/internal/filesystem"
)

const defaultRefreshInterval = 10 * time.Second

type FileSearcher struct {
	workingDir      string
	refreshInterval time.Duration
	mu              sync.RWMutex
	cache           []string
	cached          bool
	refreshing      bool
	updatedAt       time.Time
}

func NewFileSearcher(workingDir string, _ *filesystem.Guard) *FileSearcher {
	return &FileSearcher{
		workingDir:      workingDir,
		refreshInterval: defaultRefreshInterval,
	}
}

// Search returns up to limit relative paths whose names contain query as a substring.
// Returns nil if query is empty.
// The file list is cached on first call and refreshed on demand in the background.
func (s *FileSearcher) Search(query string, limit int) []string {
	if query == "" {
		return nil
	}

	s.ensureCache()
	cache := s.snapshotCache()
	s.refreshStaleCache()

	query = strings.ToLower(query)
	var results []string
	for _, p := range cache {
		if strings.Contains(strings.ToLower(p), query) {
			results = append(results, p)
			if len(results) >= limit {
				break
			}
		}
	}
	return results
}

func (s *FileSearcher) ensureCache() {
	s.mu.RLock()
	cached := s.cached
	s.mu.RUnlock()
	if cached {
		return
	}

	paths, _ := gitLsFiles(s.workingDir)

	s.mu.Lock()
	if !s.cached {
		s.cache = paths
		s.cached = true
		s.updatedAt = time.Now()
	}
	s.mu.Unlock()
}

func (s *FileSearcher) snapshotCache() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cache
}

func (s *FileSearcher) refreshStaleCache() {
	s.mu.Lock()
	if s.refreshing || time.Since(s.updatedAt) < s.refreshInterval {
		s.mu.Unlock()
		return
	}
	s.refreshing = true
	s.mu.Unlock()

	go s.rebuildCache()
}

func (s *FileSearcher) rebuildCache() {
	paths, ok := gitLsFilesIncludingUntracked(s.workingDir)

	s.mu.Lock()
	defer s.mu.Unlock()
	if ok {
		s.cache = paths
		s.cached = true
	}
	s.updatedAt = time.Now()
	s.refreshing = false
}

func gitLsFiles(dir string) ([]string, bool) {
	return gitLsFilesWithArgs(dir, "--cached")
}

func gitLsFilesIncludingUntracked(dir string) ([]string, bool) {
	return gitLsFilesWithArgs(dir, "--cached", "--others", "--exclude-standard")
}

func gitLsFilesWithArgs(dir string, args ...string) ([]string, bool) {
	cmdArgs := append([]string{"ls-files", "-z"}, args...)
	cmd := exec.Command("git", cmdArgs...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return nil, false
	}

	var paths []string
	seen := make(map[string]struct{})
	for _, path := range strings.Split(string(out), "\x00") {
		if path == "" {
			continue
		}
		if _, ok := seen[path]; !ok {
			paths = append(paths, path)
			seen[path] = struct{}{}
		}
		for d := filepath.Dir(path); d != "."; d = filepath.Dir(d) {
			if _, ok := seen[d]; ok {
				continue
			}
			paths = append(paths, d)
			seen[d] = struct{}{}
		}
	}

	return paths, true
}
