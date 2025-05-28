package apiclient

import (
	"context"
	"fmt"
	"io"
	"net/http"
)

// Client represents a generic API client
type Client struct {
	Client  *http.Client
	BaseURL string
	Token   string
}

// Query performs a GET request to the API
func (c *Client) Query(ctx context.Context, queryURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", queryURL, nil)
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do query: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	return body, nil
}
