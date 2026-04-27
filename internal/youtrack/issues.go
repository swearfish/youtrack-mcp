package youtrack

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
)

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

func (c *Client) SearchTickets(ctx context.Context, query string, limit int) (SearchResults, error) {
	if strings.TrimSpace(query) == "" {
		return SearchResults{}, fmt.Errorf("query is required")
	}
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}

	var issues []map[string]any
	if err := c.doJSON(ctx, http.MethodGet, "/api/issues", map[string]string{
		"fields": issueDetailFields,
		"query":  query,
		"$top":   strconv.Itoa(limit),
	}, nil, &issues); err != nil {
		return SearchResults{}, err
	}

	result := SearchResults{
		Query:  query,
		Issues: make([]Issue, 0, len(issues)),
	}
	for _, issue := range issues {
		result.Issues = append(result.Issues, serializeIssue(issue))
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

func (c *Client) resolveProject(ctx context.Context, project string) (Project, error) {
	var projects []map[string]any
	if err := c.doJSON(ctx, http.MethodGet, "/api/projects", map[string]string{
		"fields": projectFields,
		"query":  project,
	}, nil, &projects); err != nil {
		if err := c.doJSON(ctx, http.MethodGet, "/api/admin/projects", map[string]string{
			"fields": projectFields,
			"query":  project,
		}, nil, &projects); err != nil {
			return Project{}, err
		}
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
