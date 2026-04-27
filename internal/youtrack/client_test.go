package youtrack

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClientOperations(t *testing.T) {
	t.Parallel()

	state := newFakeState()
	server := httptest.NewServer(http.HandlerFunc(state.handle))
	defer server.Close()

	client, err := NewClient(server.URL, "perm:test", server.Client())
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	ctx := context.Background()

	story, err := client.FetchStory(ctx, "YT-39", false, "")
	if err != nil {
		t.Fatalf("fetch story: %v", err)
	}
	if !strings.Contains(story, "# YT-39: Update tickets") || !strings.Contains(story, "## Status") {
		t.Fatalf("unexpected story output: %s", story)
	}

	statuses, err := client.GetTicketStatuses(ctx, "YT-39")
	if err != nil {
		t.Fatalf("get statuses: %v", err)
	}
	if len(statuses.Statuses) != 3 || statuses.CurrentStatus != "Open" {
		t.Fatalf("unexpected statuses: %+v", statuses)
	}

	statusUpdate, err := client.UpdateTicketStatus(ctx, "YT-39", "In Progress")
	if err != nil {
		t.Fatalf("update status: %v", err)
	}
	if statusUpdate.Status != "In Progress" || statusUpdate.PreviousStatus != "Open" {
		t.Fatalf("unexpected status update: %+v", statusUpdate)
	}

	created, err := client.CreateTicket(ctx, "YT", "New ticket", "desc", map[string]any{
		"name":  "Priority",
		"value": map[string]any{"name": "Major"},
	})
	if err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	if created.Ticket == "" || created.Summary != "New ticket" {
		t.Fatalf("unexpected created ticket: %+v", created)
	}

	updatedSummary := "Updated summary"
	updatedDescription := "updated desc"
	updated, err := client.UpdateTicket(ctx, created.Ticket, &updatedSummary, &updatedDescription, "")
	if err != nil {
		t.Fatalf("update ticket: %v", err)
	}
	if updated.Summary != "Updated summary" || updated.Description != "updated desc" {
		t.Fatalf("unexpected updated ticket: %+v", updated)
	}

	clearedDescription := ""
	cleared, err := client.UpdateTicket(ctx, created.Ticket, nil, &clearedDescription, nil)
	if err != nil {
		t.Fatalf("clear description: %v", err)
	}
	if cleared.Description != "" {
		t.Fatalf("expected cleared description, got %+v", cleared)
	}

	linkResult, err := client.LinkTickets(ctx, "YT-39", created.Ticket, "relates to")
	if err != nil {
		t.Fatalf("link tickets: %v", err)
	}
	if linkResult.LinkedTicket != created.Ticket {
		t.Fatalf("unexpected link result: %+v", linkResult)
	}

	unlinkResult, err := client.UnlinkTickets(ctx, "YT-39", created.Ticket, "")
	if err != nil {
		t.Fatalf("unlink tickets: %v", err)
	}
	if unlinkResult.UnlinkedTicket != created.Ticket {
		t.Fatalf("unexpected unlink result: %+v", unlinkResult)
	}
}

func TestUpdateTicketStatusQuotesPunctuatedStatus(t *testing.T) {
	t.Parallel()

	state := newFakeState()
	state.issues["YT-39"]["available_statuses"] = []string{"Open", "Done, verified"}

	server := httptest.NewServer(http.HandlerFunc(state.handle))
	defer server.Close()

	client, err := NewClient(server.URL, "perm:test", server.Client())
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	statusUpdate, err := client.UpdateTicketStatus(context.Background(), "YT-39", "Done, verified")
	if err != nil {
		t.Fatalf("update status: %v", err)
	}
	if statusUpdate.Status != "Done, verified" {
		t.Fatalf("expected punctuated status to survive quoting, got %+v", statusUpdate)
	}
}

func TestLinkTicketsQuotesPunctuatedRelation(t *testing.T) {
	t.Parallel()

	state := newFakeState()
	state.issues["YT-40"] = map[string]any{
		"id":                 "2-40",
		"idReadable":         "YT-40",
		"summary":            "Linked issue",
		"description":        "Linked ticket description",
		"project":            state.project,
		"status_field_name":  "State",
		"status":             "Open",
		"available_statuses": []string{"Open"},
		"customFields":       []map[string]any{},
		"links":              []map[string]any{},
		"attachments":        []map[string]any{},
	}

	server := httptest.NewServer(http.HandlerFunc(state.handle))
	defer server.Close()

	client, err := NewClient(server.URL, "perm:test", server.Client())
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	linkResult, err := client.LinkTickets(context.Background(), "YT-39", "YT-40", "depends on, maybe")
	if err != nil {
		t.Fatalf("link tickets: %v", err)
	}
	if linkResult.Relation != "depends on, maybe" {
		t.Fatalf("expected relation to survive quoting, got %+v", linkResult)
	}

	links := state.issues["YT-39"]["links"].([]map[string]any)
	if len(links) != 1 {
		t.Fatalf("expected one link, got %d", len(links))
	}
	if got := links[0]["linkType"].(map[string]any)["name"]; got != "depends on, maybe" {
		t.Fatalf("expected stored relation to preserve punctuation, got %v", got)
	}
}

func TestFetchStoryIncludesDownloadedAttachments(t *testing.T) {
	t.Parallel()

	state := newFakeState()
	state.issues["YT-39"]["attachments"] = []map[string]any{
		{"name": "notes.txt", "url": "/files/notes.txt", "size": 13, "mimeType": "text/plain"},
		{"name": "diagram.bin", "url": "/files/diagram.bin", "size": 4, "mimeType": "application/octet-stream"},
	}
	state.attachmentBodies["/files/notes.txt"] = []byte("hello, world!")
	state.attachmentBodies["/files/diagram.bin"] = []byte{0x00, 0x01, 0x02, 0x03}

	server := httptest.NewServer(http.HandlerFunc(state.handle))
	defer server.Close()

	client, err := NewClient(server.URL, "perm:test", server.Client())
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	dir := t.TempDir()
	story, err := client.FetchStory(context.Background(), "YT-39", true, dir)
	if err != nil {
		t.Fatalf("fetch story with attachments: %v", err)
	}
	if !strings.Contains(story, "hello, world!") {
		t.Fatalf("expected text attachment content in story, got %s", story)
	}
	if !strings.Contains(story, "Binary attachment downloaded but content was omitted.") {
		t.Fatalf("expected binary attachment note in story, got %s", story)
	}
	if _, err := os.Stat(filepath.Join(dir, "notes.txt")); err != nil {
		t.Fatalf("expected text attachment to be written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "diagram.bin")); err != nil {
		t.Fatalf("expected binary attachment to be written: %v", err)
	}
}

func TestFetchTicketIncludesStructuredAttachments(t *testing.T) {
	t.Parallel()

	state := newFakeState()
	state.issues["YT-39"]["attachments"] = []map[string]any{
		{"name": "notes.txt", "url": "/files/notes.txt", "size": 13, "mimeType": "text/plain"},
		{"name": "diagram.bin", "url": "/files/diagram.bin", "size": 4, "mimeType": "application/octet-stream"},
	}
	state.attachmentBodies["/files/notes.txt"] = []byte("hello, world!")
	state.attachmentBodies["/files/diagram.bin"] = []byte{0x00, 0x01, 0x02, 0x03}

	server := httptest.NewServer(http.HandlerFunc(state.handle))
	defer server.Close()

	client, err := NewClient(server.URL, "perm:test", server.Client())
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	dir := t.TempDir()
	issue, err := client.FetchTicket(context.Background(), "YT-39", true, dir)
	if err != nil {
		t.Fatalf("fetch structured ticket with attachments: %v", err)
	}
	if len(issue.Attachments) != 2 {
		t.Fatalf("expected two attachments, got %+v", issue.Attachments)
	}
	if issue.Attachments[0].Content != "hello, world!" {
		t.Fatalf("expected inlined text content, got %+v", issue.Attachments[0])
	}
	if issue.Attachments[1].Content != "" {
		t.Fatalf("expected binary content to be omitted, got %+v", issue.Attachments[1])
	}
	if issue.Attachments[1].SavedPath == "" {
		t.Fatalf("expected binary attachment saved path, got %+v", issue.Attachments[1])
	}
}

func TestFetchStoryAttachmentDownloadFailure(t *testing.T) {
	t.Parallel()

	state := newFakeState()
	state.issues["YT-39"]["attachments"] = []map[string]any{
		{"name": "notes.txt", "url": "/files/notes.txt", "size": 13, "mimeType": "text/plain"},
	}
	state.attachmentStatus["/files/notes.txt"] = http.StatusInternalServerError

	server := httptest.NewServer(http.HandlerFunc(state.handle))
	defer server.Close()

	client, err := NewClient(server.URL, "perm:test", server.Client())
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	_, err = client.FetchStory(context.Background(), "YT-39", true, "")
	if err == nil {
		t.Fatalf("expected attachment download failure")
	}
	if !strings.Contains(err.Error(), "download attachment failed") {
		t.Fatalf("unexpected attachment error: %v", err)
	}
}

func TestFetchTicketReturnsNotFoundError(t *testing.T) {
	t.Parallel()

	state := newFakeState()
	server := httptest.NewServer(http.HandlerFunc(state.handle))
	defer server.Close()

	client, err := NewClient(server.URL, "perm:test", server.Client())
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	_, err = client.FetchTicket(context.Background(), "YT-404", false, "")
	if err == nil {
		t.Fatalf("expected missing ticket to fail")
	}
	if !strings.Contains(err.Error(), "youtrack request failed") {
		t.Fatalf("unexpected missing ticket error: %v", err)
	}
}

func TestCreateTicketRejectsMalformedCustomFields(t *testing.T) {
	t.Parallel()

	state := newFakeState()
	server := httptest.NewServer(http.HandlerFunc(state.handle))
	defer server.Close()

	client, err := NewClient(server.URL, "perm:test", server.Client())
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	_, err = client.CreateTicket(context.Background(), "YT", "Bad ticket", "desc", "{not valid json}")
	if err == nil {
		t.Fatalf("expected malformed custom_fields to fail")
	}
	if !strings.Contains(err.Error(), "custom_fields is not valid JSON") {
		t.Fatalf("unexpected custom_fields error: %v", err)
	}
}

func TestFetchTicketRetriesTransientGetFailure(t *testing.T) {
	t.Parallel()

	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/issues/YT-39" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte("temporary upstream error"))
			return
		}

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
			"links":       []map[string]any{},
			"attachments": []map[string]any{},
		})
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "perm:test", server.Client())
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	issue, err := client.FetchTicket(context.Background(), "YT-39", false, "")
	if err != nil {
		t.Fatalf("fetch ticket with retry: %v", err)
	}
	if issue.Ticket != "YT-39" {
		t.Fatalf("unexpected issue after retry: %+v", issue)
	}
	if attempts != 2 {
		t.Fatalf("expected exactly 2 attempts, got %d", attempts)
	}
}

func TestRenderTextCodeBlockExpandsFenceWhenNeeded(t *testing.T) {
	t.Parallel()

	rendered := renderTextCodeBlock("before ``` after")
	if !strings.Contains(rendered, "````text\nbefore ``` after\n````") {
		t.Fatalf("expected expanded fence, got %q", rendered)
	}
}

type fakeState struct {
	project          map[string]any
	projects         []map[string]any
	nextIssueNumber  int
	issues           map[string]map[string]any
	attachmentBodies map[string][]byte
	attachmentStatus map[string]int
}

func newFakeState() *fakeState {
	project := map[string]any{"id": "0-1", "name": "YouTrack MCP", "shortName": "YT"}
	return &fakeState{
		project:          project,
		projects:         []map[string]any{project},
		nextIssueNumber:  100,
		attachmentBodies: map[string][]byte{},
		attachmentStatus: map[string]int{},
		issues: map[string]map[string]any{
			"YT-39": {
				"id":                 "2-39",
				"idReadable":         "YT-39",
				"summary":            "Update tickets",
				"description":        "Ticket description",
				"project":            map[string]any{"id": "0-1", "name": "YouTrack MCP", "shortName": "YT"},
				"status_field_name":  "State",
				"status":             "Open",
				"available_statuses": []string{"Open", "In Progress", "Fixed"},
				"customFields":       []map[string]any{},
				"links":              []map[string]any{},
				"attachments":        []map[string]any{},
			},
		},
	}
}

func (s *fakeState) handle(w http.ResponseWriter, r *http.Request) {
	writeJSON := func(status int, payload any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(payload)
	}

	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/api/admin/projects":
		query := strings.ToLower(r.URL.Query().Get("query"))
		projects := make([]map[string]any, 0, len(s.projects))
		for _, project := range s.projects {
			if query == "" ||
				strings.Contains(strings.ToLower(anyString(project["id"])), query) ||
				strings.Contains(strings.ToLower(anyString(project["shortName"])), query) ||
				strings.Contains(strings.ToLower(anyString(project["name"])), query) {
				projects = append(projects, project)
			}
		}
		writeJSON(http.StatusOK, projects)
		return

	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/issues/") && strings.HasSuffix(r.URL.Path, "/links"):
		ticket := strings.Split(r.URL.Path, "/")[3]
		issue := s.issues[ticket]
		if issue == nil {
			writeJSON(http.StatusNotFound, map[string]any{"error": "not found"})
			return
		}
		writeJSON(http.StatusOK, issue["links"])
		return

	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/issues/"):
		ticket := strings.Split(r.URL.Path, "/")[3]
		issue := s.issues[ticket]
		if issue == nil {
			writeJSON(http.StatusNotFound, map[string]any{"error": "not found"})
			return
		}
		writeJSON(http.StatusOK, serializeFakeIssue(issue))
		return

	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/files/"):
		if status := s.attachmentStatus[r.URL.Path]; status != 0 {
			w.WriteHeader(status)
			_, _ = w.Write([]byte("attachment failed"))
			return
		}
		body, ok := s.attachmentBodies[r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("not found"))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
		return

	case r.Method == http.MethodPost && r.URL.Path == "/api/issues":
		var payload map[string]any
		_ = json.NewDecoder(r.Body).Decode(&payload)
		ticket := fmt.Sprintf("YT-%d", s.nextIssueNumber)
		issue := map[string]any{
			"id":                 fmt.Sprintf("2-%d", s.nextIssueNumber),
			"idReadable":         ticket,
			"summary":            payload["summary"],
			"description":        payload["description"],
			"project":            s.project,
			"status_field_name":  "State",
			"status":             "Open",
			"available_statuses": []string{"Open", "In Progress", "Fixed"},
			"customFields":       anyToMapSlice(payload["customFields"]),
			"links":              []map[string]any{},
			"attachments":        []map[string]any{},
		}
		s.issues[ticket] = issue
		s.nextIssueNumber++
		writeJSON(http.StatusOK, serializeFakeIssue(issue))
		return

	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/issues/"):
		var payload map[string]any
		_ = json.NewDecoder(r.Body).Decode(&payload)
		ticket := strings.Split(r.URL.Path, "/")[3]
		issue := s.issues[ticket]
		if issue == nil {
			writeJSON(http.StatusNotFound, map[string]any{"error": "not found"})
			return
		}
		if value, ok := payload["summary"]; ok {
			issue["summary"] = value
		}
		if value, ok := payload["description"]; ok {
			issue["description"] = value
		}
		if value, ok := payload["customFields"]; ok {
			issue["customFields"] = anyToMapSlice(value)
		}
		writeJSON(http.StatusOK, serializeFakeIssue(issue))
		return

	case r.Method == http.MethodPost && r.URL.Path == "/api/commands/assist":
		var payload map[string]any
		_ = json.NewDecoder(r.Body).Decode(&payload)
		issueRef := anyToMapSlice(payload["issues"])[0]["idReadable"].(string)
		issue := s.issues[issueRef]
		options := []map[string]any{}
		for _, status := range issue["available_statuses"].([]string) {
			options = append(options, map[string]any{"option": issue["status_field_name"].(string) + " " + status})
		}
		writeJSON(http.StatusOK, map[string]any{"suggestions": options})
		return

	case r.Method == http.MethodPost && r.URL.Path == "/api/commands":
		var payload map[string]any
		_ = json.NewDecoder(r.Body).Decode(&payload)
		query := payload["query"].(string)
		issues := anyToMapSlice(payload["issues"])
		ticket := issues[0]["idReadable"].(string)
		issue := s.issues[ticket]
		if strings.HasPrefix(query, "State ") {
			issue["status"] = parseFakeCommandValue(strings.TrimPrefix(query, "State "))
			writeJSON(http.StatusOK, map[string]any{})
			return
		}
		relation, linkedTicket := parseFakeLinkCommand(query)
		issue["links"] = append(issue["links"].([]map[string]any), map[string]any{
			"id":        "80-0",
			"direction": "OUTWARD",
			"linkType": map[string]any{
				"name":           relation,
				"sourceToTarget": relation,
				"targetToSource": "is " + relation,
			},
			"issues": []map[string]any{{
				"id":         s.issues[linkedTicket]["id"],
				"idReadable": linkedTicket,
				"summary":    s.issues[linkedTicket]["summary"],
			}},
		})
		writeJSON(http.StatusOK, map[string]any{})
		return

	case r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/links/"):
		ticket := strings.Split(r.URL.Path, "/")[3]
		issue := s.issues[ticket]
		issue["links"] = []map[string]any{}
		writeJSON(http.StatusOK, map[string]any{})
		return
	}

	writeJSON(http.StatusNotFound, map[string]any{"error": "not found"})
}

func serializeFakeIssue(issue map[string]any) map[string]any {
	return map[string]any{
		"id":          issue["id"],
		"idReadable":  issue["idReadable"],
		"summary":     issue["summary"],
		"description": issue["description"],
		"project":     issue["project"],
		"customFields": append([]map[string]any{{
			"name":  issue["status_field_name"],
			"$type": "StateIssueCustomField",
			"value": map[string]any{"name": issue["status"]},
		}}, issue["customFields"].([]map[string]any)...),
		"links":       issue["links"],
		"attachments": issue["attachments"],
	}
}

func anyToMapSlice(value any) []map[string]any {
	if value == nil {
		return nil
	}
	items, ok := value.([]any)
	if !ok {
		if maps, ok := value.([]map[string]any); ok {
			return maps
		}
		return nil
	}
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if entry, ok := item.(map[string]any); ok {
			result = append(result, entry)
		}
	}
	return result
}

func anyString(value any) string {
	text, _ := value.(string)
	return text
}

func parseFakeLinkCommand(query string) (string, string) {
	query = strings.TrimSpace(query)
	if query == "" {
		return "", ""
	}
	if strings.HasPrefix(query, "'") {
		end := findClosingQuote(query)
		if end > 0 && end+2 <= len(query) {
			return parseFakeQuotedValue(query[:end+1]), strings.TrimSpace(query[end+1:])
		}
	}

	parts := strings.Split(query, " ")
	if len(parts) == 1 {
		return parseFakeCommandValue(parts[0]), ""
	}
	return parseFakeCommandValue(strings.Join(parts[:len(parts)-1], " ")), parts[len(parts)-1]
}

func parseFakeCommandValue(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'") && len(value) >= 2 {
		return parseFakeQuotedValue(value)
	}
	if index := strings.IndexAny(value, ",[]|"); index >= 0 {
		value = value[:index]
	}
	return strings.TrimSpace(value)
}

func parseFakeQuotedValue(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 && strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'") {
		value = value[1 : len(value)-1]
	}
	return strings.ReplaceAll(value, `\'`, `'`)
}

func findClosingQuote(value string) int {
	escaped := false
	for index := 1; index < len(value); index++ {
		switch {
		case escaped:
			escaped = false
		case value[index] == '\\':
			escaped = true
		case value[index] == '\'':
			return index
		}
	}
	return -1
}

func TestResolveProjectRequiresExactMatch(t *testing.T) {
	t.Parallel()

	state := newFakeState()
	state.projects = []map[string]any{
		{"id": "0-10", "name": "Foo Bar", "shortName": "FOOBAR"},
		{"id": "0-11", "name": "Foo Baz", "shortName": "FOOBAZ"},
	}

	server := httptest.NewServer(http.HandlerFunc(state.handle))
	defer server.Close()

	client, err := NewClient(server.URL, "perm:test", server.Client())
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	_, err = client.resolveProject(context.Background(), "FOO")
	if err == nil {
		t.Fatalf("expected resolveProject to fail on ambiguous partial match")
	}
	if !strings.Contains(err.Error(), `could not resolve YouTrack project "FOO" exactly`) {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), "FOOBAR") || !strings.Contains(err.Error(), "FOOBAZ") {
		t.Fatalf("expected candidate list in error: %v", err)
	}
}

func TestNormalizeCustomFieldsAcceptsObjectAndArray(t *testing.T) {
	t.Parallel()

	objectFields, err := normalizeCustomFields(map[string]any{
		"name":  "Priority",
		"value": map[string]any{"name": "Major"},
	})
	if err != nil {
		t.Fatalf("normalize object custom fields: %v", err)
	}
	if len(objectFields) != 1 || anyString(objectFields[0]["name"]) != "Priority" {
		t.Fatalf("unexpected object custom fields: %+v", objectFields)
	}

	arrayFields, err := normalizeCustomFields([]any{
		map[string]any{"name": "Priority", "value": map[string]any{"name": "Major"}},
		map[string]any{"name": "Type", "value": map[string]any{"name": "Task"}},
	})
	if err != nil {
		t.Fatalf("normalize array custom fields: %v", err)
	}
	if len(arrayFields) != 2 {
		t.Fatalf("unexpected array custom fields: %+v", arrayFields)
	}
}

func TestCustomFieldsInputUnmarshalSupportsObjectAndArray(t *testing.T) {
	t.Parallel()

	var single CustomFieldsInput
	if err := json.Unmarshal([]byte(`{"name":"Priority","value":{"name":"Major"}}`), &single); err != nil {
		t.Fatalf("unmarshal single custom field: %v", err)
	}
	if len(single) != 1 || anyString(single[0]["name"]) != "Priority" {
		t.Fatalf("unexpected single custom field payload: %+v", single)
	}

	var many CustomFieldsInput
	if err := json.Unmarshal([]byte(`[{"name":"Priority","value":{"name":"Major"}}]`), &many); err != nil {
		t.Fatalf("unmarshal custom field array: %v", err)
	}
	if len(many) != 1 || anyString(many[0]["name"]) != "Priority" {
		t.Fatalf("unexpected custom field array payload: %+v", many)
	}
}

func TestSanitizeAttachmentName(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "simple", input: "notes.txt", expected: "notes.txt"},
		{name: "path traversal reduced to base", input: "../../report.md", expected: "report.md"},
		{name: "unsafe characters replaced", input: "release\nplan?.txt", expected: "release-plan-.txt"},
		{name: "leading dots removed", input: ".env", expected: "env"},
		{name: "windows reserved name prefixed", input: "CON.txt", expected: "attachment-CON.txt"},
		{name: "empty-ish name falls back", input: "...", expected: "attachment"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			actual, err := sanitizeAttachmentName(tc.input)
			if err != nil {
				t.Fatalf("sanitize attachment name: %v", err)
			}
			if actual != tc.expected {
				t.Fatalf("expected %q, got %q", tc.expected, actual)
			}
		})
	}
}

func TestSanitizeAttachmentNameRejectsNUL(t *testing.T) {
	t.Parallel()

	_, err := sanitizeAttachmentName("bad\x00name.txt")
	if err == nil {
		t.Fatalf("expected NUL-containing attachment name to fail")
	}
}

func TestSanitizeAttachmentNameCapsLength(t *testing.T) {
	t.Parallel()

	name := strings.Repeat("a", maxAttachmentNameLength+25) + ".txt"
	sanitized, err := sanitizeAttachmentName(name)
	if err != nil {
		t.Fatalf("sanitize attachment name: %v", err)
	}
	if len(sanitized) > maxAttachmentNameLength {
		t.Fatalf("expected sanitized name length <= %d, got %d", maxAttachmentNameLength, len(sanitized))
	}
	if !strings.HasSuffix(sanitized, ".txt") {
		t.Fatalf("expected extension to be preserved, got %q", sanitized)
	}
}
