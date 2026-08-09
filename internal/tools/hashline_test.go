package tools

import (
	"reflect"
	"regexp"
	"testing"
)

func TestComputeLineHash_FixedVectors(t *testing.T) {
	tests := []struct {
		line     string
		expected string
	}{
		{"", "811"},
		{"a", "e40"},
		{"abc", "1a4"},
		{"Hello, World!", "5ae"},
		{"package tools", "15c"},
		{"\treturn fmt.Sprintf(\"Hello, %s\", name)", "dae"},
		{"return nil", "48d"},
		{"}", "f80"},
		{"こんにちは", "1cf"},
		{"\xc3\xa9", "1e9"},     // "é" NFC
		{"\x65\xcc\x81", "fa9"}, // "e" + combining acute (NFD)
	}

	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			got := computeLineHash([]byte(tt.line))
			if got != tt.expected {
				t.Errorf("computeLineHash(%q) = %q, expected %q", tt.line, got, tt.expected)
			}
		})
	}
}

func TestComputeLineHash_ThreeLowercaseHex(t *testing.T) {
	re := regexp.MustCompile(`^[0-9a-f]{3}$`)
	inputs := []string{"", "a", "X", "Hello, World!", "\n", "\t", "こんにちは", "\r\n", "a\r"}
	for _, in := range inputs {
		got := computeLineHash([]byte(in))
		if !re.MatchString(got) {
			t.Errorf("computeLineHash(%q) = %q, not exactly three lowercase hex characters", in, got)
		}
	}
}

func TestComputeLineHash_Repeatable(t *testing.T) {
	inputs := []string{"", "a", "abc", "Hello, World!", "こんにちは"}
	for _, in := range inputs {
		first := computeLineHash([]byte(in))
		for range 10 {
			if got := computeLineHash([]byte(in)); got != first {
				t.Fatalf("computeLineHash(%q) is not repeatable: %q vs %q", in, got, first)
			}
		}
	}
}

func TestComputeLineHash_UnicodeFormsDiffer(t *testing.T) {
	nfc := computeLineHash([]byte("\xc3\xa9"))     // "é" precomposed
	nfd := computeLineHash([]byte("\x65\xcc\x81")) // "e" + combining accent
	if nfc == nfd {
		t.Errorf("NFC and NFD forms must hash differently, both %q", nfc)
	}
}

func TestSplitRawLines(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected []string
	}{
		{"empty file", "", nil},
		{"single line no newline", "a", []string{"a"}},
		{"single line trailing newline", "a\n", []string{"a"}},
		{"two lines trailing newline", "a\nb\n", []string{"a", "b"}},
		{"two lines no trailing newline", "a\nb", []string{"a", "b"}},
		{"single newline", "\n", []string{""}},
		{"two newlines", "\n\n", []string{"", ""}},
		{"blank line between", "a\n\nb\n", []string{"a", "", "b"}},
		{"consecutive trailing blank lines", "a\n\n\n", []string{"a", "", ""}},
		{"crlf trailing newline", "a\r\nb\r\n", []string{"a", "b"}},
		{"crlf no trailing newline", "a\r\nb", []string{"a", "b"}},
		{"crlf blank line", "a\r\n\r\n", []string{"a", ""}},
		{"only crlf", "\r\n", []string{""}},
		{"lone carriage return is content", "a\r", []string{"a\r"}},
		{"unicode content", "こんにちは\n世界\n", []string{"こんにちは", "世界"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines := splitRawLines([]byte(tt.content))
			got := make([]string, len(lines))
			for i, l := range lines {
				got[i] = string(l)
			}
			if len(got) == 0 && len(tt.expected) == 0 {
				if got != nil && tt.expected != nil {
					t.Errorf("splitRawLines(%q) = %q, expected %q", tt.content, got, tt.expected)
				}
				return
			}
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("splitRawLines(%q) = %q, expected %q", tt.content, got, tt.expected)
			}
		})
	}
}

func TestSplitRawLines_CRLFHashConsistency(t *testing.T) {
	// CRLF's \r is part of the delimiter: the hash of a logical line from a
	// CRLF file must equal the hash of the same content without \r.
	lines := splitRawLines([]byte("Hello, World!\r\nnext\r\n"))
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	if hash := computeLineHash(lines[0]); hash != computeLineHash([]byte("Hello, World!")) {
		t.Errorf("CRLF line hash %q does not match LF hash %q", hash, computeLineHash([]byte("Hello, World!")))
	}
}

func TestSplitRawLines_SharedSlicesReferenceInput(t *testing.T) {
	content := []byte("one\ntwo\n")
	lines := splitRawLines(content)
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	// The returned slices must alias the input buffer (no copies).
	if &lines[0][0] != &content[0] {
		t.Error("expected line slices to reference the input buffer")
	}
}
