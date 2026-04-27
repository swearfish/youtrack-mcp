package envfile

import (
	"os"
	"path/filepath"
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
