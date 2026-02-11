package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/charmbracelet/lipgloss"
	"github.com/user/keen-cli/internal/config"
)

func abbreviateHome(path string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if strings.HasPrefix(path, home) {
		return "~" + strings.TrimPrefix(path, home)
	}
	return path
}

var (
	primaryColor   = lipgloss.Color("#7C3AED")
	secondaryColor = lipgloss.Color("#10B981")
	mutedColor     = lipgloss.Color("#6B7280")
	accentColor    = lipgloss.Color("#F59E0B")
	titleStyle     = lipgloss.NewStyle().
			Bold(true).
			Foreground(primaryColor)
	infoLabelStyle = lipgloss.NewStyle().
			Foreground(mutedColor).
			Width(18)
	infoValueStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#E5E7EB"))
	highlightStyle = lipgloss.NewStyle().
			Foreground(secondaryColor).
			Bold(true)
	modeStyle = lipgloss.NewStyle().
			Foreground(accentColor).
			Bold(true)
	tipStyle = lipgloss.NewStyle().
			Foreground(mutedColor).
			Italic(true)
	boxStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(mutedColor).
			Padding(1, 2).
			MarginTop(1)
	outputStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#E5E7EB"))
	promptStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(primaryColor)
)

func RunREPL(version, workingDir string, cfg *config.ResolvedConfig) error {
	fmt.Println()
	fmt.Printf("  🤖  %s  %s\n\n", titleStyle.Render("Keen v"+version), modeStyle.Render("plan mode"))

	var info strings.Builder
	displayDir := abbreviateHome(workingDir)
	info.WriteString(fmt.Sprintf("  %s %s\n",
		infoLabelStyle.Render("Directory:"),
		infoValueStyle.Render(displayDir)))
	info.WriteString(fmt.Sprintf("  %s %s\n",
		infoLabelStyle.Render("Provider:"),
		highlightStyle.Render(cfg.Provider)))
	info.WriteString(fmt.Sprintf("  %s %s\n",
		infoLabelStyle.Render("Model:"),
		infoValueStyle.Render(cfg.Model)))

	fmt.Println(info.String())

	tips := []string{
		"Type /help  for available commands",
		"Type /exit  to quit",
		"Type /model to change provider or model",
	}
	fmt.Println(boxStyle.Render(tipStyle.Render(strings.Join(tips, "\n"))))

	fmt.Println()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print(promptStyle.Render("> "))

		scanCh := make(chan bool)
		go func() {
			scanCh <- scanner.Scan()
			close(scanCh)
		}()

		select {
		case <-ctx.Done():
			fmt.Println()
			fmt.Println(lipgloss.NewStyle().Foreground(mutedColor).Render("  Goodbye!"))
			return nil
		case scanned := <-scanCh:
			if !scanned {
				fmt.Println()
				fmt.Println(lipgloss.NewStyle().Foreground(mutedColor).Render("  Goodbye!"))
				return nil
			}
		}

		input := strings.TrimSpace(scanner.Text())

		if input == "/exit" {
			fmt.Println()
			fmt.Println(lipgloss.NewStyle().Foreground(mutedColor).Render("  Goodbye!"))
			break
		}
		if input == "" {
			continue
		}

		fmt.Println(outputStyle.Render("  " + input))
		fmt.Println()
	}

	return scanner.Err()
}
