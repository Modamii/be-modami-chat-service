package events

// ReadReceiptEvent is published when a user marks messages as read.
type ReadReceiptEvent struct {
	ConversationID string `json:"conversation_id"`
	UserID         string `json:"user_id"`
	MessageID      string `json:"message_id"`
}
