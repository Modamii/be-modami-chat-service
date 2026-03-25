package events

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)
type EventActor struct {
	ID            string `json:"id"`
	Username      string `json:"username"`
	Fullname      string `json:"fullname"`
	Avatar        string `json:"avatar"`
	Status        string `json:"status"`
}
type EventPayload interface {
	GetType() string
	Validate() error
}
type BaseEventPayload struct {
	Type      string                 `json:"type"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
}
func (b *BaseEventPayload) GetType() string {
	return b.Type
}
func (b *BaseEventPayload) Validate() error {
	if b.Type == "" {
		return errors.New("event type is required")
	}
	return nil
}
// Kafka Event Base
type KafkaEventBase struct {
	RequestID  string                 `json:"requestId"`
	EventID    string                 `json:"eventId"`
	EventKeyID string                 `json:"eventKeyId"`
	EventType  string 				`json:"eventType"`
	Payload    EventPayload           `json:"payload"`
	Actor      *EventActor            `json:"actor,omitempty"`
	Headers    map[string]interface{} `json:"headers,omitempty"`
	CreatedAt  time.Time              `json:"createdAt"`
}
func NewKafkaEventBase(payload EventPayload, eventKeyID string, actor *EventActor) *KafkaEventBase {
	return &KafkaEventBase{
		RequestID:  uuid.New().String(),
		EventID:    uuid.New().String(),
		EventKeyID: eventKeyID,
		Payload:    payload,
		Actor:      actor,
		Headers:    make(map[string]interface{}),
		CreatedAt:  time.Now(),
	}
}
func (k *KafkaEventBase) GetTopic() string {
	return k.Payload.GetType()
}
func (k *KafkaEventBase) GetEventName() string {
	return k.Payload.GetType()
}
func (k *KafkaEventBase) SetHeader(key string, value interface{}) {
	if k.Headers == nil {
		k.Headers = make(map[string]interface{})
	}
	k.Headers[key] = value
}
func (k *KafkaEventBase) GetHeader(key string) (interface{}, bool) {
	if k.Headers == nil {
		return nil, false
	}
	value, exists := k.Headers[key]
	return value, exists
}

// Outbox Event Base
type OutboxEventBase struct {
	EventID     string                 `json:"eventId"`
	RequestID   string                 `json:"requestId"`
	AggregateID string                 `json:"aggregateId"`
	Payload     EventPayload           `json:"payload"`
	Actor       *EventActor            `json:"actor,omitempty"`
	CreatedAt   time.Time              `json:"createdAt"`
}
func NewOutboxEventBase(payload EventPayload, aggregateID string, actor *EventActor) *OutboxEventBase {
	return &OutboxEventBase{
		EventID:     uuid.New().String(),
		RequestID:   uuid.New().String(),
		AggregateID: aggregateID,
		Payload:     payload,
		Actor:       actor,
		CreatedAt:   time.Now(),
	}
}
func (o *OutboxEventBase) GetAggregateType() string {
	return o.Payload.GetType()
}
func (o *OutboxEventBase) GetEventName() string {
	return o.Payload.GetType()
}
type OutboxEventMessage struct {
	Payload   EventPayload `json:"payload"`
	EventType string       `json:"eventType"`
	Actor     *EventActor  `json:"actor,omitempty"`
	CreatedAt time.Time    `json:"createdAt"`
}
type CDCEventMessage struct {
	Operation string                 `json:"op"` // c=create, u=update, d=delete
	Before    map[string]interface{} `json:"before,omitempty"`
	After     map[string]interface{} `json:"after,omitempty"`
	Source    CDCSource              `json:"source"`
	Timestamp int64                  `json:"ts_ms"`
}
type CDCSource struct {
	Version   string `json:"version"`
	Connector string `json:"connector"`
	Name      string `json:"name"`
	Database  string `json:"db"`
	Table     string `json:"table"`
	Schema    string `json:"schema"`
}
type contextKey string
const (
	requestIDKey contextKey = "requestId"
	userKey      contextKey = "user"
)
func GetRequestIDFromContext(ctx context.Context) string {
	if requestID, ok := ctx.Value(requestIDKey).(string); ok {
		return requestID
	}
	return ""
}
func SetRequestIDInContext(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDKey, requestID)
}
func GetUserFromContext(ctx context.Context) *EventActor {
	if user, ok := ctx.Value(userKey).(*EventActor); ok {
		return user
	}
	return nil
}
func SetUserInContext(ctx context.Context, user *EventActor) context.Context {
	return context.WithValue(ctx, userKey, user)
} 