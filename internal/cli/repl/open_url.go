package repl

import (
	"os/exec"
	"runtime"

	"github.com/charmbracelet/x/ansi"
	"github.com/clipperhouse/displaywidth"

	"github.com/mochow13/keen-code/internal/cli/repl/urldetect"
)

// urlAtDisplayColumn returns the URL covering the given display column on a
// (possibly ANSI-styled) line, or "" if none.
func urlAtDisplayColumn(line string, col int) string {
	if col < 0 {
		return ""
	}

	plain := ansi.Strip(line)
	for _, loc := range urldetect.Pattern().FindAllStringIndex(plain, -1) {
		raw := plain[loc[0]:loc[1]]
		url, lead := urldetect.Trim(raw)
		if url == "" {
			continue
		}
		startCol := displaywidth.String(plain[:loc[0]+lead])
		endCol := startCol + displaywidth.String(url)
		if col >= startCol && col < endCol {
			return url
		}
	}
	return ""
}

// openURLCmd opens a URL in the default browser; a var so tests can stub it.
var openURLCmd = func(url string) *exec.Cmd {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url)
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return exec.Command("xdg-open", url)
	}
}

func openURL(url string) error {
	cmd := openURLCmd(url)
	if cmd == nil {
		return nil
	}
	return cmd.Start()
}
