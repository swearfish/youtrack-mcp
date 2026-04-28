package envfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadParsesQuotesAndExportPrefix(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	content := "# Comment\nYOUTRACK_URL=\"https://example.test\"\nexport YOUTRACK_API_TOKEN='perm:token'\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	values, err := Load(path)
	if err != nil {
		t.Fatalf("load env file: %v", err)
	}

	if got := values["YOUTRACK_URL"]; got != "https://example.test" {
		t.Fatalf("unexpected YOUTRACK_URL: %q", got)
	}
	if got := values["YOUTRACK_API_TOKEN"]; got != "perm:token" {
		t.Fatalf("unexpected YOUTRACK_API_TOKEN: %q", got)
	}
}

func TestLoadParsesEmptyQuotedValues(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	content := "DOUBLE_EMPTY=\"\"\nSINGLE_EMPTY=''\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	values, err := Load(path)
	if err != nil {
		t.Fatalf("load env file: %v", err)
	}

	if got := values["DOUBLE_EMPTY"]; got != "" {
		t.Fatalf("unexpected DOUBLE_EMPTY: %q", got)
	}
	if got := values["SINGLE_EMPTY"]; got != "" {
		t.Fatalf("unexpected SINGLE_EMPTY: %q", got)
	}
}

func TestApplyPreservesExistingEnvironment(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	content := "YOUTRACK_URL=https://from-file.test\nYOUTRACK_API_TOKEN=perm:file-token\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	t.Setenv("YOUTRACK_URL", "https://from-env.test")

	if _, err := Apply(path, ApplyOptions{}); err != nil {
		t.Fatalf("apply env file: %v", err)
	}

	if got := os.Getenv("YOUTRACK_URL"); got != "https://from-env.test" {
		t.Fatalf("unexpected YOUTRACK_URL: %q", got)
	}
	if got := os.Getenv("YOUTRACK_API_TOKEN"); got != "perm:file-token" {
		t.Fatalf("unexpected YOUTRACK_API_TOKEN: %q", got)
	}
}

func TestApplyOverridesExistingEnvironmentWhenRequested(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	content := "YOUTRACK_URL=https://from-file.test\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	t.Setenv("YOUTRACK_URL", "https://from-env.test")

	if _, err := Apply(path, ApplyOptions{Override: true}); err != nil {
		t.Fatalf("apply env file: %v", err)
	}

	if got := os.Getenv("YOUTRACK_URL"); got != "https://from-file.test" {
		t.Fatalf("unexpected YOUTRACK_URL: %q", got)
	}
}

func TestLoadSupportsLargeSingleLineValues(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	largeValue := strings.Repeat("a", 70*1024)
	content := "YOUTRACK_API_TOKEN=" + largeValue + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	values, err := Load(path)
	if err != nil {
		t.Fatalf("load env file with large value: %v", err)
	}
	if got := values["YOUTRACK_API_TOKEN"]; got != largeValue {
		t.Fatalf("unexpected large env value length: got %d want %d", len(got), len(largeValue))
	}
}

func TestApplyDefaultIfPresentFallsBackToExecutableParent(t *testing.T) {
	cwd := t.TempDir()
	exeDir := t.TempDir()
	envPath := filepath.Join(exeDir, ".env")
	if err := os.WriteFile(envPath, []byte("YOUTRACK_URL=https://from-binary.test\n"), 0o644); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	originalGetWorkingDirectory := getWorkingDirectory
	originalGetExecutablePath := getExecutablePath
	t.Cleanup(func() {
		getWorkingDirectory = originalGetWorkingDirectory
		getExecutablePath = originalGetExecutablePath
	})

	getWorkingDirectory = func() (string, error) { return cwd, nil }
	getExecutablePath = func() (string, error) { return filepath.Join(exeDir, "youtrack-mcp"), nil }

	if err := os.Unsetenv("YOUTRACK_URL"); err != nil {
		t.Fatalf("unset env var: %v", err)
	}

	resolved, err := ApplyDefaultIfPresent(ApplyOptions{})
	if err != nil {
		t.Fatalf("apply default env file: %v", err)
	}
	if resolved != envPath {
		t.Fatalf("unexpected resolved env path: got %q want %q", resolved, envPath)
	}
	if got := os.Getenv("YOUTRACK_URL"); got != "https://from-binary.test" {
		t.Fatalf("unexpected YOUTRACK_URL: %q", got)
	}
}

func TestApplyDefaultIfPresentPrefersWorkingDirectory(t *testing.T) {
	cwd := t.TempDir()
	exeDir := t.TempDir()
	cwdEnvPath := filepath.Join(cwd, ".env")
	exeEnvPath := filepath.Join(exeDir, ".env")
	if err := os.WriteFile(cwdEnvPath, []byte("YOUTRACK_URL=https://from-cwd.test\n"), 0o644); err != nil {
		t.Fatalf("write cwd env file: %v", err)
	}
	if err := os.WriteFile(exeEnvPath, []byte("YOUTRACK_URL=https://from-binary.test\n"), 0o644); err != nil {
		t.Fatalf("write binary env file: %v", err)
	}

	originalGetWorkingDirectory := getWorkingDirectory
	originalGetExecutablePath := getExecutablePath
	t.Cleanup(func() {
		getWorkingDirectory = originalGetWorkingDirectory
		getExecutablePath = originalGetExecutablePath
	})

	getWorkingDirectory = func() (string, error) { return cwd, nil }
	getExecutablePath = func() (string, error) { return filepath.Join(exeDir, "youtrack-mcp"), nil }

	if err := os.Unsetenv("YOUTRACK_URL"); err != nil {
		t.Fatalf("unset env var: %v", err)
	}

	resolved, err := ApplyDefaultIfPresent(ApplyOptions{})
	if err != nil {
		t.Fatalf("apply default env file: %v", err)
	}
	if resolved != cwdEnvPath {
		t.Fatalf("unexpected resolved env path: got %q want %q", resolved, cwdEnvPath)
	}
	if got := os.Getenv("YOUTRACK_URL"); got != "https://from-cwd.test" {
		t.Fatalf("unexpected YOUTRACK_URL: %q", got)
	}
}
