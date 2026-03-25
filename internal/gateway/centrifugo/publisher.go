package centrifugo

import (
	"context"
	"encoding/json"
	"fmt"

	pkgcentrifugo "be-modami-chat-service/pkg/centrifugo"
)

// Publisher implements service.RealtimePublisher using Centrifugo.
type Publisher struct {
	client *pkgcentrifugo.Client
}

// NewPublisher creates a new Centrifugo publisher.
func NewPublisher(client *pkgcentrifugo.Client) *Publisher {
	return &Publisher{client: client}
}

// NewClient creates a new Centrifugo API client (convenience wrapper).
func NewClient(apiURL, apiKey string) *pkgcentrifugo.Client {
	return pkgcentrifugo.NewClient(apiURL, apiKey)
}

type realtimeEvent struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

func (p *Publisher) PublishToConversation(ctx context.Context, conversationID string, eventType string, data interface{}) error {
	channel := "conversation:" + conversationID
	payload := realtimeEvent{Type: eventType, Data: data}

	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal realtime event: %w", err)
	}

	if err := p.client.Publish(ctx, channel, json.RawMessage(encoded)); err != nil {
		return fmt.Errorf("publish to conversation: %w", err)
	}
	return nil
}

func (p *Publisher) PublishToUser(ctx context.Context, userID string, eventType string, data interface{}) error {
	channel := "personal:notifications#" + userID
	payload := realtimeEvent{Type: eventType, Data: data}

	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal realtime event: %w", err)
	}

	if err := p.client.Publish(ctx, channel, json.RawMessage(encoded)); err != nil {
		return fmt.Errorf("publish to user: %w", err)
	}
	return nil
}
