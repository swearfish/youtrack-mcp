package install

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRegisterVSCodeWorkspaceWritesServersConfig(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	result, err := Register(RegisterOptions{
		Target:    "vscode",
		Workspace: workspace,
		Binary:    "/tmp/youtrack-mcp",
	})
	if err != nil {
		t.Fatalf("register vscode workspace: %v", err)
	}

	configPath := filepath.Join(workspace, ".vscode", "mcp.json")
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}

	if result.ConfigPath != configPath {
		t.Fatalf("unexpected config path: %s", result.ConfigPath)
	}
	text := string(content)
	if !strings.Contains(text, `"servers"`) {
		t.Fatalf("expected servers object in %s", text)
	}
	if !strings.Contains(text, `"command": "/tmp/youtrack-mcp"`) {
		t.Fatalf("expected binary path in %s", text)
	}
	if !strings.Contains(text, `"args": [`) || !strings.Contains(text, `"mcp"`) {
		t.Fatalf("expected mcp args in %s", text)
	}
}

func TestRegisterCopilotWorkspaceWritesMCPServersConfig(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	_, err := Register(RegisterOptions{
		Target:    "copilot",
		Workspace: workspace,
		Binary:    "/tmp/youtrack-mcp",
	})
	if err != nil {
		t.Fatalf("register copilot workspace: %v", err)
	}

	configPath := filepath.Join(workspace, ".copilot", "mcp.json")
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}

	text := string(content)
	if !strings.Contains(text, `"mcpServers"`) {
		t.Fatalf("expected mcpServers object in %s", text)
	}
	if !strings.Contains(text, `"tools": [`) || !strings.Contains(text, `"*"`) {
		t.Fatalf("expected tool wildcard in %s", text)
	}
}

func TestRegisterDryRunDoesNotWriteFile(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	result, err := Register(RegisterOptions{
		Target:    "vscode",
		Workspace: workspace,
		Binary:    "/tmp/youtrack-mcp",
		DryRun:    true,
	})
	if err != nil {
		t.Fatalf("dry-run register: %v", err)
	}

	if _, err := os.Stat(filepath.Join(workspace, ".vscode", "mcp.json")); !os.IsNotExist(err) {
		t.Fatalf("expected no config file to be written, got err=%v", err)
	}
	if !strings.Contains(result.Message, "Would write MCP registration") {
		t.Fatalf("unexpected message: %s", result.Message)
	}
}

func TestRegisterIncludesOverrideEnvFlagWhenRequested(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	envPath := filepath.Join(workspace, ".env")
	if err := os.WriteFile(envPath, []byte("YOUTRACK_URL=https://example.test\n"), 0o644); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	_, err := Register(RegisterOptions{
		Target:      "vscode",
		Workspace:   workspace,
		Binary:      "/tmp/youtrack-mcp",
		EnvFile:     envPath,
		OverrideEnv: true,
	})
	if err != nil {
		t.Fatalf("register config: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(workspace, ".vscode", "mcp.json"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	text := string(content)
	if !strings.Contains(text, `"--override-env"`) {
		t.Fatalf("expected --override-env in %s", text)
	}
}

func TestDefaultCopilotConfigPathUsesMCPConfigJSON(t *testing.T) {
	t.Setenv("HOME", "/tmp/test-home")
	t.Setenv("COPILOT_MCP_CONFIG_PATH", "")

	path := defaultCopilotConfigPath()
	if path != filepath.Join("/tmp/test-home", ".copilot", "mcp-config.json") {
		t.Fatalf("unexpected default copilot config path: %s", path)
	}
}

func TestLoadConfigSupportsJSONC(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "mcp.json")
	content := "{\n  // comment\n  \"servers\": {\n    /* block */\n    \"existing\": {\"command\": \"x\"}\n  }\n}\n"
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	changed, err := upsertConfig(configPath, "servers", "youtrack-mcp", map[string]any{"command": "/tmp/youtrack-mcp"})
	if err != nil {
		t.Fatalf("upsert config: %v", err)
	}
	if !changed {
		t.Fatalf("expected config to change")
	}

	updated, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read updated config: %v", err)
	}
	text := string(updated)
	if !strings.Contains(text, "// comment") || !strings.Contains(text, "/* block */") {
		t.Fatalf("expected comments to survive, got %s", text)
	}
	if !strings.Contains(text, `"youtrack-mcp"`) {
		t.Fatalf("expected server entry to be added, got %s", text)
	}
}

func TestUpsertConfigTreatsEmptyFileAsMissing(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "mcp.json")
	if err := os.WriteFile(configPath, nil, 0o644); err != nil {
		t.Fatalf("write empty config: %v", err)
	}

	changed, err := upsertConfig(configPath, "servers", "youtrack-mcp", map[string]any{"command": "/tmp/youtrack-mcp"})
	if err != nil {
		t.Fatalf("upsert empty config: %v", err)
	}
	if !changed {
		t.Fatalf("expected empty config to be populated")
	}

	updated, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read updated config: %v", err)
	}
	text := string(updated)
	if !strings.Contains(text, `"servers"`) || !strings.Contains(text, `"youtrack-mcp"`) {
		t.Fatalf("expected server entry in populated config, got %s", text)
	}
}

func TestRegisterPreservesExistingJSONCComments(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	configPath := filepath.Join(workspace, ".vscode", "mcp.json")
	content := "{\n  // comment\n  \"servers\": {\n    \"existing\": {\"command\": \"x\"}\n  }\n}\n"
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("create config dir: %v", err)
	}
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	result, err := Register(RegisterOptions{
		Target:    "vscode",
		Workspace: workspace,
		Binary:    "/tmp/youtrack-mcp",
	})
	if err != nil {
		t.Fatalf("register config: %v", err)
	}

	if result.Message == "" {
		t.Fatalf("expected non-empty result message")
	}

	updated, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	text := string(updated)
	if !strings.Contains(text, "// comment") {
		t.Fatalf("expected line comment to survive, got %s", text)
	}
	if !strings.Contains(text, `"youtrack-mcp"`) {
		t.Fatalf("expected server entry to be added, got %s", text)
	}
}

func TestWriteFileAtomicPreservesExistingPermissions(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.json")
	if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("write original file: %v", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatalf("chmod original file: %v", err)
	}

	if err := writeFileAtomic(path, []byte("{\"servers\":{}}\n"), 0o644); err != nil {
		t.Fatalf("write file atomically: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat updated file: %v", err)
	}
	if perms := info.Mode().Perm(); perms != 0o600 {
		t.Fatalf("expected permissions 0600, got %#o", perms)
	}
}
