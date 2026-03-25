package domain

import "errors"

// Sentinel errors for the chat domain.
var (
	ErrNotFound             = errors.New("not found")
	ErrNotChatMember        = errors.New("not a member of this chat")
	ErrUnauthorized         = errors.New("unauthorized")
	ErrAlreadyMember        = errors.New("user is already a member")
	ErrGroupFull            = errors.New("group has reached maximum participants")
	ErrDuplicateMessage     = errors.New("duplicate message")
	ErrDuplicateConversation = errors.New("direct conversation already exists")
	ErrReactionLimitReached = errors.New("reaction limit reached")
	ErrRateLimited          = errors.New("rate limited")
	ErrForbidden            = errors.New("forbidden")
	ErrMessageDeleted       = errors.New("message has been deleted")
)

// ValidationError represents an input validation error.
type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string {
	return e.Message
}

// ErrInvalidInput creates a new ValidationError.
func ErrInvalidInput(msg string) *ValidationError {
	return &ValidationError{Message: msg}
}

// IsValidationError checks if an error is a ValidationError.
func IsValidationError(err error) bool {
	var ve *ValidationError
	return errors.As(err, &ve)
}
