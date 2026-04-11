package utils

import (
	"strings"

	"github.com/google/uuid"
)

// ObjectIDGenerator generates unique ID strings using UUID v4.
type ObjectIDGenerator struct{}

// NewID generates a new unique ID (UUID v4, no dashes).
func (g *ObjectIDGenerator) NewID() string {
	id := uuid.New()
	return strings.ReplaceAll(id.String(), "-", "")
}
