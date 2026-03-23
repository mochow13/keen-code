package repl

import "strings"

type slashCommand struct {
	Name        string
	Description string
}

var allSlashCommands = []slashCommand{
	{"/exit", "Quit Keen"},
	{"/help", "Show available commands"},
	{"/model", "Change provider or model"},
}

func filterCommands(input string) []slashCommand {
	if input == "" || !strings.HasPrefix(input, "/") {
		return nil
	}
	prefix := strings.ToLower(strings.TrimPrefix(input, "/"))
	var results []slashCommand
	for _, cmd := range allSlashCommands {
		name := strings.TrimPrefix(cmd.Name, "/")
		if strings.HasPrefix(name, prefix) {
			results = append(results, cmd)
		}
	}
	return results
}
