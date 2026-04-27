package config

import (
	"fmt"
	"os"
	"strings"
	"time"
)

const (
	ServerName             = "youtrack-mcp"
	EnvYouTrackURL         = "YOUTRACK_URL"
	EnvYouTrackToken       = "YOUTRACK_API_TOKEN"
	EnvYouTrackHTTPTimeout = "YOUTRACK_HTTP_TIMEOUT"

	EnvVSCodeMCPConfigPath  = "VSCODE_MCP_CONFIG_PATH"
	EnvCopilotMCPConfigPath = "COPILOT_MCP_CONFIG_PATH"
)

const DefaultRequestTimeout = 30 * time.Second

func RequestTimeout() (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv(EnvYouTrackHTTPTimeout))
	if raw == "" {
		return DefaultRequestTimeout, nil
	}

	timeout, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", EnvYouTrackHTTPTimeout, err)
	}
	if timeout <= 0 {
		return 0, fmt.Errorf("%s must be greater than zero", EnvYouTrackHTTPTimeout)
	}
	return timeout, nil
}
