package config

import "testing"

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
