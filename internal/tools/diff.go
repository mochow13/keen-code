package tools

type EditDiffLineKind int

const (
	DiffLineContext EditDiffLineKind = iota
	DiffLineAdded
	DiffLineRemoved
	DiffLineHunk
)

type EditDiffLine struct {
	Kind       EditDiffLineKind
	OldLineNum int    // 0 for added lines and hunk headers
	NewLineNum int    // 0 for removed lines and hunk headers
	Content    string // raw line content without +/- prefix
}

type DiffEmitter interface {
	EmitDiff(lines []EditDiffLine)
}
