package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client wraps HTTP client for AI Hub API calls
type Client struct {
	BaseURL string
	HTTP    *http.Client
}

// NewClient creates a new API client
func NewClient(port int) *Client {
	return &Client{
		BaseURL: fmt.Sprintf("http://localhost:%d/api/v1", port),
		HTTP: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Request makes an HTTP request and returns response body
func (c *Client) Request(method, path string, body interface{}) ([]byte, error) {
	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}

	url := c.BaseURL + path
	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// GET makes a GET request
func (c *Client) GET(path string) ([]byte, error) {
	return c.Request("GET", path, nil)
}

// POST makes a POST request
func (c *Client) POST(path string, body interface{}) ([]byte, error) {
	return c.Request("POST", path, body)
}

// PUT makes a PUT request
func (c *Client) PUT(path string, body interface{}) ([]byte, error) {
	return c.Request("PUT", path, body)
}

// DELETE makes a DELETE request
func (c *Client) DELETE(path string) ([]byte, error) {
	return c.Request("DELETE", path, nil)
}
