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
	"github.com/user/keen-cli/configs/providers"
	"github.com/user/keen-cli/internal/config"
)

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
	helpCmdStyle = lipgloss.NewStyle().
			Foreground(secondaryColor).
			Bold(true).
			Width(12)
	helpDescStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#E5E7EB"))
)

const (
	exitCommand  = "/exit"
	helpCommand  = "/help"
	modelCommand = "/model"
)

type replState struct {
	version    string
	workingDir string
	cfg        *config.ResolvedConfig
	globalCfg  *config.GlobalConfig
	loader     *config.Loader
	registry   *providers.Registry
}

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

func printHeader(version string) {
	asciiArt := []string{
		" █████   ████ ██████████ ██████████ ██████   █████",
		"░░███   ███░ ░░███░░░░░█░░███░░░░░█░░██████ ░░███",
		" ░███  ███    ░███  █ ░  ░███  █ ░  ░███░███ ░███",
		" ░███████     ░██████    ░██████    ░███░░███░███",
		" ░███░░███    ░███░░█    ░███░░█    ░███ ░░██████",
		" ░███ ░░███   ░███ ░   █ ░███ ░   █ ░███  ░░█████",
		" █████ ░░████ ██████████ ██████████ █████  ░░█████",
		"░░░░░   ░░░░ ░░░░░░░░░░ ░░░░░░░░░░ ░░░░░    ░░░░░",
	}

	colors := []string{
		"#00F2FE", "#05E5FE", "#10D3FE", "#1ABFFE", "#25ACFE", "#4FACFE", "#6696FE", "#7C3AED",
	}

	fmt.Println()
	for i, line := range asciiArt {
		color := colors[i%len(colors)]
		fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render(line))
	}

	fmt.Printf("\n  %s  %s\n\n", titleStyle.Render("Keen v"+version), modeStyle.Render("plan mode"))
}

func printInfo(workingDir string, cfg *config.ResolvedConfig) {
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
}

func printTips() {
	tips := []string{
		"Type /help  for available commands",
		"Type /exit  to quit",
		"Type /model to change provider or model",
	}
	fmt.Println(boxStyle.Render(tipStyle.Render(strings.Join(tips, "\n"))))
	fmt.Println()
}

func setupSignalHandling() context.Context {
	ctx, cancel := context.WithCancel(context.Background())

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	return ctx
}

func readInput(ctx context.Context, scanner *bufio.Scanner) (string, bool) {
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
		return "", false
	case scanned := <-scanCh:
		if !scanned {
			fmt.Println()
			fmt.Println(lipgloss.NewStyle().Foreground(mutedColor).Render("  Goodbye!"))
			return "", false
		}
	}

	return strings.TrimSpace(scanner.Text()), true
}

func printHelp() {
	cmds := []struct{ cmd, desc string }{
		{"/help", "Show available commands"},
		{"/model", "Change provider or model"},
		{"/exit", "Quit Keen"},
	}

	var lines []string
	for _, c := range cmds {
		lines = append(lines, helpCmdStyle.Render(c.cmd)+helpDescStyle.Render(c.desc))
	}

	header := titleStyle.Render("Available Commands")
	content := header + "\n\n" + strings.Join(lines, "\n")
	fmt.Println(boxStyle.Render(content))
	fmt.Println()
}

func (s *replState) handleInput(input string) bool {
	if input == exitCommand {
		fmt.Println()
		fmt.Println(lipgloss.NewStyle().Foreground(mutedColor).Render("  Goodbye!"))
		return false
	}

	if input == helpCommand {
		printHelp()
		return true
	}

	if input == modelCommand {
		resolved, err := RunSetup(s.loader, s.globalCfg, s.registry)
		if err != nil {
			fmt.Println(errorStyle.Render(fmt.Sprintf("  ✗ Model selection failed: %v", err)))
			fmt.Println()
			return true
		}
		s.cfg.Provider = resolved.Provider
		s.cfg.Model = resolved.Model
		s.cfg.APIKey = resolved.APIKey
		fmt.Println()
		printInfo(s.workingDir, s.cfg)
		return true
	}

	if input == "" {
		return true
	}

	fmt.Println(outputStyle.Render("  " + input))
	fmt.Println()
	return true
}

func RunREPL(version, workingDir string, cfg *config.ResolvedConfig, loader *config.Loader, globalCfg *config.GlobalConfig, registry *providers.Registry) error {
	printHeader(version)
	printInfo(workingDir, cfg)
	printTips()

	state := &replState{
		version:    version,
		workingDir: workingDir,
		cfg:        cfg,
		globalCfg:  globalCfg,
		loader:     loader,
		registry:   registry,
	}

	ctx := setupSignalHandling()
	scanner := bufio.NewScanner(os.Stdin)

	for {
		input, ok := readInput(ctx, scanner)
		if !ok {
			return nil
		}

		if !state.handleInput(input) {
			break
		}
	}

	return scanner.Err()
}
