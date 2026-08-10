package markdown

import (
	"regexp"
	"strings"

	"github.com/mochow13/keen-code/internal/cli/repl/urldetect"
)

const (
	osc8Open  = "\x1b]8;;"
	osc8ST    = "\x1b\\"
	osc8Close = "\x1b]8;;\x1b\\"
)

// ansiEscape matches ANSI/OSC sequences, skipped during URL detection.
var ansiEscape = regexp.MustCompile(`\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)|\x1b\[[0-9;]*[A-Za-z]`)

// makeURLsClickable wraps bare http(s) URLs in OSC 8 escapes; visible text is unchanged.
func makeURLsClickable(rendered string) string {
	if rendered == "" || !strings.Contains(rendered, "http") {
		return rendered
	}

	var b strings.Builder
	b.Grow(len(rendered))

	underlined := false
	for len(rendered) > 0 {
		loc := ansiEscape.FindStringIndex(rendered)
		if loc != nil && loc[0] == 0 {
			esc := rendered[:loc[1]]
			underlined = underlineActive(underlined, esc)
			b.WriteString(esc)
			rendered = rendered[loc[1]:]
			continue
		}

		end := len(rendered)
		if loc != nil {
			end = loc[0]
		}

		b.WriteString(linkifySegment(rendered[:end], underlined))
		rendered = rendered[end:]
	}

	return b.String()
}

// underlineActive returns whether underline is on after applying an SGR escape.
func underlineActive(current bool, esc string) bool {
	if !strings.HasPrefix(esc, "\x1b[") || !strings.HasSuffix(esc, "m") {
		return current
	}
	params := esc[2 : len(esc)-1]
	if params == "" {
		return false
	}
	for p := range strings.SplitSeq(params, ";") {
		switch p {
		case "0", "":
			current = false
		case "4":
			current = true
		case "24":
			current = false
		}
	}
	return current
}

// linkifySegment wraps URLs in an escape-free segment, underlining the visible
// URL when surrounding text is not already underlined.
func linkifySegment(segment string, underlined bool) string {
	if !strings.Contains(segment, "http") {
		return segment
	}

	var b strings.Builder
	last := 0
	for _, loc := range urldetect.Pattern().FindAllStringIndex(segment, -1) {
		raw := segment[loc[0]:loc[1]]
		url, lead := urldetect.Trim(raw)
		if url == "" {
			continue
		}
		b.WriteString(segment[last : loc[0]+lead])

		visible := url
		if !underlined {
			visible = "\x1b[4m" + url + "\x1b[24m"
		}
		b.WriteString(osc8Open + url + osc8ST + visible + osc8Close)
		last = loc[0] + lead + len(url)
	}
	b.WriteString(segment[last:])
	return b.String()
}
