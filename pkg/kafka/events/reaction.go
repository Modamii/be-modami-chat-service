package events

import "errors"

const (
	EventTypeReactionAdded   = "reaction.added"
	EventTypeReactionRemoved = "reaction.removed"
)

// ReactionEvent is published when a reaction is added or removed.
// Action is either "added" or "removed" and determines the event type.
type ReactionEvent struct {
	BaseEventPayload
	Action         string `json:"action"` // "added" or "removed"
	ConversationID string `json:"conversation_id"`
	MessageID      string `json:"message_id"`
	Emoji          string `json:"emoji"`
	UserID         string `json:"user_id"`
}

func (e *ReactionEvent) GetType() string { return "reaction." + e.Action }
func (e *ReactionEvent) Validate() error {
	if err := e.BaseEventPayload.Validate(); err != nil {
		return err
	}
	if e.Action != "added" && e.Action != "removed" {
		return errors.New("action must be 'added' or 'removed'")
	}
	if e.ConversationID == "" {
		return errors.New("conversation_id is required")
	}
	if e.MessageID == "" {
		return errors.New("message_id is required")
	}
	if e.Emoji == "" {
		return errors.New("emoji is required")
	}
	if e.UserID == "" {
		return errors.New("user_id is required")
	}
	return nil
}
