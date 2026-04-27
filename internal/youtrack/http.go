package youtrack

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func (c *Client) doJSON(ctx context.Context, method string, requestPath string, query map[string]string, payload any, out any) error {
	endpoint, err := c.resolveEndpoint(requestPath)
	if err != nil {
		return fmt.Errorf("build request url: %w", err)
	}
	if len(query) > 0 {
		values := endpoint.Query()
		for key, value := range query {
			values.Set(key, value)
		}
		endpoint.RawQuery = values.Encode()
	}

	var bodyReader io.Reader
	var data []byte
	if payload != nil {
		data, err = json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("marshal request payload: %w", err)
		}
	}

	attempts := 1
	if method == http.MethodGet {
		attempts = maxHTTPGetAttempts
	}

	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		if data != nil {
			bodyReader = bytes.NewReader(data)
		} else {
			bodyReader = nil
		}

		request, err := http.NewRequestWithContext(ctx, method, endpoint.String(), bodyReader)
		if err != nil {
			return fmt.Errorf("build request: %w", err)
		}
		request.Header.Set("Authorization", "Bearer "+c.apiToken)
		request.Header.Set("Accept", "application/json")
		if payload != nil {
			request.Header.Set("Content-Type", "application/json")
		}

		response, err := c.httpClient.Do(request)
		if err != nil {
			lastErr = fmt.Errorf("perform request: %w", err)
			if shouldRetryGET(attempt, 0, err) {
				logDebug(ctx, "Retrying YouTrack request after transport error", "method", method, "url", endpoint.String(), "attempt", attempt+1, "error", err)
				if waitErr := waitForRetry(ctx, attempt); waitErr != nil {
					return waitErr
				}
				continue
			}
			logWarn(ctx, "YouTrack request failed", "method", method, "url", endpoint.String(), "error", lastErr)
			return lastErr
		}

		body, readErr := readLimitedBody(response.Body, maxJSONResponseBytes, "JSON response")
		response.Body.Close()
		if readErr != nil {
			return fmt.Errorf("read response: %w", readErr)
		}
		if response.StatusCode >= 400 {
			lastErr = fmt.Errorf("youtrack request failed: %s", formatHTTPError(response.StatusCode, body))
			if shouldRetryGET(attempt, response.StatusCode, nil) {
				logDebug(ctx, "Retrying YouTrack request after upstream error", "method", method, "url", endpoint.String(), "attempt", attempt+1, "status", response.StatusCode)
				if waitErr := waitForRetry(ctx, attempt); waitErr != nil {
					return waitErr
				}
				continue
			}
			logWarn(ctx, "YouTrack request failed", "method", method, "url", endpoint.String(), "status", response.StatusCode, "error", lastErr)
			return lastErr
		}
		if out == nil || len(body) == 0 {
			return nil
		}
		if err := json.Unmarshal(body, out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
		return nil
	}

	return lastErr
}

func (c *Client) resolveEndpoint(requestPath string) (*url.URL, error) {
	if c.parsedBase == nil {
		return nil, fmt.Errorf("missing parsed base url")
	}
	endpoint := c.parsedBase.JoinPath(requestPath)
	if strings.Contains(requestPath, "?") {
		pathOnly, rawQuery, _ := strings.Cut(requestPath, "?")
		endpoint = c.parsedBase.JoinPath(pathOnly)
		endpoint.RawQuery = rawQuery
	}
	return endpoint, nil
}

func shouldRetryGET(attempt int, statusCode int, err error) bool {
	if attempt >= maxHTTPGetAttempts-1 {
		return false
	}
	if err != nil {
		return true
	}
	switch statusCode {
	case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

func waitForRetry(ctx context.Context, attempt int) error {
	baseDelay := time.Duration(attempt+1) * 50 * time.Millisecond
	jitter := time.Duration(rand.Int63n(int64(25 * time.Millisecond)))
	timer := time.NewTimer(baseDelay + jitter)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func readLimitedBody(reader io.Reader, limit int64, label string) ([]byte, error) {
	limited := io.LimitReader(reader, limit+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("%s exceeds %d bytes", label, limit)
	}
	return body, nil
}

func formatHTTPError(statusCode int, body []byte) string {
	message := strings.TrimSpace(string(body))
	if message == "" {
		return fmt.Sprintf("status %d", statusCode)
	}
	return fmt.Sprintf("status %d: %s", statusCode, message)
}
