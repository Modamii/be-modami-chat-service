package kafka

import (
	"encoding/json"
	"fmt"
	"time"
)

// Envelope wraps all Kafka messages with metadata.
type Envelope struct {
	EventID   string          `json:"event_id"`
	EventType string          `json:"event_type"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt time.Time       `json:"created_at"`
}

func newEnvelope(eventType string, payload interface{}, idFunc func() string) (*Envelope, []byte, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal payload: %w", err)
	}

	env := &Envelope{
		EventID:   idFunc(),
		EventType: eventType,
		Payload:   data,
		CreatedAt: time.Now(),
	}

	encoded, err := json.Marshal(env)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal envelope: %w", err)
	}

	return env, encoded, nil
}

// DecodeEnvelope decodes a Kafka message value into an Envelope.
func DecodeEnvelope(data []byte) (*Envelope, error) {
	var env Envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, fmt.Errorf("decode envelope: %w", err)
	}
	return &env, nil
}
