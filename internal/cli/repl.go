package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// NewReplCommand creates the REPL command.
func NewReplCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "repl",
		Short: "Start interactive REPL mode",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("REPL mode - TODO: Implement interactive shell")
		},
	}
}
