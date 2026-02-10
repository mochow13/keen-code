// Package cli provides the command-line interface for Keen.
package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// NewRootCommand creates the root command for the CLI.
func NewRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "keen",
		Short: "Keen - A coding agent CLI",
		Long:  `Keen is a terminal-based coding agent that provides AI-assisted code editing.`,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("Keen CLI - Use --help for available commands")
		},
	}

	cmd.Flags().StringP("config", "c", "", "Config file path")
	cmd.Flags().BoolP("verbose", "v", false, "Enable verbose output")

	return cmd
}
