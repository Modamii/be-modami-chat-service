package events

type UserCreatedEvent struct {
	EventType string    `json:"eventType"`
	UserID    string    `json:"userId"`
}

func NewUserCreatedEvent(userID string) *UserCreatedEvent {
	return &UserCreatedEvent{
		EventType: "user_created",
		UserID:    userID,
	}
}

type UserUpdatedEvent struct {
	EventType string `json:"eventType"`
	UserID    string `json:"userId"`
}

func NewUserUpdatedEvent(userID string) *UserUpdatedEvent {
	return &UserUpdatedEvent{
		EventType: "user_updated",
		UserID:    userID,
	}
}
