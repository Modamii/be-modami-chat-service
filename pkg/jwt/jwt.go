package jwt

import (
	"fmt"
	"time"

	jwtlib "github.com/golang-jwt/jwt/v5"
)

// Claims represents the JWT claims for the chat service.
type Claims struct {
	UserID      string `json:"user_id"`
	DisplayName string `json:"display_name,omitempty"`
	jwtlib.RegisteredClaims
}

// Service handles JWT operations.
type Service struct {
	secret     []byte
	expiration time.Duration
}

// NewService creates a new JWT service.
func NewService(secret string, expirationMinutes int) *Service {
	return &Service{
		secret:     []byte(secret),
		expiration: time.Duration(expirationMinutes) * time.Minute,
	}
}

// GenerateToken creates a new JWT token for a user.
func (s *Service) GenerateToken(userID, displayName string) (string, error) {
	claims := Claims{
		UserID:      userID,
		DisplayName: displayName,
		RegisteredClaims: jwtlib.RegisteredClaims{
			Subject:   userID,
			ExpiresAt: jwtlib.NewNumericDate(time.Now().Add(s.expiration)),
			IssuedAt:  jwtlib.NewNumericDate(time.Now()),
		},
	}

	token := jwtlib.NewWithClaims(jwtlib.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(s.secret)
	if err != nil {
		return "", fmt.Errorf("sign token: %w", err)
	}
	return tokenString, nil
}

// ValidateToken validates a JWT token and returns the claims.
func (s *Service) ValidateToken(tokenString string) (*Claims, error) {
	token, err := jwtlib.ParseWithClaims(tokenString, &Claims{}, func(token *jwtlib.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwtlib.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.secret, nil
	})
	if err != nil {
		return nil, fmt.Errorf("parse token: %w", err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token claims")
	}

	return claims, nil
}

// GenerateCentrifugoToken creates a Centrifugo connection JWT.
func (s *Service) GenerateCentrifugoToken(userID, displayName, avatarURL string) (string, error) {
	claims := jwtlib.MapClaims{
		"sub": userID,
		"exp": time.Now().Add(s.expiration).Unix(),
		"info": map[string]interface{}{
			"name":   displayName,
			"avatar": avatarURL,
		},
		"channels": []string{"personal:notifications#" + userID},
	}

	token := jwtlib.NewWithClaims(jwtlib.SigningMethodHS256, claims)
	return token.SignedString(s.secret)
}
