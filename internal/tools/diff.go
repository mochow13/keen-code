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
	OldLineNum int
	NewLineNum int
	Content    string
}

type DiffEmitter interface {
	EmitDiff(lines []EditDiffLine)
}
