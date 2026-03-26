package events

// MessageSentEvent is published when a new message is sent.
type MessageSentEvent struct {
	ConversationID string `json:"conversation_id"`
	MessageID      string `json:"message_id"`
	SenderID       string `json:"sender_id"`
	Type           string `json:"type"`
	Text           string `json:"text,omitempty"`
}

// MessageUpdatedEvent is published when a message is edited.
type MessageUpdatedEvent struct {
	ConversationID string `json:"conversation_id"`
	MessageID      string `json:"message_id"`
	SenderID       string `json:"sender_id"`
	Text           string `json:"text"`
}

// MessageDeletedEvent is published when a message is soft-deleted.
type MessageDeletedEvent struct {
	ConversationID string `json:"conversation_id"`
	MessageID      string `json:"message_id"`
	SenderID       string `json:"sender_id"`
}
