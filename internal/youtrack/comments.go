package youtrack

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

func (c *Client) PostComment(ctx context.Context, ticketID string, text string) (Comment, error) {
	if strings.TrimSpace(text) == "" {
		return Comment{}, fmt.Errorf("comment text is required")
	}

	var response struct {
		ID   string `json:"id"`
		Text string `json:"text"`
	}
	if err := c.doJSON(ctx, http.MethodPost, "/api/issues/"+url.PathEscape(ticketID)+"/comments", map[string]string{
		"fields": "id,text",
	}, map[string]string{"text": text}, &response); err != nil {
		return Comment{}, err
	}
	return Comment{ID: response.ID, Ticket: ticketID, Text: response.Text}, nil
}
