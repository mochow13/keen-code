package cli

import (
	"testing"
)

func TestNewRootCommand(t *testing.T) {
	cmd := NewRootCommand("0.1.0")

	if cmd.Use != "keen" {
		t.Errorf("command Use = %q, want 'keen'", cmd.Use)
	}

	if cmd.Version != "0.1.0" {
		t.Errorf("command Version = %q, want '0.1.0'", cmd.Version)
	}

	if cmd.Short == "" {
		t.Error("command Short should not be empty")
	}

	if cmd.Long == "" {
		t.Error("command Long should not be empty")
	}
}

func TestNewRootCommand_DifferentVersion(t *testing.T) {
	cmd := NewRootCommand("1.2.3")

	if cmd.Version != "1.2.3" {
		t.Errorf("command Version = %q, want '1.2.3'", cmd.Version)
	}
}
