package config

import (
	"os"
	"testing"
)

func TestRequestTimeoutDefault(t *testing.T) {
	t.Setenv(EnvYouTrackHTTPTimeout, "")

	timeout, err := RequestTimeout()
	if err != nil {
		t.Fatalf("request timeout: %v", err)
	}
	if timeout != DefaultRequestTimeout {
		t.Fatalf("expected default timeout %v, got %v", DefaultRequestTimeout, timeout)
	}
}

func TestRequestTimeoutFromEnv(t *testing.T) {
	t.Setenv(EnvYouTrackHTTPTimeout, "45s")

	timeout, err := RequestTimeout()
	if err != nil {
		t.Fatalf("request timeout: %v", err)
	}
	if timeout.String() != "45s" {
		t.Fatalf("expected 45s timeout, got %v", timeout)
	}
}

func TestRequestTimeoutRejectsInvalidValue(t *testing.T) {
	t.Setenv(EnvYouTrackHTTPTimeout, "not-a-duration")

	_, err := RequestTimeout()
	if err == nil {
		t.Fatalf("expected invalid timeout to fail")
	}
}

func TestToolPrefixDefault(t *testing.T) {
	t.Setenv(EnvYouTrackToolPrefix, "")
	if got := ToolPrefix(); got != "" {
		t.Fatalf("expected explicit empty prefix to be preserved, got %q", got)
	}
}

func TestToolPrefixUsesDefaultWhenUnset(t *testing.T) {
	original, hadOriginal := os.LookupEnv(EnvYouTrackToolPrefix)
	_ = os.Unsetenv(EnvYouTrackToolPrefix)
	defer func() {
		if hadOriginal {
			_ = os.Setenv(EnvYouTrackToolPrefix, original)
			return
		}
		_ = os.Unsetenv(EnvYouTrackToolPrefix)
	}()

	if got := ToolPrefix(); got != DefaultToolPrefix {
		t.Fatalf("expected default prefix %q, got %q", DefaultToolPrefix, got)
	}
}

func TestToolPrefixFromEnv(t *testing.T) {
	t.Setenv(EnvYouTrackToolPrefix, "acme_")
	if got := ToolPrefix(); got != "acme_" {
		t.Fatalf("expected custom prefix, got %q", got)
	}
}
