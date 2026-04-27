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
	"youtrack-mcp/internal/youtrack"
)

func init() {
	_ = os.Setenv(config.EnvYouTrackInsecure, "1")
}

func TestFetchReturnsStructuredIssueByDefault(t *testing.T) {
	server := newFetchTestServer()
	defer server.Close()

	t.Setenv(config.EnvYouTrackURL, server.URL)
	t.Setenv(config.EnvYouTrackToken, "perm:test")

	result, payload, err := fetchYouTrackUserStory(context.Background(), nil, fetchStoryArgs{
		Ticket: "YT-39",
	})
	if err != nil {
		t.Fatalf("fetch issue: %v", err)
	}
	if result != nil {
		t.Fatalf("expected no text result in structured mode, got %+v", result)
	}

	issue, ok := payload.(youtrack.Issue)
	if !ok {
		t.Fatalf("expected structured issue payload, got %T", payload)
	}
	if issue.Ticket != "YT-39" || issue.Summary != "Update tickets" || issue.Status != "Open" {
		t.Fatalf("unexpected structured issue: %+v", issue)
	}
}

func TestSearchReturnsStructuredIssues(t *testing.T) {
	server := newFetchTestServer()
	defer server.Close()

	t.Setenv(config.EnvYouTrackURL, server.URL)
	t.Setenv(config.EnvYouTrackToken, "perm:test")

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

func TestFetchReturnsMarkdownWhenRequested(t *testing.T) {
	server := newFetchTestServer()
	defer server.Close()

	t.Setenv(config.EnvYouTrackURL, server.URL)
	t.Setenv(config.EnvYouTrackToken, "perm:test")

	result, payload, err := fetchYouTrackUserStory(context.Background(), nil, fetchStoryArgs{
		Ticket:   "YT-39",
		Markdown: true,
	})
	if err != nil {
		t.Fatalf("fetch markdown: %v", err)
	}
	if payload != nil {
		t.Fatalf("expected no structured payload in markdown mode, got %T", payload)
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

func TestFetchStructuredModeSupportsAttachments(t *testing.T) {
	server := newFetchTestServer()
	defer server.Close()

	t.Setenv(config.EnvYouTrackURL, server.URL)
	t.Setenv(config.EnvYouTrackToken, "perm:test")

	attachmentDir := t.TempDir()
	result, payload, err := fetchYouTrackUserStory(context.Background(), nil, fetchStoryArgs{
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

	issue, ok := payload.(youtrack.Issue)
	if !ok {
		t.Fatalf("expected structured issue payload, got %T", payload)
	}
	if len(issue.Attachments) != 2 {
		t.Fatalf("expected two attachments, got %+v", issue.Attachments)
	}

	textAttachment := issue.Attachments[0]
	if textAttachment.Content != "hello from text attachment" {
		t.Fatalf("expected inlined text attachment content, got %+v", textAttachment)
	}
	if textAttachment.SavedPath == "" {
		t.Fatalf("expected saved path for text attachment, got %+v", textAttachment)
	}

	binaryAttachment := issue.Attachments[1]
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
