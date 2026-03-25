package utils

import "go.mongodb.org/mongo-driver/v2/bson"

// ObjectIDGenerator generates MongoDB ObjectID strings.
type ObjectIDGenerator struct{}

// NewID generates a new ObjectID hex string.
func (g *ObjectIDGenerator) NewID() string {
	return bson.NewObjectID().Hex()
}
