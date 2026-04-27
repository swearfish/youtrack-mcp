package youtrack

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"unicode"
)

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
	query := commandFieldToken(fieldName) + " "
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
		candidate := stripAssistQueryPrefix(option, fieldName)
		if candidate == "" {
			continue
		}
		if slices.Contains(statuses, candidate) {
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
	if err := c.applyCommand(ctx, commandFieldToken(fieldName)+" "+quoteCommandValue(status), []string{ticketID}); err != nil {
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
		Relation:       firstNonEmpty(strings.TrimSpace(relation), displayRelationName(linkData)),
	}, nil
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
	escaped := strings.NewReplacer(`\`, `\\`, `}`, `\}`).Replace(value)
	return "{" + escaped + "}"
}

func commandFieldToken(fieldName string) string {
	if needsCommandQuoting(fieldName) {
		return quoteCommandValue(fieldName)
	}
	return strings.TrimSpace(fieldName)
}

func stripAssistQueryPrefix(option string, fieldName string) string {
	prefixes := []string{
		commandFieldToken(fieldName) + " ",
		strings.TrimSpace(fieldName) + " ",
	}
	for _, prefix := range prefixes {
		if strings.HasPrefix(option, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(option, prefix))
		}
	}
	return ""
}

func needsCommandQuoting(value string) bool {
	trimmed := strings.TrimSpace(value)
	return strings.IndexFunc(trimmed, func(r rune) bool {
		return unicode.IsSpace(r) || (!unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' && r != '-')
	}) >= 0
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
