package youtrack

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	"youtrack-mcp/internal/config"
)

func NewClient(baseURL string, apiToken string, httpClient *http.Client) (*Client, error) {
	trimmedURL := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if trimmedURL == "" {
		return nil, fmt.Errorf("youtrack url is required")
	}
	if err := validateYouTrackURL(trimmedURL, allowInsecureYouTrack()); err != nil {
		return nil, err
	}
	if strings.TrimSpace(apiToken) == "" {
		return nil, fmt.Errorf("youtrack api token is required")
	}
	if httpClient == nil {
		timeout, err := config.RequestTimeout()
		if err != nil {
			return nil, err
		}
		httpClient = &http.Client{Timeout: timeout}
	}
	return &Client{
		baseURL:    trimmedURL,
		apiToken:   strings.TrimSpace(apiToken),
		httpClient: httpClient,
	}, nil
}

func validateYouTrackURL(baseURL string, allowInsecure bool) error {
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return fmt.Errorf("parse youtrack url: %w", err)
	}
	if strings.EqualFold(parsedURL.Scheme, "http") && !allowInsecure {
		return fmt.Errorf("refusing insecure youtrack url %q; set %s=1 to allow http", baseURL, config.EnvYouTrackInsecure)
	}
	return nil
}

func allowInsecureYouTrack() bool {
	value := strings.TrimSpace(os.Getenv(config.EnvYouTrackInsecure))
	switch strings.ToLower(value) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}
