package youtrack

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"youtrack-mcp/internal/config"
)

var textAttachmentTypes = []string{
	"application/json",
	"application/xml",
	"application/yaml",
	"application/x-yaml",
}

var textAttachmentExtensions = []string{
	".csv",
	".json",
	".log",
	".md",
	".py",
	".sh",
	".sql",
	".txt",
	".xml",
	".yaml",
	".yml",
}

var reservedWindowsFileNames = map[string]struct{}{
	"aux":  {},
	"con":  {},
	"nul":  {},
	"prn":  {},
	"com1": {},
	"com2": {},
	"com3": {},
	"com4": {},
	"com5": {},
	"com6": {},
	"com7": {},
	"com8": {},
	"com9": {},
	"lpt1": {},
	"lpt2": {},
	"lpt3": {},
	"lpt4": {},
	"lpt5": {},
	"lpt6": {},
	"lpt7": {},
	"lpt8": {},
	"lpt9": {},
}

const (
	projectFields           = "id,name,shortName"
	issueDetailFields       = "id,idReadable,summary,description,project(id,name,shortName),customFields(name,$type,value(name,presentation,text,login,fullName)),links(id,direction,linkType(name,sourceToTarget,targetToSource),issues(id,idReadable,summary))"
	linkFields              = "id,direction,linkType(name,sourceToTarget,targetToSource),issues(id,idReadable,summary)"
	commandSuggestionFields = "query,suggestions(option,description,group)"
	defaultStoryIssueFields = "summary,description,customFields(name,value(name,presentation,text))"
	storyLinkFields         = "links(direction,linkType(name,sourceToTarget,targetToSource),issues(idReadable,summary))"
	storyAttachmentFields   = "attachments(name,url,size,mimeType)"
	maxAttachmentNameLength = 128
	maxHTTPGetAttempts      = 2
)

type Client struct {
	baseURL    string
	apiToken   string
	httpClient *http.Client
}

type Project struct {
	ID        string `json:"id,omitempty"`
	Name      string `json:"name,omitempty"`
	ShortName string `json:"shortName,omitempty"`
}

type Issue struct {
	ID                 string            `json:"id,omitempty"`
	Ticket             string            `json:"ticket,omitempty"`
	Summary            string            `json:"summary,omitempty"`
	Description        string            `json:"description,omitempty"`
	Project            Project           `json:"project,omitempty"`
	Status             string            `json:"status,omitempty"`
	AcceptanceCriteria string            `json:"acceptance_criteria,omitempty"`
	LinkedTickets      []string          `json:"linked_tickets,omitempty"`
	Attachments        []IssueAttachment `json:"attachments,omitempty"`
}

type IssueAttachment struct {
	Name        string `json:"name,omitempty"`
	MIMEType    string `json:"mime_type,omitempty"`
	Size        int64  `json:"size,omitempty"`
	Content     string `json:"content,omitempty"`
	SavedPath   string `json:"saved_path,omitempty"`
	DownloadURL string `json:"download_url,omitempty"`
}

type CustomFieldsInput []map[string]any

func (c *CustomFieldsInput) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*c = nil
		return nil
	}

	var single map[string]any
	if err := json.Unmarshal(data, &single); err == nil {
		*c = CustomFieldsInput{single}
		return nil
	}

	var list []map[string]any
	if err := json.Unmarshal(data, &list); err == nil {
		*c = CustomFieldsInput(list)
		return nil
	}

	return fmt.Errorf("custom_fields must be a JSON object or array of objects")
}

type TicketStatuses struct {
	Ticket        string   `json:"ticket"`
	FieldName     string   `json:"field_name"`
	CurrentStatus string   `json:"current_status,omitempty"`
	Statuses      []string `json:"statuses"`
}

type StatusUpdate struct {
	Ticket         string `json:"ticket"`
	FieldName      string `json:"field_name"`
	PreviousStatus string `json:"previous_status,omitempty"`
	Status         string `json:"status,omitempty"`
}

type LinkResult struct {
	Ticket       string `json:"ticket"`
	LinkedTicket string `json:"linked_ticket,omitempty"`
	Relation     string `json:"relation"`
}

type UnlinkResult struct {
	Ticket         string `json:"ticket"`
	UnlinkedTicket string `json:"unlinked_ticket"`
	Relation       string `json:"relation"`
}

func NewClient(baseURL string, apiToken string, httpClient *http.Client) (*Client, error) {
	trimmedURL := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if trimmedURL == "" {
		return nil, fmt.Errorf("youtrack url is required")
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

func (c *Client) FetchStory(ctx context.Context, ticketID string, attachments bool, attachmentPath string) (string, error) {
	fields := strings.Join([]string{defaultStoryIssueFields, storyLinkFields}, ",")
	includeAttachments := attachments || strings.TrimSpace(attachmentPath) != ""
	if includeAttachments {
		fields = strings.Join([]string{fields, storyAttachmentFields}, ",")
	}

	var issue map[string]any
	if err := c.doJSON(ctx, http.MethodGet, "/api/issues/"+url.PathEscape(ticketID), map[string]string{
		"fields": fields,
	}, nil, &issue); err != nil {
		return "", err
	}

	summary, _ := issue["summary"].(string)
	description, _ := issue["description"].(string)
	customFields := asMapSlice(issue["customFields"])
	links := asMapSlice(issue["links"])

	var sections []string
	sections = append(sections, fmt.Sprintf("# %s: %s", ticketID, summary))
	if description != "" {
		sections = append(sections, "## Description\n\n"+description)
	}
	if status := extractStatus(customFields); status != "" {
		sections = append(sections, "## Status\n\n"+status)
	}
	if criteria := extractAcceptanceCriteria(customFields); criteria != "" {
		sections = append(sections, "## Acceptance Criteria\n\n"+criteria)
	}
	if linked := extractLinkedTickets(links); len(linked) > 0 {
		sections = append(sections, "## Linked Tickets\n\n"+strings.Join(linked, "\n"))
	}

	if includeAttachments {
		attachmentSections, err := c.buildAttachmentSections(ctx, asMapSlice(issue["attachments"]), attachments, attachmentPath)
		if err != nil {
			return "", err
		}
		if len(attachmentSections) > 0 {
			sections = append(sections, "## Attachments\n\n"+strings.Join(attachmentSections, "\n\n"))
		}
	}

	return strings.Join(sections, "\n\n") + "\n", nil
}

func (c *Client) FetchTicket(ctx context.Context, ticketID string, attachments bool, attachmentPath string) (Issue, error) {
	fields := issueDetailFields
	if attachments || strings.TrimSpace(attachmentPath) != "" {
		fields = strings.Join([]string{fields, storyAttachmentFields}, ",")
	}

	issue, err := c.fetchIssueWithFields(ctx, ticketID, fields)
	if err != nil {
		return Issue{}, err
	}

	result := serializeIssue(issue)
	if attachments || strings.TrimSpace(attachmentPath) != "" {
		result.Attachments, err = c.buildStructuredAttachments(ctx, asMapSlice(issue["attachments"]), attachments, attachmentPath)
		if err != nil {
			return Issue{}, err
		}
	}
	return result, nil
}

func (c *Client) CreateTicket(ctx context.Context, project string, summary string, description string, customFields any) (Issue, error) {
	projectData, err := c.resolveProject(ctx, project)
	if err != nil {
		return Issue{}, err
	}

	payload := map[string]any{
		"project": map[string]any{"id": projectData.ID},
		"summary": summary,
	}
	if strings.TrimSpace(description) != "" {
		payload["description"] = description
	}
	if fields, err := normalizeCustomFields(customFields); err != nil {
		return Issue{}, err
	} else if fields != nil {
		payload["customFields"] = fields
	}

	var issue map[string]any
	if err := c.doJSON(ctx, http.MethodPost, "/api/issues", map[string]string{
		"fields": issueDetailFields,
	}, payload, &issue); err != nil {
		return Issue{}, err
	}
	return serializeIssue(issue), nil
}

func (c *Client) UpdateTicket(ctx context.Context, ticketID string, summary *string, description *string, customFields any) (Issue, error) {
	payload := map[string]any{}
	if summary != nil {
		payload["summary"] = *summary
	}
	if description != nil {
		payload["description"] = *description
	}
	if fields, err := normalizeCustomFields(customFields); err != nil {
		return Issue{}, err
	} else if fields != nil {
		payload["customFields"] = fields
	}
	if len(payload) == 0 {
		return Issue{}, fmt.Errorf("at least one update field must be provided")
	}

	var issue map[string]any
	if err := c.doJSON(ctx, http.MethodPost, "/api/issues/"+url.PathEscape(ticketID), map[string]string{
		"fields": issueDetailFields,
	}, payload, &issue); err != nil {
		return Issue{}, err
	}
	return serializeIssue(issue), nil
}

func (c *Client) GetTicketStatuses(ctx context.Context, ticketID string) (TicketStatuses, error) {
	issue, err := c.fetchIssue(ctx, ticketID)
	if err != nil {
		return TicketStatuses{}, err
	}

	statusField := findStatusField(asMapSlice(issue["customFields"]))
	if statusField == nil {
		return TicketStatuses{}, fmt.Errorf("could not find a status field on YouTrack ticket %s", ticketID)
	}

	fieldName := textValue(statusField["name"])
	query := fieldName + " "
	var response map[string]any
	if err := c.doJSON(ctx, http.MethodPost, "/api/commands/assist", map[string]string{
		"fields": commandSuggestionFields,
	}, map[string]any{
		"query":  query,
		"caret":  len(query),
		"issues": []map[string]any{{"idReadable": ticketID}},
	}, &response); err != nil {
		return TicketStatuses{}, err
	}

	statuses := []string{}
	for _, suggestion := range asMapSlice(response["suggestions"]) {
		option := textValue(suggestion["option"])
		if !strings.HasPrefix(option, query) {
			continue
		}
		candidate := strings.TrimSpace(strings.TrimPrefix(option, query))
		if candidate == "" || slices.Contains(statuses, candidate) {
			continue
		}
		statuses = append(statuses, candidate)
	}

	return TicketStatuses{
		Ticket:        ticketID,
		FieldName:     fieldName,
		CurrentStatus: extractStatus(asMapSlice(issue["customFields"])),
		Statuses:      statuses,
	}, nil
}

func (c *Client) UpdateTicketStatus(ctx context.Context, ticketID string, status string) (StatusUpdate, error) {
	issue, err := c.fetchIssue(ctx, ticketID)
	if err != nil {
		return StatusUpdate{}, err
	}

	statusField := findStatusField(asMapSlice(issue["customFields"]))
	if statusField == nil {
		return StatusUpdate{}, fmt.Errorf("could not find a status field on YouTrack ticket %s", ticketID)
	}

	fieldName := textValue(statusField["name"])
	previous := fieldValueToText(statusField["value"])
	if err := c.applyCommand(ctx, fieldName+" "+quoteCommandValue(status), []string{ticketID}); err != nil {
		return StatusUpdate{}, err
	}

	updated, err := c.fetchIssue(ctx, ticketID)
	if err != nil {
		return StatusUpdate{}, err
	}

	return StatusUpdate{
		Ticket:         ticketID,
		FieldName:      fieldName,
		PreviousStatus: previous,
		Status:         extractStatus(asMapSlice(updated["customFields"])),
	}, nil
}

func (c *Client) LinkTickets(ctx context.Context, ticketID string, linkedTicketID string, relation string) (LinkResult, error) {
	if err := c.applyCommand(ctx, quoteCommandValue(relation)+" "+linkedTicketID, []string{ticketID}); err != nil {
		return LinkResult{}, err
	}

	return LinkResult{
		Ticket:       ticketID,
		LinkedTicket: linkedTicketID,
		Relation:     relation,
	}, nil
}

func (c *Client) UnlinkTickets(ctx context.Context, ticketID string, linkedTicketID string, relation string) (UnlinkResult, error) {
	links, err := c.fetchIssueLinks(ctx, ticketID)
	if err != nil {
		return UnlinkResult{}, err
	}

	linkData, linkedIssue, err := findMatchingLink(links, linkedTicketID, relation)
	if err != nil {
		return UnlinkResult{}, err
	}

	path := fmt.Sprintf("/api/issues/%s/links/%s/issues/%s", url.PathEscape(ticketID), url.PathEscape(textValue(linkData["id"])), url.PathEscape(textValue(linkedIssue["id"])))
	if err := c.doJSON(ctx, http.MethodDelete, path, nil, nil, nil); err != nil {
		return UnlinkResult{}, err
	}

	return UnlinkResult{
		Ticket:         ticketID,
		UnlinkedTicket: linkedTicketID,
		Relation:       displayRelationName(linkData),
	}, nil
}

func (c *Client) resolveProject(ctx context.Context, project string) (Project, error) {
	var projects []map[string]any
	if err := c.doJSON(ctx, http.MethodGet, "/api/admin/projects", map[string]string{
		"fields": projectFields,
		"query":  project,
	}, nil, &projects); err != nil {
		return Project{}, err
	}

	normalized := strings.ToLower(project)
	for _, candidate := range projects {
		if textValue(candidate["id"]) == project {
			return projectFromMap(candidate), nil
		}
		if strings.ToLower(textValue(candidate["shortName"])) == normalized {
			return projectFromMap(candidate), nil
		}
		if strings.ToLower(textValue(candidate["name"])) == normalized {
			return projectFromMap(candidate), nil
		}
	}

	if len(projects) > 0 {
		candidates := make([]string, 0, len(projects))
		for _, candidate := range projects {
			label := firstNonEmpty(
				textValue(candidate["shortName"]),
				textValue(candidate["name"]),
				textValue(candidate["id"]),
			)
			if label != "" && !slices.Contains(candidates, label) {
				candidates = append(candidates, label)
			}
		}
		if len(candidates) > 0 {
			return Project{}, fmt.Errorf(
				"could not resolve YouTrack project %q exactly; candidates: %s",
				project,
				strings.Join(candidates, ", "),
			)
		}
	}

	return Project{}, fmt.Errorf("could not resolve YouTrack project %q", project)
}

func (c *Client) fetchIssue(ctx context.Context, ticketID string) (map[string]any, error) {
	return c.fetchIssueWithFields(ctx, ticketID, issueDetailFields)
}

func (c *Client) fetchIssueWithFields(ctx context.Context, ticketID string, fields string) (map[string]any, error) {
	var issue map[string]any
	if err := c.doJSON(ctx, http.MethodGet, "/api/issues/"+url.PathEscape(ticketID), map[string]string{
		"fields": fields,
	}, nil, &issue); err != nil {
		return nil, err
	}
	return issue, nil
}

func (c *Client) fetchIssueLinks(ctx context.Context, ticketID string) ([]map[string]any, error) {
	var links []map[string]any
	if err := c.doJSON(ctx, http.MethodGet, "/api/issues/"+url.PathEscape(ticketID)+"/links", map[string]string{
		"fields": linkFields,
	}, nil, &links); err != nil {
		return nil, err
	}
	return links, nil
}

func (c *Client) applyCommand(ctx context.Context, query string, ticketIDs []string) error {
	issues := make([]map[string]any, 0, len(ticketIDs))
	for _, ticketID := range ticketIDs {
		issues = append(issues, map[string]any{"idReadable": ticketID})
	}

	var response struct {
		Commands []struct {
			Errors []string `json:"errors"`
		} `json:"commands"`
	}
	if err := c.doJSON(ctx, http.MethodPost, "/api/commands", map[string]string{
		"fields": "commands(errors)",
	}, map[string]any{
		"query":  query,
		"issues": issues,
	}, &response); err != nil {
		return err
	}

	var commandErrors []string
	for _, command := range response.Commands {
		commandErrors = append(commandErrors, command.Errors...)
	}
	if len(commandErrors) > 0 {
		return fmt.Errorf("youtrack command failed: %s", strings.Join(commandErrors, "; "))
	}

	return nil
}

func quoteCommandValue(value string) string {
	escaped := strings.ReplaceAll(value, `'`, `\'`)
	return "'" + escaped + "'"
}

func (c *Client) buildAttachmentSections(ctx context.Context, attachments []map[string]any, includeContent bool, attachmentPath string) ([]string, error) {
	targetDir, err := resolveAttachmentDir(attachmentPath)
	if err != nil {
		return nil, err
	}

	sections := make([]string, 0, len(attachments))
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

		if targetDir != "" {
			savedPath, err := storeAttachment(targetDir, name, content)
			if err != nil {
				return nil, err
			}
			lines = append(lines, "- Saved to: "+savedPath)
		}

		if includeContent {
			if decoded := decodeAttachmentText(content, attachment); decoded != "" {
				lines = append(lines, "", renderTextCodeBlock(decoded))
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
			if targetDir != "" {
				savedPath, err := storeAttachment(targetDir, entry.Name, content)
				if err != nil {
					return nil, err
				}
				entry.SavedPath = savedPath
			}
			if includeContent {
				entry.Content = decodeAttachmentText(content, attachment)
			}
		}

		result = append(result, entry)
	}

	return result, nil
}

func (c *Client) downloadAttachment(ctx context.Context, attachmentURL string) ([]byte, error) {
	requestURL := attachmentURL
	if !strings.HasPrefix(attachmentURL, "http://") && !strings.HasPrefix(attachmentURL, "https://") {
		requestURL = c.baseURL + "/" + strings.TrimLeft(attachmentURL, "/")
	}

	var lastErr error
	for attempt := 0; attempt < maxHTTPGetAttempts; attempt++ {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
		if err != nil {
			return nil, fmt.Errorf("build attachment request: %w", err)
		}
		request.Header.Set("Authorization", "Bearer "+c.apiToken)
		request.Header.Set("Accept", "*/*")

		response, err := c.httpClient.Do(request)
		if err != nil {
			lastErr = fmt.Errorf("download attachment: %w", err)
			if shouldRetryGET(attempt, 0, err) {
				continue
			}
			return nil, lastErr
		}

		body, readErr := io.ReadAll(response.Body)
		response.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read attachment response: %w", readErr)
		}
		if response.StatusCode >= 400 {
			lastErr = fmt.Errorf("download attachment failed: %s", formatHTTPError(response.StatusCode, body))
			if shouldRetryGET(attempt, response.StatusCode, nil) {
				continue
			}
			return nil, lastErr
		}

		return body, nil
	}

	return nil, lastErr
}

func (c *Client) doJSON(ctx context.Context, method string, requestPath string, query map[string]string, payload any, out any) error {
	endpoint, err := url.Parse(c.baseURL + requestPath)
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
				continue
			}
			return lastErr
		}

		body, readErr := io.ReadAll(response.Body)
		response.Body.Close()
		if readErr != nil {
			return fmt.Errorf("read response: %w", readErr)
		}
		if response.StatusCode >= 400 {
			lastErr = fmt.Errorf("youtrack request failed: %s", formatHTTPError(response.StatusCode, body))
			if shouldRetryGET(attempt, response.StatusCode, nil) {
				continue
			}
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

func normalizeCustomFields(raw any) ([]map[string]any, error) {
	if raw == nil {
		return nil, nil
	}

	switch value := raw.(type) {
	case CustomFieldsInput:
		if len(value) == 0 {
			return nil, nil
		}
		return []map[string]any(value), nil
	case string:
		if strings.TrimSpace(value) == "" {
			return nil, nil
		}
		var parsed any
		if err := json.Unmarshal([]byte(value), &parsed); err != nil {
			return nil, fmt.Errorf("custom_fields is not valid JSON: %w", err)
		}
		return normalizeCustomFields(parsed)
	case map[string]any:
		return []map[string]any{value}, nil
	case []map[string]any:
		if len(value) == 0 {
			return nil, nil
		}
		return value, nil
	case []any:
		result := make([]map[string]any, 0, len(value))
		for _, item := range value {
			obj, ok := item.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("custom_fields must be a JSON object or array of objects")
			}
			result = append(result, obj)
		}
		return result, nil
	default:
		return nil, fmt.Errorf("custom_fields must be a JSON object or array of objects")
	}
}

func serializeIssue(issue map[string]any) Issue {
	customFields := asMapSlice(issue["customFields"])
	return Issue{
		ID:                 textValue(issue["id"]),
		Ticket:             firstNonEmpty(textValue(issue["idReadable"]), textValue(issue["id"])),
		Summary:            textValue(issue["summary"]),
		Description:        textValue(issue["description"]),
		Project:            projectFromMap(asMap(issue["project"])),
		Status:             extractStatus(customFields),
		AcceptanceCriteria: extractAcceptanceCriteria(customFields),
		LinkedTickets:      extractLinkedTickets(asMapSlice(issue["links"])),
	}
}

func projectFromMap(data map[string]any) Project {
	return Project{
		ID:        textValue(data["id"]),
		Name:      textValue(data["name"]),
		ShortName: textValue(data["shortName"]),
	}
}

func findStatusField(customFields []map[string]any) map[string]any {
	for _, field := range customFields {
		fieldName := strings.ToLower(strings.TrimSpace(textValue(field["name"])))
		fieldType := strings.ToLower(strings.TrimSpace(textValue(field["$type"])))
		if fieldName == "state" || fieldName == "status" || fieldType == "stateissuecustomfield" {
			return field
		}
	}
	return nil
}

func extractStatus(customFields []map[string]any) string {
	field := findStatusField(customFields)
	if field == nil {
		return ""
	}
	return fieldValueToText(field["value"])
}

func extractAcceptanceCriteria(customFields []map[string]any) string {
	for _, field := range customFields {
		name := strings.ToLower(strings.ReplaceAll(textValue(field["name"]), " ", ""))
		if name != "acceptancecriteria" {
			continue
		}
		return fieldValueToText(field["value"])
	}
	return ""
}

func extractLinkedTickets(links []map[string]any) []string {
	result := []string{}
	for _, link := range links {
		relation := displayRelationName(link)
		for _, issue := range asMapSlice(link["issues"]) {
			issueID := firstNonEmpty(textValue(issue["idReadable"]), textValue(issue["id"]), "unknown")
			line := "- " + relation + ": " + issueID
			if summary := textValue(issue["summary"]); summary != "" {
				line += " - " + summary
			}
			result = append(result, line)
		}
	}
	return result
}

func displayRelationName(link map[string]any) string {
	linkType := asMap(link["linkType"])
	direction := textValue(link["direction"])
	switch direction {
	case "OUTWARD":
		return firstNonEmpty(textValue(linkType["sourceToTarget"]), textValue(linkType["name"]), "linked to")
	case "INWARD":
		return firstNonEmpty(textValue(linkType["targetToSource"]), textValue(linkType["name"]), "linked from")
	default:
		return firstNonEmpty(textValue(linkType["name"]), textValue(linkType["sourceToTarget"]), "linked to")
	}
}

func findMatchingLink(links []map[string]any, linkedTicketID string, relation string) (map[string]any, map[string]any, error) {
	normalizedTicket := strings.ToLower(linkedTicketID)
	normalizedRelation := strings.ToLower(strings.TrimSpace(relation))
	type match struct {
		link  map[string]any
		issue map[string]any
	}
	matches := []match{}

	for _, link := range links {
		linkType := asMap(link["linkType"])
		relationNames := []string{
			strings.ToLower(strings.TrimSpace(textValue(linkType["name"]))),
			strings.ToLower(strings.TrimSpace(textValue(linkType["sourceToTarget"]))),
			strings.ToLower(strings.TrimSpace(textValue(linkType["targetToSource"]))),
		}
		if normalizedRelation != "" && !slices.Contains(relationNames, normalizedRelation) {
			continue
		}

		for _, issue := range asMapSlice(link["issues"]) {
			issueID := strings.ToLower(firstNonEmpty(textValue(issue["idReadable"]), textValue(issue["id"])))
			if issueID == normalizedTicket {
				matches = append(matches, match{link: link, issue: issue})
			}
		}
	}

	if len(matches) == 0 {
		message := fmt.Sprintf("could not find a link to %s", linkedTicketID)
		if relation != "" {
			message += fmt.Sprintf(" with relation %q", relation)
		}
		return nil, nil, errors.New(message)
	}
	if len(matches) > 1 {
		return nil, nil, fmt.Errorf("multiple links matched %s; provide relation to disambiguate", linkedTicketID)
	}

	return matches[0].link, matches[0].issue, nil
}

func fieldValueToText(value any) string {
	switch typed := value.(type) {
	case map[string]any:
		// YouTrack commonly exposes a user-facing "presentation" string before the raw field value.
		for _, key := range []string{"presentation", "name", "text", "fullName", "login"} {
			if candidate := textValue(typed[key]); candidate != "" {
				return candidate
			}
		}
	case string:
		return strings.TrimSpace(typed)
	}
	return ""
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
		if _, err := os.Stat(destination); os.IsNotExist(err) {
			break
		}
		destination = filepath.Join(targetDir, fmt.Sprintf("%s-%d%s", stem, index, ext))
		index++
	}

	if err := os.WriteFile(destination, content, 0o644); err != nil {
		return "", fmt.Errorf("write attachment: %w", err)
	}
	return destination, nil
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

func formatHTTPError(statusCode int, body []byte) string {
	message := strings.TrimSpace(string(body))
	if message == "" {
		return fmt.Sprintf("status %d", statusCode)
	}
	return fmt.Sprintf("status %d: %s", statusCode, message)
}

func asMap(value any) map[string]any {
	result, _ := value.(map[string]any)
	return result
}

func asMapSlice(value any) []map[string]any {
	items, ok := value.([]any)
	if !ok {
		if typed, ok := value.([]map[string]any); ok {
			return typed
		}
		return nil
	}
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if obj, ok := item.(map[string]any); ok {
			result = append(result, obj)
		}
	}
	return result
}

func textValue(value any) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func int64Value(value any) int64 {
	switch typed := value.(type) {
	case int:
		return int64(typed)
	case int64:
		return typed
	case float64:
		return int64(typed)
	case json.Number:
		parsed, err := typed.Int64()
		if err == nil {
			return parsed
		}
	}
	return 0
}
