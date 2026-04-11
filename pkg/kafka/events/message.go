package events

import "errors"

const (
	EventTypeMessageSent    = "message.sent"
	EventTypeMessageUpdated = "message.updated"
	EventTypeMessageDeleted = "message.deleted"
)

// MessageSentEvent is published when a new message is sent.
type MessageSentEvent struct {
	BaseEventPayload
	ConversationID string `json:"conversation_id"`
	MessageID      string `json:"message_id"`
	SenderID       string `json:"sender_id"`
	MessageType    string `json:"message_type"`
	Text           string `json:"text,omitempty"`
}

func (e *MessageSentEvent) GetType() string { return EventTypeMessageSent }
func (e *MessageSentEvent) Validate() error {
	if err := e.BaseEventPayload.Validate(); err != nil {
		return err
	}
	if e.ConversationID == "" {
		return errors.New("conversation_id is required")
	}
	if e.MessageID == "" {
		return errors.New("message_id is required")
	}
	if e.SenderID == "" {
		return errors.New("sender_id is required")
	}
	return nil
}

// MessageUpdatedEvent is published when a message is edited.
type MessageUpdatedEvent struct {
	BaseEventPayload
	ConversationID string `json:"conversation_id"`
	MessageID      string `json:"message_id"`
	SenderID       string `json:"sender_id"`
	Text           string `json:"text"`
}

func (e *MessageUpdatedEvent) GetType() string { return EventTypeMessageUpdated }
func (e *MessageUpdatedEvent) Validate() error {
	if err := e.BaseEventPayload.Validate(); err != nil {
		return err
	}
	if e.ConversationID == "" {
		return errors.New("conversation_id is required")
	}
	if e.MessageID == "" {
		return errors.New("message_id is required")
	}
	return nil
}

// MessageDeletedEvent is published when a message is soft-deleted.
type MessageDeletedEvent struct {
	BaseEventPayload
	ConversationID string `json:"conversation_id"`
	MessageID      string `json:"message_id"`
	SenderID       string `json:"sender_id"`
}

func (e *MessageDeletedEvent) GetType() string { return EventTypeMessageDeleted }
func (e *MessageDeletedEvent) Validate() error {
	if err := e.BaseEventPayload.Validate(); err != nil {
		return err
	}
	if e.ConversationID == "" {
		return errors.New("conversation_id is required")
	}
	if e.MessageID == "" {
		return errors.New("message_id is required")
	}
	return nil
}
