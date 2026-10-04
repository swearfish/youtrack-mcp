package youtrack

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPostComment(t *testing.T) {
	const commentText = "  First line\nSecond line  "
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != http.MethodPost || r.URL.Path != "/api/issues/YT-39/comments" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.URL.Query().Get("fields"); got != "id,text" {
			t.Errorf("unexpected fields: %q", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer perm:test" {
			t.Errorf("unexpected authorization header: %q", got)
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if len(payload) != 1 || payload["text"] != commentText {
			t.Errorf("unexpected payload: %+v", payload)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "4-14", "text": commentText})
	}))
	defer server.Close()

	client := newHTTPTestClient(t, server.URL, server.Client())
	comment, err := client.PostComment(context.Background(), "YT-39", commentText)
	if err != nil {
		t.Fatalf("post comment: %v", err)
	}
	if comment.ID != "4-14" || comment.Ticket != "YT-39" || comment.Text != commentText {
		t.Fatalf("unexpected comment result: %+v", comment)
	}
	if requests != 1 {
		t.Fatalf("expected one POST request, got %d", requests)
	}
}

func TestPostCommentRejectsBlankText(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
	}))
	defer server.Close()

	client := newHTTPTestClient(t, server.URL, server.Client())
	_, err := client.PostComment(context.Background(), "YT-39", " \n\t ")
	if err == nil || !strings.Contains(err.Error(), "comment text is required") {
		t.Fatalf("expected blank comment error, got %v", err)
	}
	if requests != 0 {
		t.Fatalf("expected no request, got %d", requests)
	}
}

func TestPostCommentSurfacesYouTrackErrorWithoutRetry(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("Create Comment permission required"))
	}))
	defer server.Close()

	client := newHTTPTestClient(t, server.URL, server.Client())
	_, err := client.PostComment(context.Background(), "YT-39", "Hello")
	if err == nil || !strings.Contains(err.Error(), "status 403") || !strings.Contains(err.Error(), "Create Comment permission required") {
		t.Fatalf("expected YouTrack permission error, got %v", err)
	}
	if requests != 1 {
		t.Fatalf("expected no POST retry, got %d requests", requests)
	}
}
