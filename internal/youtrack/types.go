package youtrack

import (
	"encoding/json"
	"fmt"
	"net/http"
)

const (
	projectFields                   = "id,name,shortName"
	issueDetailFields               = "id,idReadable,summary,description,project(id,name,shortName),customFields(name,$type,value(name,presentation,text,login,fullName)),links(id,direction,linkType(name,sourceToTarget,targetToSource),issues(id,idReadable,summary))"
	linkFields                      = "id,direction,linkType(name,sourceToTarget,targetToSource),issues(id,idReadable,summary)"
	commandSuggestionFields         = "query,suggestions(option,description,group)"
	defaultStoryIssueFields         = "summary,description,customFields(name,value(name,presentation,text))"
	storyLinkFields                 = "links(direction,linkType(name,sourceToTarget,targetToSource),issues(idReadable,summary))"
	storyAttachmentFields           = "attachments(name,url,size,mimeType)"
	maxAttachmentNameLength         = 128
	maxHTTPGetAttempts              = 2
	maxJSONResponseBytes            = 1 << 20
	maxAttachmentBytes              = 8 << 20
	defaultMaxIssueAttachmentBytes  = 32 << 20
	defaultMaxInlineAttachmentBytes = 64 << 10
	defaultMaxIssueInlineBytes      = 256 << 10
)

var maxIssueAttachmentBytes int64 = defaultMaxIssueAttachmentBytes
var maxInlineAttachmentBytes int64 = defaultMaxInlineAttachmentBytes
var maxIssueInlineBytes int64 = defaultMaxIssueInlineBytes

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

type SearchResults struct {
	Query  string  `json:"query"`
	Issues []Issue `json:"issues"`
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
