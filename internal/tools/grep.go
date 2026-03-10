package tools

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/user/keen-code/internal/filesystem"
)

const maxMatchLimit = 1000

type searchConfig struct {
	pattern     string
	regex       *regexp.Regexp
	basePath    string
	includeGlob string
	outputMode  string
}

type searchResult struct {
	files   []string
	matches []map[string]any
	count   int
}

type GrepTool struct {
	guard               *filesystem.Guard
	permissionRequester PermissionRequester
}

func NewGrepTool(guard *filesystem.Guard, permissionRequester PermissionRequester) *GrepTool {
	return &GrepTool{
		guard:               guard,
		permissionRequester: permissionRequester,
	}
}

func (t *GrepTool) Name() string {
	return "grep"
}

func (t *GrepTool) Description() string {
	return "Search for text patterns in files recursively after filesystem policy + user permission checks."
}

func (t *GrepTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"pattern": map[string]any{
				"type":        "string",
				"description": "Regular expression pattern to search for in file contents",
			},
			"path": map[string]any{
				"type":        "string",
				"description": "Optional base directory for the search (defaults to working directory)",
			},
			"include": map[string]any{
				"type":        "string",
				"description": "Optional glob pattern to filter which files to search (e.g., '*.go', '**/*.md')",
			},
			"output_mode": map[string]any{
				"type":        "string",
				"enum":        []string{"file", "content"},
				"description": "Output mode: 'file' returns matching file paths, 'content' returns matching lines with file and line number (defaults to 'content')",
			},
		},
		"required":             []string{"pattern"},
		"additionalProperties": false,
	}
}

func (t *GrepTool) Execute(ctx context.Context, input any) (any, error) {
	config, err := t.parseSearchConfig(input)
	if err != nil {
		return nil, err
	}

	resolvedPath, err := t.guard.ResolvePath(config.basePath)
	if err != nil {
		return nil, fmt.Errorf("path resolution failed: %w", err)
	}

	if err := t.checkPermission(ctx, config.basePath, resolvedPath); err != nil {
		return nil, err
	}

	result, err := t.searchFiles(resolvedPath, config)
	if err != nil {
		return nil, err
	}

	return t.formatResult(config, result), nil
}

func (t *GrepTool) parseSearchConfig(input any) (*searchConfig, error) {
	params, ok := input.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid input: expected map[string]any, got %T", input)
	}

	pattern, err := t.extractPattern(params)
	if err != nil {
		return nil, err
	}

	regex, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid pattern: %v", err)
	}

	includeGlob, err := t.extractIncludeGlob(params)
	if err != nil {
		return nil, err
	}

	outputMode, err := t.extractOutputMode(params)
	if err != nil {
		return nil, err
	}

	return &searchConfig{
		pattern:     pattern,
		regex:       regex,
		basePath:    t.extractBasePath(params),
		includeGlob: includeGlob,
		outputMode:  outputMode,
	}, nil
}

func (t *GrepTool) extractPattern(params map[string]any) (string, error) {
	patternValue, ok := params["pattern"]
	if !ok {
		return "", fmt.Errorf("invalid input: missing required 'pattern' parameter")
	}

	pattern, ok := patternValue.(string)
	if !ok || pattern == "" {
		return "", fmt.Errorf("invalid input: pattern must be a non-empty string")
	}

	return pattern, nil
}

func (t *GrepTool) extractBasePath(params map[string]any) string {
	if pathValue, exists := params["path"]; exists {
		if pathStr, ok := pathValue.(string); ok {
			return pathStr
		}
	}
	return ""
}

func (t *GrepTool) extractIncludeGlob(params map[string]any) (string, error) {
	includeValue, exists := params["include"]
	if !exists {
		return "", nil
	}

	includeStr, ok := includeValue.(string)
	if !ok || includeStr == "" {
		return "", nil
	}

	if !doublestar.ValidatePattern(includeStr) {
		return "", fmt.Errorf("invalid include: malformed glob pattern %q", includeStr)
	}

	return includeStr, nil
}

func (t *GrepTool) extractOutputMode(params map[string]any) (string, error) {
	modeValue, exists := params["output_mode"]
	if !exists {
		return "content", nil
	}

	modeStr, ok := modeValue.(string)
	if !ok {
		return "content", nil
	}

	switch modeStr {
	case "file", "content":
		return modeStr, nil
	default:
		return "", fmt.Errorf("invalid output_mode: %q (must be 'file' or 'content')", modeStr)
	}
}

func (t *GrepTool) checkPermission(ctx context.Context, basePath, resolvedPath string) error {
	permission := t.guard.CheckPath(resolvedPath, "read")

	switch permission {
	case filesystem.PermissionDenied:
		return fmt.Errorf("permission denied by policy: path %q is blocked", basePath)
	case filesystem.PermissionPending:
		return t.requestPermission(ctx, basePath, resolvedPath)
	}

	return nil
}

func (t *GrepTool) requestPermission(ctx context.Context, basePath, resolvedPath string) error {
	if t.permissionRequester == nil {
		return fmt.Errorf("permission denied: user approval required but not available")
	}

	allowed, err := t.permissionRequester.RequestPermission(ctx, t.Name(), basePath, resolvedPath, "read", false)
	if err != nil {
		return fmt.Errorf("permission request failed: %w", err)
	}

	if !allowed {
		return fmt.Errorf("permission denied by user: read access rejected for path %q", basePath)
	}

	return nil
}

func (t *GrepTool) searchFiles(basePath string, config *searchConfig) (*searchResult, error) {
	searcher := &fileSearcher{
		guard:      t.guard,
		basePath:   basePath,
		config:     config,
		result:     &searchResult{},
		matchLimit: maxMatchLimit,
	}

	err := filepath.WalkDir(basePath, searcher.walkFn)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	return searcher.result, nil
}

type fileSearcher struct {
	guard      *filesystem.Guard
	basePath   string
	config     *searchConfig
	result     *searchResult
	matchLimit int
}

func (s *fileSearcher) walkFn(path string, d fs.DirEntry, err error) error {
	if err != nil {
		return err
	}

	if d.IsDir() {
		return s.handleDirectory(path)
	}

	return s.handleFile(path)
}

func (s *fileSearcher) handleDirectory(path string) error {
	if s.guard.IsBlocked(path) {
		return fs.SkipDir
	}
	return nil
}

func (s *fileSearcher) handleFile(path string) error {
	if s.guard.IsBlocked(path) {
		return nil
	}

	if !s.matchesIncludeGlob(path) {
		return nil
	}

	matches, err := s.searchInFile(path)
	if err != nil || len(matches) == 0 {
		return nil
	}

	return s.addMatches(path, matches)
}

func (s *fileSearcher) matchesIncludeGlob(path string) bool {
	if s.config.includeGlob == "" {
		return true
	}

	relPath, err := filepath.Rel(s.basePath, path)
	if err != nil {
		return false
	}

	matched, err := doublestar.Match(s.config.includeGlob, filepath.ToSlash(relPath))
	if err != nil {
		return false
	}

	return matched
}

func (s *fileSearcher) addMatches(path string, matches []map[string]any) error {
	if s.config.outputMode == "file" {
		return s.addFileMatch(path)
	}
	return s.addContentMatches(matches)
}

func (s *fileSearcher) addFileMatch(path string) error {
	if s.result.count >= s.matchLimit {
		return fmt.Errorf("search too broad: found more than %d matches", s.matchLimit)
	}
	s.result.files = append(s.result.files, path)
	s.result.count++
	return nil
}

func (s *fileSearcher) addContentMatches(matches []map[string]any) error {
	for _, m := range matches {
		if s.result.count >= s.matchLimit {
			return fmt.Errorf("search too broad: found more than %d matches", s.matchLimit)
		}
		s.result.matches = append(s.result.matches, m)
		s.result.count++
	}
	return nil
}

func (s *fileSearcher) searchInFile(path string) ([]map[string]any, error) {
	content, err := readFileContent(path)
	if err != nil {
		return nil, nil
	}

	return s.scanContentForMatches(path, content)
}

func (s *fileSearcher) scanContentForMatches(path string, content []byte) ([]map[string]any, error) {
	var matches []map[string]any
	scanner := bufio.NewScanner(bytes.NewReader(content))
	lineNumber := 0

	for scanner.Scan() {
		lineNumber++
		line := scanner.Text()

		if s.config.regex.MatchString(line) {
			if s.config.outputMode == "file" {
				return []map[string]any{{}}, nil
			}
			matches = append(matches, map[string]any{
				"file":        path,
				"line_number": lineNumber,
				"line":        line,
			})
		}
	}

	return matches, nil
}

func (t *GrepTool) formatResult(config *searchConfig, result *searchResult) any {
	if config.outputMode == "file" {
		return map[string]any{
			"pattern":     config.pattern,
			"base_path":   config.basePath,
			"output_mode": config.outputMode,
			"files":       result.files,
			"count":       len(result.files),
		}
	}

	return map[string]any{
		"pattern":     config.pattern,
		"base_path":   config.basePath,
		"output_mode": config.outputMode,
		"matches":     result.matches,
		"count":       len(result.matches),
	}
}
