package mcp

import (
	"context"
	"fmt"
	"os"
	"sync"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"youtrack-mcp/internal/config"
	"youtrack-mcp/internal/version"
	"youtrack-mcp/internal/youtrack"
)

var clientCache sync.Map

func Run(ctx context.Context) error {
	server := sdkmcp.NewServer(&sdkmcp.Implementation{
		Name:    config.ServerName,
		Version: version.Value,
	}, nil)

	addTools(server)
	return server.Run(ctx, &sdkmcp.StdioTransport{})
}

func addTools(server *sdkmcp.Server) {
	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name:        toolName("search"),
		Annotations: readOnlyToolAnnotations(),
		Description: "Search YouTrack tickets by free-text or YouTrack query syntax when you do not already know the exact ticket ID. Returns structured JSON issue data for the matching tickets. Use `limit` to control how many matches are returned.",
	}, searchYouTrackTickets)

	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name:        toolName("fetch"),
		Annotations: readOnlyToolAnnotations(),
		Description: "Fetch a YouTrack ticket by explicit `ticket`. Returns structured JSON issue data, including `custom_fields` and synthesized fields like `status`. Set `attachments=true` to inline text attachment content. Use `attachment_path` to download attachments to disk and return their saved paths.",
	}, fetchYouTrackTicket)

	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name:        toolName("fetch_markdown"),
		Annotations: readOnlyToolAnnotations(),
		Description: "Fetch a YouTrack ticket by explicit `ticket`. Returns rendered Markdown. Set `attachments=true` to inline text attachment sections. Use `attachment_path` to download attachments to disk and include their saved paths.",
	}, fetchYouTrackTicketMarkdown)

	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name:        toolName("get_statuses"),
		Annotations: readOnlyToolAnnotations(),
		Description: "List the valid status values for an explicit YouTrack `ticket`, including the current status and the available status transitions returned by YouTrack.",
	}, getYouTrackTicketStatuses)

	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name:        toolName("update_status"),
		Annotations: writeToolAnnotations(true, true),
		Description: "Update the status of an explicit YouTrack `ticket` to the provided `status` value. Use the status-listing tool first when you need YouTrack-valid status names.",
	}, updateYouTrackTicketStatus)

	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name:        toolName("create"),
		Annotations: writeToolAnnotations(false, false),
		Description: "Create a new YouTrack ticket in the specified `project` with the provided `summary`, and optional `description` and `custom_fields` JSON.",
	}, createYouTrackTicket)

	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name:        toolName("update"),
		Annotations: writeToolAnnotations(true, true),
		Description: "Update an existing explicit YouTrack `ticket`. Provide at least one of `summary`, `description`, or `custom_fields`; calling this tool with none of them is an error. Set `summary` or `description` to an explicit empty string when you want to clear that field.",
	}, updateYouTrackTicket)

	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name:        toolName("link"),
		Annotations: writeToolAnnotations(false, false),
		Description: "Create a YouTrack link from `ticket` to `linked_ticket` using the provided `relation` command text, for example `relates to`. Multi-word or punctuated relation values are automatically quoted before sending the command to YouTrack.",
	}, linkYouTrackTickets)

	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name:        toolName("unlink"),
		Annotations: writeToolAnnotations(true, true),
		Description: "Remove an existing YouTrack link between `ticket` and `linked_ticket`. Provide `relation` when the ticket pair has multiple link types and you need to disambiguate which one to remove.",
	}, unlinkYouTrackTickets)
}

func toolName(base string) string {
	return config.ToolPrefix() + base
}

func readOnlyToolAnnotations() *sdkmcp.ToolAnnotations {
	return &sdkmcp.ToolAnnotations{
		ReadOnlyHint:  true,
		OpenWorldHint: boolPtr(false),
	}
}

func writeToolAnnotations(idempotent bool, destructive bool) *sdkmcp.ToolAnnotations {
	return &sdkmcp.ToolAnnotations{
		DestructiveHint: boolPtr(destructive),
		IdempotentHint:  idempotent,
		OpenWorldHint:   boolPtr(false),
	}
}

func boolPtr(value bool) *bool {
	return &value
}

type repoTicketArgs struct {
	Ticket string `json:"ticket" jsonschema:"Required YouTrack ticket ID. This server does not infer tickets from branch names or repo paths."`
}

type searchArgs struct {
	Query string `json:"query" jsonschema:"Required free-text search string or YouTrack issue query."`
	Limit int    `json:"limit,omitempty" jsonschema:"Optional maximum number of matching tickets to return. Defaults to 10 and is capped at 100."`
}

type fetchTicketArgs struct {
	Ticket         string `json:"ticket" jsonschema:"Required YouTrack ticket ID. This server does not infer tickets from branch names or repo paths."`
	Attachments    bool   `json:"attachments,omitempty" jsonschema:"When true, inline text attachment contents into the structured JSON attachments array. This can substantially increase response size for large attachments."`
	AttachmentPath string `json:"attachment_path,omitempty" jsonschema:"Optional directory where attachments should be downloaded to disk. Saved file paths are returned in the structured JSON attachments array."`
}

type fetchTicketMarkdownArgs struct {
	Ticket         string `json:"ticket" jsonschema:"Required YouTrack ticket ID. This server does not infer tickets from branch names or repo paths."`
	Attachments    bool   `json:"attachments,omitempty" jsonschema:"When true, inline text attachment contents into the rendered Markdown. This can substantially increase response size for large attachments."`
	AttachmentPath string `json:"attachment_path,omitempty" jsonschema:"Optional directory where attachments should be downloaded to disk. The rendered Markdown includes the saved paths."`
}

type createTicketArgs struct {
	Project      string                     `json:"project" jsonschema:"YouTrack project ID, short name, or full name."`
	Summary      string                     `json:"summary" jsonschema:"Ticket summary."`
	Description  string                     `json:"description,omitempty" jsonschema:"Optional ticket description."`
	CustomFields youtrack.CustomFieldsInput `json:"custom_fields,omitempty" jsonschema:"Optional YouTrack custom field object or array of field objects. Pass real JSON, not a JSON-encoded string."`
}

type updateTicketArgs struct {
	Ticket       string                     `json:"ticket" jsonschema:"Required YouTrack ticket ID. This server does not infer tickets from branch names or repo paths."`
	Summary      *string                    `json:"summary,omitempty" jsonschema:"Optional replacement summary. Provide this, description, or custom_fields; at least one update field is required. Set it to an empty string to clear the summary."`
	Description  *string                    `json:"description,omitempty" jsonschema:"Optional replacement description. Provide this, summary, or custom_fields; at least one update field is required. Set it to an empty string to clear the description."`
	CustomFields youtrack.CustomFieldsInput `json:"custom_fields,omitempty" jsonschema:"Optional YouTrack custom field object or array of field objects. Provide this, summary, or description; at least one update field is required. Pass real JSON, not a JSON-encoded string."`
}

type updateStatusArgs struct {
	Status string `json:"status" jsonschema:"The new status value to apply."`
	Ticket string `json:"ticket" jsonschema:"Required YouTrack ticket ID. This server does not infer tickets from branch names or repo paths."`
}

type linkTicketArgs struct {
	LinkedTicket string `json:"linked_ticket" jsonschema:"The YouTrack ticket to link to."`
	Relation     string `json:"relation" jsonschema:"The YouTrack command relation, for example 'relates to'."`
	Ticket       string `json:"ticket" jsonschema:"Required YouTrack ticket ID. This server does not infer tickets from branch names or repo paths."`
}

type unlinkTicketArgs struct {
	LinkedTicket string `json:"linked_ticket" jsonschema:"The linked YouTrack ticket to remove."`
	Ticket       string `json:"ticket" jsonschema:"Required YouTrack ticket ID. This server does not infer tickets from branch names or repo paths."`
	Relation     string `json:"relation,omitempty" jsonschema:"Optional relation name to disambiguate unlinking."`
}

func fetchYouTrackTicket(ctx context.Context, _ *sdkmcp.CallToolRequest, input fetchTicketArgs) (*sdkmcp.CallToolResult, youtrack.Issue, error) {
	client, ticketID, err := resolveClientAndTicket(input.Ticket)
	if err != nil {
		return nil, youtrack.Issue{}, err
	}

	issue, err := client.FetchTicket(ctx, ticketID, input.Attachments, input.AttachmentPath)
	if err != nil {
		return nil, youtrack.Issue{}, err
	}
	return nil, issue, nil
}

func fetchYouTrackTicketMarkdown(ctx context.Context, _ *sdkmcp.CallToolRequest, input fetchTicketMarkdownArgs) (*sdkmcp.CallToolResult, struct{}, error) {
	client, ticketID, err := resolveClientAndTicket(input.Ticket)
	if err != nil {
		return nil, struct{}{}, err
	}

	story, err := client.FetchStory(ctx, ticketID, input.Attachments, input.AttachmentPath)
	if err != nil {
		return nil, struct{}{}, err
	}

	return &sdkmcp.CallToolResult{
		Content: []sdkmcp.Content{
			&sdkmcp.TextContent{Text: story},
		},
	}, struct{}{}, nil
}

func searchYouTrackTickets(ctx context.Context, _ *sdkmcp.CallToolRequest, input searchArgs) (*sdkmcp.CallToolResult, youtrack.SearchResults, error) {
	client, err := resolveClient()
	if err != nil {
		return nil, youtrack.SearchResults{}, err
	}

	results, err := client.SearchTickets(ctx, input.Query, input.Limit)
	return nil, results, err
}

func getYouTrackTicketStatuses(ctx context.Context, _ *sdkmcp.CallToolRequest, input repoTicketArgs) (*sdkmcp.CallToolResult, youtrack.TicketStatuses, error) {
	client, ticketID, err := resolveClientAndTicket(input.Ticket)
	if err != nil {
		return nil, youtrack.TicketStatuses{}, err
	}

	result, err := client.GetTicketStatuses(ctx, ticketID)
	return nil, result, err
}

func updateYouTrackTicketStatus(ctx context.Context, _ *sdkmcp.CallToolRequest, input updateStatusArgs) (*sdkmcp.CallToolResult, youtrack.StatusUpdate, error) {
	client, ticketID, err := resolveClientAndTicket(input.Ticket)
	if err != nil {
		return nil, youtrack.StatusUpdate{}, err
	}

	result, err := client.UpdateTicketStatus(ctx, ticketID, input.Status)
	return nil, result, err
}

func createYouTrackTicket(ctx context.Context, _ *sdkmcp.CallToolRequest, input createTicketArgs) (*sdkmcp.CallToolResult, youtrack.Issue, error) {
	client, err := resolveClient()
	if err != nil {
		return nil, youtrack.Issue{}, err
	}

	result, err := client.CreateTicket(ctx, input.Project, input.Summary, input.Description, input.CustomFields)
	return nil, result, err
}

func updateYouTrackTicket(ctx context.Context, _ *sdkmcp.CallToolRequest, input updateTicketArgs) (*sdkmcp.CallToolResult, youtrack.Issue, error) {
	client, ticketID, err := resolveClientAndTicket(input.Ticket)
	if err != nil {
		return nil, youtrack.Issue{}, err
	}
	if input.Summary == nil && input.Description == nil && len(input.CustomFields) == 0 {
		return nil, youtrack.Issue{}, fmt.Errorf("provide at least one of summary, description, or custom_fields")
	}

	result, err := client.UpdateTicket(ctx, ticketID, input.Summary, input.Description, input.CustomFields)
	return nil, result, err
}

func linkYouTrackTickets(ctx context.Context, _ *sdkmcp.CallToolRequest, input linkTicketArgs) (*sdkmcp.CallToolResult, youtrack.LinkResult, error) {
	client, ticketID, err := resolveClientAndTicket(input.Ticket)
	if err != nil {
		return nil, youtrack.LinkResult{}, err
	}

	result, err := client.LinkTickets(ctx, ticketID, input.LinkedTicket, input.Relation)
	return nil, result, err
}

func unlinkYouTrackTickets(ctx context.Context, _ *sdkmcp.CallToolRequest, input unlinkTicketArgs) (*sdkmcp.CallToolResult, youtrack.UnlinkResult, error) {
	client, ticketID, err := resolveClientAndTicket(input.Ticket)
	if err != nil {
		return nil, youtrack.UnlinkResult{}, err
	}

	result, err := client.UnlinkTickets(ctx, ticketID, input.LinkedTicket, input.Relation)
	return nil, result, err
}

func resolveClientAndTicket(ticket string) (*youtrack.Client, string, error) {
	client, err := resolveClient()
	if err != nil {
		return nil, "", err
	}

	resolvedTicket := ticket
	if resolvedTicket == "" {
		return nil, "", fmt.Errorf("ticket is required")
	}
	return client, resolvedTicket, nil
}

func resolveClient() (*youtrack.Client, error) {
	urlValue := os.Getenv(config.EnvYouTrackURL)
	if urlValue == "" {
		return nil, fmt.Errorf("youtrack url not configured; set %s", config.EnvYouTrackURL)
	}

	tokenValue := os.Getenv(config.EnvYouTrackToken)
	if tokenValue == "" {
		return nil, fmt.Errorf("youtrack api token not configured; set %s", config.EnvYouTrackToken)
	}

	cacheKey := urlValue + "\x00" + tokenValue
	if cached, ok := clientCache.Load(cacheKey); ok {
		return cached.(*youtrack.Client), nil
	}

	client, err := youtrack.NewClient(urlValue, tokenValue, nil)
	if err != nil {
		return nil, err
	}
	actual, _ := clientCache.LoadOrStore(cacheKey, client)
	return actual.(*youtrack.Client), nil
}

func clearClientCache() {
	clientCache.Range(func(key any, value any) bool {
		clientCache.Delete(key)
		return true
	})
}
