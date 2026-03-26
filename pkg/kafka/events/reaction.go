package events

// ReactionEvent is published when a reaction is added or removed.
type ReactionEvent struct {
	ConversationID string `json:"conversation_id"`
	MessageID      string `json:"message_id"`
	Emoji          string `json:"emoji"`
	UserID         string `json:"user_id"`
}
