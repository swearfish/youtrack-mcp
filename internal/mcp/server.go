package mcp

import (
	"context"
	"fmt"
	"os"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"youtrack-mcp/internal/config"
	"youtrack-mcp/internal/version"
	"youtrack-mcp/internal/youtrack"
)

var (
	falseBool = false
	trueBool  = true
)

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
		Name:        "fetch",
		Annotations: readOnlyToolAnnotations(),
		Description: "Fetch a YouTrack ticket by explicit `ticket`. By default this returns structured JSON issue data like the other tools. Set `markdown=true` to get the rendered Markdown story output instead. In structured mode, `attachments=true` inlines text attachment content into the JSON response, and `attachment_path` downloads attachments to disk and returns saved file paths in the JSON. In Markdown mode, the same flags control inline attachment sections and attachment downloads.",
	}, fetchYouTrackUserStory)

	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name:        "get_statuses",
		Annotations: readOnlyToolAnnotations(),
		Description: "List the valid status values for an explicit YouTrack `ticket`, including the current status and the available status transitions returned by YouTrack.",
	}, getYouTrackTicketStatuses)

	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name:        "update_status",
		Annotations: writeToolAnnotations(true),
		Description: "Update the status of an explicit YouTrack `ticket` to the provided `status` value. Use `get_statuses` first when you need YouTrack-valid status names.",
	}, updateYouTrackTicketStatus)

	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name:        "create",
		Annotations: writeToolAnnotations(false),
		Description: "Create a new YouTrack ticket in the specified `project` with the provided `summary`, and optional `description` and `custom_fields` JSON.",
	}, createYouTrackTicket)

	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name:        "update",
		Annotations: writeToolAnnotations(true),
		Description: "Update an existing explicit YouTrack `ticket`. Provide at least one of `summary`, `description`, or `custom_fields`; calling this tool with none of them is an error. Set `summary` or `description` to an explicit empty string when you want to clear that field.",
	}, updateYouTrackTicket)

	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name:        "link",
		Annotations: writeToolAnnotations(false),
		Description: "Create a YouTrack link from `ticket` to `linked_ticket` using the provided `relation` command text, for example `relates to`.",
	}, linkYouTrackTickets)

	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name:        "unlink",
		Annotations: writeToolAnnotations(true),
		Description: "Remove an existing YouTrack link between `ticket` and `linked_ticket`. Provide `relation` when the ticket pair has multiple link types and you need to disambiguate which one to remove.",
	}, unlinkYouTrackTickets)
}

func readOnlyToolAnnotations() *sdkmcp.ToolAnnotations {
	return &sdkmcp.ToolAnnotations{
		ReadOnlyHint:  true,
		OpenWorldHint: &falseBool,
	}
}

func writeToolAnnotations(idempotent bool) *sdkmcp.ToolAnnotations {
	return &sdkmcp.ToolAnnotations{
		DestructiveHint: &trueBool,
		IdempotentHint:  idempotent,
		OpenWorldHint:   &falseBool,
	}
}

type repoTicketArgs struct {
	Ticket string `json:"ticket" jsonschema:"Required YouTrack ticket ID. This server does not infer tickets from branch names or repo paths."`
}

type fetchStoryArgs struct {
	Ticket         string `json:"ticket" jsonschema:"Required YouTrack ticket ID. This server does not infer tickets from branch names or repo paths."`
	Markdown       bool   `json:"markdown,omitempty" jsonschema:"When true, return the current Markdown story output instead of structured JSON issue data."`
	Attachments    bool   `json:"attachments,omitempty" jsonschema:"When true, inline text attachment contents. In structured JSON mode the text is added to the attachments array; in Markdown mode it is appended to the rendered story. This can substantially increase response size for large attachments."`
	AttachmentPath string `json:"attachment_path,omitempty" jsonschema:"Optional directory where attachments should be downloaded to disk. In structured JSON mode saved file paths are returned in the attachments array; in Markdown mode the rendered story includes the saved paths."`
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

func fetchYouTrackUserStory(ctx context.Context, _ *sdkmcp.CallToolRequest, input fetchStoryArgs) (*sdkmcp.CallToolResult, any, error) {
	client, ticketID, err := resolveClientAndTicket(input.Ticket)
	if err != nil {
		return nil, nil, err
	}

	if input.Markdown {
		story, err := client.FetchStory(ctx, ticketID, input.Attachments, input.AttachmentPath)
		if err != nil {
			return nil, nil, err
		}

		return &sdkmcp.CallToolResult{
			Content: []sdkmcp.Content{
				&sdkmcp.TextContent{Text: story},
			},
		}, nil, nil
	}

	issue, err := client.FetchTicket(ctx, ticketID, input.Attachments, input.AttachmentPath)
	if err != nil {
		return nil, nil, err
	}
	return nil, issue, nil
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

	client, err := youtrack.NewClient(urlValue, tokenValue, nil)
	if err != nil {
		return nil, err
	}
	return client, nil
}
