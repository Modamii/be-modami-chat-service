package centrifugo

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// GenerateConnectionToken creates a JWT for Centrifugo client connection.
// The token contains the user ID (sub) and expiration time.
func GenerateConnectionToken(secret string, userID string, ttlSeconds int) (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"sub": userID,
		"iat": now.Unix(),
		"exp": now.Add(time.Duration(ttlSeconds) * time.Second).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		return "", fmt.Errorf("centrifugo: sign connection token: %w", err)
	}
	return signed, nil
}

// GenerateSubscriptionToken creates a JWT for subscribing to a specific Centrifugo channel.
func GenerateSubscriptionToken(secret string, userID string, channel string, ttlSeconds int) (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"sub":     userID,
		"channel": channel,
		"iat":     now.Unix(),
		"exp":     now.Add(time.Duration(ttlSeconds) * time.Second).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		return "", fmt.Errorf("centrifugo: sign subscription token: %w", err)
	}
	return signed, nil
}
