package ai

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Verifier executes validation routines for automation workflows.
type Verifier interface {
	Run(ctx context.Context) error
}

// CommandVerifier runs a sequence of shell commands, streaming output.
type CommandVerifier struct {
	Commands [][]string
}

// Run executes configured commands in order, stopping at the first failure.
func (v CommandVerifier) Run(ctx context.Context) error {
	for _, cmd := range v.Commands {
		if len(cmd) == 0 {
			continue
		}
		c := exec.CommandContext(ctx, cmd[0], cmd[1:]...)
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		if err := c.Run(); err != nil {
			return fmt.Errorf("command %q failed: %w", strings.Join(cmd, " "), err)
		}
	}
	return nil
}

// BasicVerifier returns a verifier covering fmt/vet/test and an HTTP probe.
func BasicVerifier() Verifier {
	return CommandVerifier{
		Commands: [][]string{
			{"go", "fmt", "./..."},
			{"go", "vet", "./..."},
			{"go", "test", "./..."},
			{"curl", "-sf", "http://localhost:8080/healthz"},
		},
	}
}
