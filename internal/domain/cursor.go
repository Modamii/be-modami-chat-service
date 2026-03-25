package domain

import "time"

// Cursor represents a cursor for paginating through results.
type Cursor struct {
	CreatedAt time.Time
	ID        string
}

// PageRequest holds pagination parameters.
type PageRequest struct {
	Cursor *Cursor
	Limit  int
}

// PageResponse holds paginated results metadata.
type PageResponse[T any] struct {
	Items      []T
	NextCursor *Cursor
	HasMore    bool
}

const (
	DefaultPageSize = 50
	MaxPageSize     = 100
)

// NormalizeLimit ensures limit is within bounds.
func NormalizeLimit(limit int) int {
	if limit <= 0 {
		return DefaultPageSize
	}
	if limit > MaxPageSize {
		return MaxPageSize
	}
	return limit
}
