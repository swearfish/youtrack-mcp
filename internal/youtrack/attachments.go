package youtrack

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"
)

func (c *Client) buildAttachmentSections(ctx context.Context, attachments []map[string]any, includeContent bool, attachmentPath string) ([]string, error) {
	targetDir, err := resolveAttachmentDir(attachmentPath)
	if err != nil {
		return nil, err
	}

	sections := make([]string, 0, len(attachments))
	var totalDownloadedBytes int64
	totalInlineBytesRemaining := maxIssueInlineBytes
	for _, attachment := range attachments {
		name := textValue(attachment["name"])
		if name == "" {
			name = "attachment"
		}
		mimeType := textValue(attachment["mimeType"])
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}

		lines := []string{
			"### " + name,
			"- MIME type: " + mimeType,
		}
		if size := attachment["size"]; size != nil {
			lines = append(lines, fmt.Sprintf("- Size: %v bytes", size))
		}

		downloadURL := textValue(attachment["url"])
		if downloadURL == "" {
			lines = append(lines, "", "Attachment URL is not available.")
			sections = append(sections, strings.Join(lines, "\n"))
			continue
		}

		content, err := c.downloadAttachment(ctx, downloadURL)
		if err != nil {
			return nil, err
		}
		totalDownloadedBytes += int64(len(content))
		if totalDownloadedBytes > maxIssueAttachmentBytes {
			return nil, fmt.Errorf("attachment downloads exceed %d bytes for one issue", maxIssueAttachmentBytes)
		}

		if targetDir != "" {
			savedPath, err := storeAttachment(targetDir, name, content)
			if err != nil {
				return nil, err
			}
			lines = append(lines, "- Saved to: "+savedPath)
		}

		if includeContent {
			if decoded, note := inlineAttachmentText(content, attachment, &totalInlineBytesRemaining); decoded != "" {
				lines = append(lines, "", renderTextCodeBlock(decoded))
				if note != "" {
					lines = append(lines, note)
				}
			} else if note != "" {
				lines = append(lines, "", note)
			} else {
				lines = append(lines, "", "Binary attachment downloaded but content was omitted.")
			}
		}

		sections = append(sections, strings.Join(lines, "\n"))
	}

	return sections, nil
}

func (c *Client) buildStructuredAttachments(ctx context.Context, attachments []map[string]any, includeContent bool, attachmentPath string) ([]IssueAttachment, error) {
	targetDir, err := resolveAttachmentDir(attachmentPath)
	if err != nil {
		return nil, err
	}

	result := make([]IssueAttachment, 0, len(attachments))
	var totalDownloadedBytes int64
	totalInlineBytesRemaining := maxIssueInlineBytes
	for _, attachment := range attachments {
		entry := IssueAttachment{
			Name:        firstNonEmpty(textValue(attachment["name"]), "attachment"),
			MIMEType:    firstNonEmpty(textValue(attachment["mimeType"]), "application/octet-stream"),
			Size:        int64Value(attachment["size"]),
			DownloadURL: textValue(attachment["url"]),
		}

		needsDownload := entry.DownloadURL != "" && (targetDir != "" || (includeContent && isTextAttachment(attachment)))
		if needsDownload {
			content, err := c.downloadAttachment(ctx, entry.DownloadURL)
			if err != nil {
				return nil, err
			}
			totalDownloadedBytes += int64(len(content))
			if totalDownloadedBytes > maxIssueAttachmentBytes {
				return nil, fmt.Errorf("attachment downloads exceed %d bytes for one issue", maxIssueAttachmentBytes)
			}
			if targetDir != "" {
				savedPath, err := storeAttachment(targetDir, entry.Name, content)
				if err != nil {
					return nil, err
				}
				entry.SavedPath = savedPath
			}
			if includeContent {
				decoded, note := inlineAttachmentText(content, attachment, &totalInlineBytesRemaining)
				entry.Content = decoded
				if note != "" {
					if entry.Content != "" {
						entry.Content += "\n\n" + note
					} else {
						entry.Content = note
					}
				}
			}
		}

		result = append(result, entry)
	}

	return result, nil
}

func (c *Client) downloadAttachment(ctx context.Context, attachmentURL string) ([]byte, error) {
	requestURL, err := c.resolveAttachmentURL(attachmentURL)
	if err != nil {
		return nil, err
	}

	var lastErr error
	for attempt := 0; attempt < maxHTTPGetAttempts; attempt++ {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
		if err != nil {
			return nil, fmt.Errorf("build attachment request: %w", err)
		}
		if c.shouldAuthorizeAttachmentRequest(requestURL) {
			request.Header.Set("Authorization", "Bearer "+c.apiToken)
		}
		request.Header.Set("Accept", "*/*")

		response, err := c.httpClient.Do(request)
		if err != nil {
			lastErr = fmt.Errorf("download attachment: %w", err)
			if shouldRetryGET(attempt, 0, err) {
				if waitErr := waitForRetry(ctx, attempt); waitErr != nil {
					return nil, waitErr
				}
				continue
			}
			return nil, lastErr
		}

		body, readErr := readLimitedBody(response.Body, maxAttachmentBytes, "attachment response")
		response.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read attachment response: %w", readErr)
		}
		if response.StatusCode >= 400 {
			lastErr = fmt.Errorf("download attachment failed: %s", formatHTTPError(response.StatusCode, body))
			if shouldRetryGET(attempt, response.StatusCode, nil) {
				if waitErr := waitForRetry(ctx, attempt); waitErr != nil {
					return nil, waitErr
				}
				continue
			}
			return nil, lastErr
		}

		return body, nil
	}

	return nil, lastErr
}

func (c *Client) resolveAttachmentURL(attachmentURL string) (string, error) {
	base, err := url.Parse(c.baseURL)
	if err != nil {
		return "", fmt.Errorf("parse base attachment url: %w", err)
	}
	reference, err := url.Parse(strings.TrimSpace(attachmentURL))
	if err != nil {
		return "", fmt.Errorf("parse attachment url: %w", err)
	}
	resolved := base.ResolveReference(reference)
	if !strings.EqualFold(resolved.Scheme, "http") && !strings.EqualFold(resolved.Scheme, "https") {
		return "", fmt.Errorf("attachment url uses unsupported scheme %q", resolved.Scheme)
	}
	return resolved.String(), nil
}

func (c *Client) shouldAuthorizeAttachmentRequest(requestURL string) bool {
	base, err := url.Parse(c.baseURL)
	if err != nil {
		return false
	}
	target, err := url.Parse(requestURL)
	if err != nil {
		return false
	}
	return strings.EqualFold(base.Scheme, target.Scheme) && strings.EqualFold(base.Host, target.Host)
}

func renderTextCodeBlock(content string) string {
	fenceLength := longestBacktickRun(content) + 1
	if fenceLength < 3 {
		fenceLength = 3
	}
	fence := strings.Repeat("`", fenceLength)
	return fence + "text\n" + content + "\n" + fence
}

func longestBacktickRun(content string) int {
	longest := 0
	current := 0
	for _, r := range content {
		if r == '`' {
			current++
			if current > longest {
				longest = current
			}
			continue
		}
		current = 0
	}
	return longest
}

func resolveAttachmentDir(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", nil
	}

	resolved, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve attachment path: %w", err)
	}
	if err := os.MkdirAll(resolved, 0o755); err != nil {
		return "", fmt.Errorf("create attachment directory: %w", err)
	}
	return resolved, nil
}

func storeAttachment(targetDir string, attachmentName string, content []byte) (string, error) {
	base, err := sanitizeAttachmentName(attachmentName)
	if err != nil {
		return "", fmt.Errorf("sanitize attachment name: %w", err)
	}

	destination := filepath.Join(targetDir, base)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	index := 1
	for {
		file, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err == nil {
			if _, writeErr := file.Write(content); writeErr != nil {
				file.Close()
				_ = os.Remove(destination)
				return "", fmt.Errorf("write attachment: %w", writeErr)
			}
			if closeErr := file.Close(); closeErr != nil {
				_ = os.Remove(destination)
				return "", fmt.Errorf("close attachment file: %w", closeErr)
			}
			return destination, nil
		}
		if !os.IsExist(err) {
			return "", fmt.Errorf("create attachment file: %w", err)
		}
		destination = filepath.Join(targetDir, fmt.Sprintf("%s-%d%s", stem, index, ext))
		index++
	}
}

func sanitizeAttachmentName(name string) (string, error) {
	base := filepath.Base(strings.TrimSpace(name))
	if strings.ContainsRune(base, '\x00') {
		return "", fmt.Errorf("attachment name contains NUL byte")
	}
	if base == "." || base == ".." || base == string(filepath.Separator) || base == "" {
		return "attachment", nil
	}

	var builder strings.Builder
	builder.Grow(len(base))
	for _, r := range base {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			builder.WriteRune(r)
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '.' || r == '_' || r == '-':
			builder.WriteRune(r)
		default:
			builder.WriteByte('-')
		}
	}

	sanitized := strings.Trim(builder.String(), ".")
	if sanitized == "" || sanitized == "." || sanitized == ".." {
		sanitized = "attachment"
	}

	ext := filepath.Ext(sanitized)
	stem := strings.TrimSuffix(sanitized, ext)
	if stem == "" {
		stem = "attachment"
		ext = ""
	}
	if _, reserved := reservedWindowsFileNames[strings.ToLower(stem)]; reserved {
		stem = "attachment-" + stem
	}

	if ext != "" && len(ext) >= maxAttachmentNameLength {
		ext = ext[:maxAttachmentNameLength/2]
	}
	if len(stem)+len(ext) > maxAttachmentNameLength {
		maxStemLength := maxAttachmentNameLength - len(ext)
		if maxStemLength <= 0 {
			stem = "attachment"
			ext = ""
		} else {
			stem = stem[:maxStemLength]
		}
	}

	sanitized = stem + ext
	if sanitized == "" {
		return "attachment", nil
	}
	return sanitized, nil
}

func decodeAttachmentText(content []byte, attachment map[string]any) string {
	if !isTextAttachment(attachment) {
		return ""
	}
	return strings.ToValidUTF8(string(content), "")
}

func inlineAttachmentText(content []byte, attachment map[string]any, remaining *int64) (string, string) {
	decoded := decodeAttachmentText(content, attachment)
	if decoded == "" {
		return "", ""
	}
	if remaining != nil && *remaining <= 0 {
		return "", "Text attachment content was omitted because the inline attachment budget was exhausted."
	}

	limit := maxInlineAttachmentBytes
	if remaining != nil && *remaining < limit {
		limit = *remaining
	}
	if limit <= 0 {
		return "", "Text attachment content was omitted because the inline attachment budget was exhausted."
	}

	truncated := truncateUTF8(decoded, limit)
	if remaining != nil {
		*remaining -= int64(len(truncated))
	}

	if len(truncated) < len(decoded) {
		return truncated, fmt.Sprintf("Text attachment content was truncated to %d bytes. Use attachment_path to save the full file.", limit)
	}
	return truncated, ""
}

func truncateUTF8(text string, limit int64) string {
	if int64(len(text)) <= limit {
		return text
	}
	truncated := text[:limit]
	for !utf8.ValidString(truncated) && len(truncated) > 0 {
		truncated = truncated[:len(truncated)-1]
	}
	return truncated
}

func isTextAttachment(attachment map[string]any) bool {
	mimeType := strings.ToLower(textValue(attachment["mimeType"]))
	if strings.HasPrefix(mimeType, "text/") || slices.Contains(textAttachmentTypes, mimeType) {
		return true
	}

	name := strings.ToLower(textValue(attachment["name"]))
	for _, extension := range textAttachmentExtensions {
		if strings.HasSuffix(name, extension) {
			return true
		}
	}
	return false
}
