package filesystem

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
)

type PatternSet struct {
	matcher gitignore.Matcher
	domain  []string
}

type GitAwareness struct {
	patternSets []PatternSet
}

func NewGitAwareness() *GitAwareness {
	return &GitAwareness{
		patternSets: make([]PatternSet, 0),
	}
}

func (g *GitAwareness) LoadGitignoreRecursive(root string) error {
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if strings.HasPrefix(d.Name(), ".gitignore") && !d.IsDir() {
			return g.LoadGitignore(path)
		}
		return nil
	})
}

func (g *GitAwareness) LoadGitignore(gitignorePath string) error {
	file, err := os.Open(gitignorePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer file.Close()

	dir := filepath.Dir(gitignorePath)
	domain := strings.Split(dir, string(filepath.Separator))

	var patterns []gitignore.Pattern
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, gitignore.ParsePattern(line, domain))
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	if len(patterns) > 0 {
		g.patternSets = append(g.patternSets, PatternSet{
			matcher: gitignore.NewMatcher(patterns),
			domain:  domain,
		})
	}

	return nil
}

func (g *GitAwareness) IsIgnored(filePath string) bool {
	pathParts := strings.Split(filePath, string(filepath.Separator))

	for _, ps := range g.patternSets {
		if ps.matcher.Match(pathParts, false) {
			return true
		}
	}
	return false
}

func (g *GitAwareness) FilterPaths(paths []string) []string {
	result := make([]string, 0, len(paths))
	for _, path := range paths {
		if !g.IsIgnored(path) {
			result = append(result, path)
		}
	}
	return result
}
