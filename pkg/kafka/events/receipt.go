package events

import "errors"

const EventTypeReadReceipt = "read_receipt"

// ReadReceiptEvent is published when a user marks messages as read.
type ReadReceiptEvent struct {
	BaseEventPayload
	ConversationID string `json:"conversation_id"`
	UserID         string `json:"user_id"`
	MessageID      string `json:"message_id"`
}

func (e *ReadReceiptEvent) GetType() string { return EventTypeReadReceipt }
func (e *ReadReceiptEvent) Validate() error {
	if err := e.BaseEventPayload.Validate(); err != nil {
		return err
	}
	if e.ConversationID == "" {
		return errors.New("conversation_id is required")
	}
	if e.UserID == "" {
		return errors.New("user_id is required")
	}
	if e.MessageID == "" {
		return errors.New("message_id is required")
	}
	return nil
}
