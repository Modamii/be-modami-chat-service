package kafka

import (
	"context"
	"encoding/json"

	"github.com/twmb/franz-go/pkg/kgo"
)

// TopicResolver interface for resolving topic names
// This allows different projects to implement their own topic naming strategy
type TopicResolver interface {
	ResolveTopic(baseTopic string) string
	GetAllTopics() []string
}

// Producer interface for sending messages to Kafka
// Projects can implement their own producers based on this interface
type Producer interface {
	Emit(ctx context.Context, topic string, message *ProducerMessage) error
	EmitAsync(ctx context.Context, topic string, message *ProducerMessage)
	SendMessages(ctx context.Context, topic string, messages []*ProducerMessage) error
}

// ConsumerHandler interface for handling Kafka messages
// This is already well-designed and generic
type ConsumerHandler interface {
	HandleMessage(ctx context.Context, message *kgo.Record) error
	GetTopics() []string
}

// MessageSerializer interface for custom serialization
type MessageSerializer interface {
	Serialize(value interface{}) ([]byte, error)
	Deserialize(data []byte, target interface{}) error
}

// DefaultJSONSerializer implements MessageSerializer using JSON
type DefaultJSONSerializer struct{}

func (s *DefaultJSONSerializer) Serialize(value interface{}) ([]byte, error) {
	return json.Marshal(value)
}

func (s *DefaultJSONSerializer) Deserialize(data []byte, target interface{}) error {
	return json.Unmarshal(data, target)
}
