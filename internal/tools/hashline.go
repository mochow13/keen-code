package tools

import (
	"bytes"
	"fmt"
	"hash/fnv"
)

const anchorHashLen = 3

func computeLineHash(line []byte) string {
	h := fnv.New32a()
	_, _ = h.Write(line)
	return fmt.Sprintf("%08x", h.Sum32())[:anchorHashLen]
}

func splitRawLines(content []byte) [][]byte {
	if len(content) == 0 {
		return nil
	}
	lines := make([][]byte, 0, bytes.Count(content, []byte("\n"))+1)
	start := 0
	for i := range content {
		if content[i] == '\n' {
			end := i
			if end > start && content[end-1] == '\r' {
				end--
			}
			lines = append(lines, content[start:end])
			start = i + 1
		}
	}
	if start < len(content) {
		lines = append(lines, content[start:])
	}
	return lines
}

func joinLines(lines [][]byte, original []byte) []byte {
	sep := []byte("\n")
	if bytes.Contains(original, []byte("\r\n")) {
		sep = []byte("\r\n")
	}
	total := 0
	for _, l := range lines {
		total += len(l) + len(sep)
	}
	out := make([]byte, 0, total)
	for _, l := range lines {
		out = append(out, l...)
		out = append(out, sep...)
	}
	if len(lines) > 0 && (len(original) == 0 || original[len(original)-1] != '\n') {
		out = out[:len(out)-len(sep)]
	}
	return out
}
