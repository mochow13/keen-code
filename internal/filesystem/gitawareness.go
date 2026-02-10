package filesystem

import (
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
)

type GitAwareness struct {
	matchers []gitignore.Matcher
}

func NewGitAwareness() *GitAwareness {
	return &GitAwareness{
		matchers: make([]gitignore.Matcher, 0),
	}
}

func (g *GitAwareness) LoadGitignore(path string) error {
	// TODO: Implement gitignore loading
	return nil
}

func (g *GitAwareness) IsIgnored(filePath string) bool {
	// TODO: Implement ignore checking
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
