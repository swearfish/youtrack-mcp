package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"youtrack-mcp/internal/config"
)

func setYouTrackTestEnv(t *testing.T, baseURL string) {
	t.Helper()
	t.Setenv(config.EnvYouTrackInsecure, "1")
	t.Setenv(config.EnvYouTrackURL, baseURL)
	t.Setenv(config.EnvYouTrackToken, "perm:test")
}

func TestStructuredFetchReturnsIssue(t *testing.T) {
	server := newFetchTestServer()
	defer server.Close()

	setYouTrackTestEnv(t, server.URL)

	result, payload, err := fetchYouTrackTicket(context.Background(), nil, fetchTicketArgs{
		Ticket: "YT-39",
	})
	if err != nil {
		t.Fatalf("fetch issue: %v", err)
	}
	if result != nil {
		t.Fatalf("expected no text result in structured mode, got %+v", result)
	}

	if payload.Ticket != "YT-39" || payload.Summary != "Update tickets" || payload.Status != "Open" {
		t.Fatalf("unexpected structured issue: %+v", payload)
	}
	if len(payload.CustomFields) != 2 || payload.CustomFields[1].Name != "Priority" || payload.CustomFields[1].Value != "Major" {
		t.Fatalf("expected structured custom fields in fetch payload, got %+v", payload.CustomFields)
	}
}

func TestSearchReturnsStructuredIssues(t *testing.T) {
	server := newFetchTestServer()
	defer server.Close()

	setYouTrackTestEnv(t, server.URL)

	result, payload, err := searchYouTrackTickets(context.Background(), nil, searchArgs{
		Query: "update",
		Limit: 5,
	})
	if err != nil {
		t.Fatalf("search issues: %v", err)
	}
	if result != nil {
		t.Fatalf("expected no text result for search, got %+v", result)
	}

	if payload.Query != "update" || len(payload.Issues) != 1 || payload.Issues[0].Ticket != "YT-39" {
		t.Fatalf("unexpected search results: %+v", payload)
	}
}

func TestMarkdownFetchReturnsText(t *testing.T) {
	server := newFetchTestServer()
	defer server.Close()

	setYouTrackTestEnv(t, server.URL)

	result, payload, err := fetchYouTrackTicketMarkdown(context.Background(), nil, fetchTicketMarkdownArgs{
		Ticket: "YT-39",
	})
	if err != nil {
		t.Fatalf("fetch markdown: %v", err)
	}
	if payload != (struct{}{}) {
		t.Fatalf("expected empty structured payload in markdown mode, got %#v", payload)
	}
	if result == nil || len(result.Content) != 1 {
		t.Fatalf("expected markdown text content, got %+v", result)
	}
	text, ok := result.Content[0].(*sdkmcp.TextContent)
	if !ok {
		t.Fatalf("expected text content, got %T", result.Content[0])
	}
	if text.Text == "" || text.Text[:1] != "#" {
		t.Fatalf("unexpected markdown output: %q", text.Text)
	}
}

func TestReadTicketResourceReturnsJSON(t *testing.T) {
	server := newFetchTestServer()
	defer server.Close()

	setYouTrackTestEnv(t, server.URL)

	result, err := readYouTrackTicketResource(context.Background(), &sdkmcp.ReadResourceRequest{
		Params: &sdkmcp.ReadResourceParams{URI: "youtrack://YT-39"},
	})
	if err != nil {
		t.Fatalf("read resource: %v", err)
	}
	if result == nil || len(result.Contents) != 1 {
		t.Fatalf("expected one resource content entry, got %+v", result)
	}
	content := result.Contents[0]
	if content.URI != "youtrack://YT-39" {
		t.Fatalf("unexpected resource URI: %+v", content)
	}
	if content.MIMEType != ticketResourceJSONMIMEType {
		t.Fatalf("unexpected resource MIME type: %+v", content)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(content.Text), &payload); err != nil {
		t.Fatalf("resource should contain JSON: %v", err)
	}
	if payload["ticket"] != "YT-39" || payload["summary"] != "Update tickets" {
		t.Fatalf("unexpected resource payload: %+v", payload)
	}
	attachments, ok := payload["attachments"].([]any)
	if !ok || len(attachments) != 2 {
		t.Fatalf("expected JSON resource attachments, got %+v", payload["attachments"])
	}
	firstAttachment, ok := attachments[0].(map[string]any)
	if !ok || firstAttachment["content"] != "hello from text attachment" {
		t.Fatalf("expected inline attachment content in JSON resource, got %+v", attachments[0])
	}
}

func TestReadTicketMarkdownResourceReturnsMarkdown(t *testing.T) {
	server := newFetchTestServer()
	defer server.Close()

	setYouTrackTestEnv(t, server.URL)

	result, err := readYouTrackTicketResource(context.Background(), &sdkmcp.ReadResourceRequest{
		Params: &sdkmcp.ReadResourceParams{URI: "youtrack://YT-39/markdown"},
	})
	if err != nil {
		t.Fatalf("read markdown resource: %v", err)
	}
	if result == nil || len(result.Contents) != 1 {
		t.Fatalf("expected one resource content entry, got %+v", result)
	}
	content := result.Contents[0]
	if content.URI != "youtrack://YT-39/markdown" {
		t.Fatalf("unexpected markdown resource URI: %+v", content)
	}
	if content.MIMEType != ticketResourceMarkdownMIMEType {
		t.Fatalf("unexpected markdown resource MIME type: %+v", content)
	}
	if !strings.HasPrefix(content.Text, "# YT-39: Update tickets") || !strings.Contains(content.Text, "hello from text attachment") {
		t.Fatalf("unexpected markdown resource content: %q", content.Text)
	}
}

func TestParseTicketResourceURI(t *testing.T) {
	t.Run("host form defaults to JSON", func(t *testing.T) {
		resource, err := parseTicketResourceURI("youtrack://YT-39")
		if err != nil {
			t.Fatalf("parse host form: %v", err)
		}
		if resource.Ticket != "YT-39" || resource.Format != ticketResourceFormatJSON {
			t.Fatalf("unexpected resource ref: %+v", resource)
		}
	})

	t.Run("host form markdown", func(t *testing.T) {
		resource, err := parseTicketResourceURI("youtrack://YT-39/markdown")
		if err != nil {
			t.Fatalf("parse markdown form: %v", err)
		}
		if resource.Ticket != "YT-39" || resource.Format != ticketResourceFormatMarkdown {
			t.Fatalf("unexpected resource ref: %+v", resource)
		}
	})

	t.Run("path form", func(t *testing.T) {
		resource, err := parseTicketResourceURI("youtrack:///YT-39/markdown")
		if err != nil {
			t.Fatalf("parse path form: %v", err)
		}
		if resource.Ticket != "YT-39" || resource.Format != ticketResourceFormatMarkdown {
			t.Fatalf("unexpected resource ref: %+v", resource)
		}
	})

	t.Run("invalid URI", func(t *testing.T) {
		if _, err := parseTicketResourceURI("https://example.com/YT-39"); err == nil {
			t.Fatalf("expected invalid resource URI to fail")
		}
	})

	t.Run("invalid suffix", func(t *testing.T) {
		if _, err := parseTicketResourceURI("youtrack://YT-39/json"); err == nil {
			t.Fatalf("expected invalid resource suffix to fail")
		}
	})
}

func TestStructuredFetchSupportsAttachments(t *testing.T) {
	server := newFetchTestServer()
	defer server.Close()

	setYouTrackTestEnv(t, server.URL)

	attachmentDir := t.TempDir()
	result, payload, err := fetchYouTrackTicket(context.Background(), nil, fetchTicketArgs{
		Ticket:         "YT-39",
		Attachments:    true,
		AttachmentPath: attachmentDir,
	})
	if err != nil {
		t.Fatalf("fetch structured issue with attachments: %v", err)
	}
	if result != nil {
		t.Fatalf("expected structured payload, got text result %+v", result)
	}

	if len(payload.Attachments) != 2 {
		t.Fatalf("expected two attachments, got %+v", payload.Attachments)
	}

	textAttachment := payload.Attachments[0]
	if textAttachment.Content != "hello from text attachment" {
		t.Fatalf("expected inlined text attachment content, got %+v", textAttachment)
	}
	if textAttachment.SavedPath == "" {
		t.Fatalf("expected saved path for text attachment, got %+v", textAttachment)
	}

	binaryAttachment := payload.Attachments[1]
	if binaryAttachment.Content != "" {
		t.Fatalf("expected binary attachment content to stay omitted, got %+v", binaryAttachment)
	}
	if binaryAttachment.SavedPath == "" {
		t.Fatalf("expected saved path for binary attachment, got %+v", binaryAttachment)
	}
	if _, err := os.Stat(binaryAttachment.SavedPath); err != nil {
		t.Fatalf("expected saved binary attachment on disk: %v", err)
	}
}

func TestResolveClientAndTicketRequiresConfiguredEnv(t *testing.T) {
	t.Setenv(config.EnvYouTrackURL, "")
	t.Setenv(config.EnvYouTrackToken, "")

	_, _, err := resolveClientAndTicket("YT-39")
	if err == nil {
		t.Fatalf("expected missing env to fail")
	}
	if !strings.Contains(err.Error(), "youtrack url not configured") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpdateRejectsMissingFieldsBeforeCallingYouTrack(t *testing.T) {
	server := newFetchTestServer()
	defer server.Close()

	setYouTrackTestEnv(t, server.URL)

	_, _, err := updateYouTrackTicket(context.Background(), nil, updateTicketArgs{
		Ticket: "YT-39",
	})
	if err == nil {
		t.Fatalf("expected empty update to fail")
	}
	if !strings.Contains(err.Error(), "provide at least one of summary, description, or custom_fields") {
		t.Fatalf("unexpected update validation error: %v", err)
	}
}

func TestResolveClientCachesByConfiguredTuple(t *testing.T) {
	server := newFetchTestServer()
	defer server.Close()

	clearClientCache()
	t.Cleanup(clearClientCache)
	setYouTrackTestEnv(t, server.URL)

	first, err := resolveClient()
	if err != nil {
		t.Fatalf("resolve first client: %v", err)
	}
	second, err := resolveClient()
	if err != nil {
		t.Fatalf("resolve second client: %v", err)
	}
	if first != second {
		t.Fatalf("expected cached client pointer reuse, got %p and %p", first, second)
	}
}

func TestToolNameUsesDefaultPrefix(t *testing.T) {
	original, hadOriginal := os.LookupEnv(config.EnvYouTrackToolPrefix)
	_ = os.Unsetenv(config.EnvYouTrackToolPrefix)
	defer func() {
		if hadOriginal {
			_ = os.Setenv(config.EnvYouTrackToolPrefix, original)
			return
		}
		_ = os.Unsetenv(config.EnvYouTrackToolPrefix)
	}()

	if got := toolName("fetch"); got != "youtrack_fetch" {
		t.Fatalf("expected default prefixed tool name, got %q", got)
	}
}

func TestToolNameAllowsEmptyPrefix(t *testing.T) {
	t.Setenv(config.EnvYouTrackToolPrefix, "")
	if got := toolName("fetch"); got != "fetch" {
		t.Fatalf("expected unprefixed tool name, got %q", got)
	}
}

func TestWriteToolAnnotationsMarkCreateAndLinkNonDestructive(t *testing.T) {
	createAnnotations := writeToolAnnotations(false, false)
	if createAnnotations.DestructiveHint == nil || *createAnnotations.DestructiveHint {
		t.Fatalf("expected create annotations to be non-destructive, got %+v", createAnnotations)
	}

	linkAnnotations := writeToolAnnotations(false, false)
	if linkAnnotations.DestructiveHint == nil || *linkAnnotations.DestructiveHint {
		t.Fatalf("expected link annotations to be non-destructive, got %+v", linkAnnotations)
	}

	updateAnnotations := writeToolAnnotations(true, true)
	if updateAnnotations.DestructiveHint == nil || !*updateAnnotations.DestructiveHint {
		t.Fatalf("expected update annotations to stay destructive, got %+v", updateAnnotations)
	}
}

func newFetchTestServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/issues" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]map[string]any{{
				"id":          "2-39",
				"idReadable":  "YT-39",
				"summary":     "Update tickets",
				"description": "Ticket description",
				"project":     map[string]any{"id": "0-1", "name": "YouTrack MCP", "shortName": "YT"},
				"customFields": []map[string]any{{
					"name":  "State",
					"$type": "StateIssueCustomField",
					"value": map[string]any{"name": "Open"},
				}, {
					"name":  "Priority",
					"$type": "EnumIssueCustomField",
					"value": map[string]any{"name": "Major"},
				}},
				"links":       []map[string]any{},
				"attachments": []map[string]any{},
			}})
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == "/api/issues/YT-39" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":          "2-39",
				"idReadable":  "YT-39",
				"summary":     "Update tickets",
				"description": "Ticket description",
				"project":     map[string]any{"id": "0-1", "name": "YouTrack MCP", "shortName": "YT"},
				"customFields": []map[string]any{{
					"name":  "State",
					"$type": "StateIssueCustomField",
					"value": map[string]any{"name": "Open"},
				}, {
					"name":  "Priority",
					"$type": "EnumIssueCustomField",
					"value": map[string]any{"name": "Major"},
				}},
				"links": []map[string]any{{
					"direction": "OUTWARD",
					"linkType": map[string]any{
						"name":           "relates to",
						"sourceToTarget": "relates to",
						"targetToSource": "is related to",
					},
					"issues": []map[string]any{{
						"idReadable": "YT-40",
						"summary":    "Linked ticket",
					}},
				}},
				"attachments": []map[string]any{
					{
						"name":     "story.txt",
						"url":      "/files/story.txt",
						"size":     26,
						"mimeType": "text/plain",
					},
					{
						"name":     "diagram.bin",
						"url":      "/files/diagram.bin",
						"size":     4,
						"mimeType": "application/octet-stream",
					},
				},
			})
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == "/files/story.txt" {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("hello from text attachment"))
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == "/files/diagram.bin" {
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write([]byte{0x00, 0x01, 0x02, 0x03})
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
}
