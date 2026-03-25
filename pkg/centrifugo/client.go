package centrifugo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client publishes messages to Centrifugo via its server HTTP API.
type Client struct {
	apiURL string
	apiKey string
	http   *http.Client
}

// NewClient creates a Centrifugo API client.
func NewClient(apiURL, apiKey string) *Client {
	return &Client{
		apiURL: apiURL,
		apiKey: apiKey,
		http: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

type apiRequest struct {
	Method string      `json:"method"`
	Params interface{} `json:"params"`
}

type publishParams struct {
	Channel string      `json:"channel"`
	Data    interface{} `json:"data"`
}

type broadcastParams struct {
	Channels []string    `json:"channels"`
	Data     interface{} `json:"data"`
}

type apiResponse struct {
	Error *apiError `json:"error,omitempty"`
}

type apiError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Publish sends a message to a single Centrifugo channel.
func (c *Client) Publish(ctx context.Context, channel string, data interface{}) error {
	return c.call(ctx, apiRequest{
		Method: "publish",
		Params: publishParams{Channel: channel, Data: data},
	})
}

// Broadcast sends a message to multiple Centrifugo channels in a single call.
func (c *Client) Broadcast(ctx context.Context, channels []string, data interface{}) error {
	return c.call(ctx, apiRequest{
		Method: "broadcast",
		Params: broadcastParams{Channels: channels, Data: data},
	})
}

// Ping checks connectivity to Centrifugo by calling the info method.
func (c *Client) Ping(ctx context.Context) error {
	return c.call(ctx, apiRequest{Method: "info", Params: struct{}{}})
}

func (c *Client) call(ctx context.Context, req apiRequest) error {
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("centrifugo: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("centrifugo: create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "apikey "+c.apiKey)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return fmt.Errorf("centrifugo: http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("centrifugo: unexpected status %d: %s", resp.StatusCode, string(respBody))
	}

	var apiResp apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return fmt.Errorf("centrifugo: decode response: %w", err)
	}
	if apiResp.Error != nil {
		return fmt.Errorf("centrifugo: api error %d: %s", apiResp.Error.Code, apiResp.Error.Message)
	}
	return nil
}
