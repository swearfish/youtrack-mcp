package cli

import (
	"os"
	"path/filepath"
	"testing"

	"youtrack-mcp/internal/config"
)

func TestRootCommandPersistentPreRunLoadsEnvFile(t *testing.T) {
	envPath := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(envPath, []byte("YOUTRACK_URL=https://example.test\n"), 0o644); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	cmd := NewRootCommand()
	if err := os.Unsetenv(config.EnvYouTrackURL); err != nil {
		t.Fatalf("unset env var: %v", err)
	}
	if err := cmd.PersistentFlags().Set("env-file", envPath); err != nil {
		t.Fatalf("set env-file flag: %v", err)
	}
	if err := cmd.PersistentPreRunE(cmd, nil); err != nil {
		t.Fatalf("run persistent pre-run: %v", err)
	}
	if got := os.Getenv(config.EnvYouTrackURL); got != "https://example.test" {
		t.Fatalf("expected env file to be applied, got %q", got)
	}
}

func TestRootCommandRegistersExpectedSubcommands(t *testing.T) {
	cmd := NewRootCommand()

	if got := cmd.CommandPath(); got != "youtrack-mcp" {
		t.Fatalf("unexpected command path: %s", got)
	}
	if found, _, err := cmd.Find([]string{"mcp"}); err != nil || found == nil {
		t.Fatalf("expected mcp subcommand to be registered")
	}
	if found, _, err := cmd.Find([]string{"install"}); err != nil || found == nil {
		t.Fatalf("expected install subcommand to be registered")
	}
}
