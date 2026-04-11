package middleware

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	gokit "gitlab.com/lifegoeson-libs/pkg-gokit/apperror"
	"gitlab.com/lifegoeson-libs/pkg-gokit/response"
)

type contextKey string

const (
	ctxKeyUserID      contextKey = "user_id"
	ctxKeyRole        contextKey = "user_role"
	ctxKeyPermissions contextKey = "user_permissions"
)

var (
	errUnauthorized = gokit.New(gokit.CodeUnauthorized, "chưa xác thực")
	errForbidden    = gokit.New(gokit.CodeForbidden, "không có quyền truy cập")
)

// AuthMiddleware validates Keycloak-issued JWTs via JWKS and loads the user into context.
type AuthMiddleware struct {
	jwksURL string
	cache   *jwksCache
}

// NewAuthMiddleware creates an AuthMiddleware. If jwksURL is empty, tokens are parsed
// without signature verification (dev/test mode). Returns a non-nil error when the
// initial JWKS fetch fails (the middleware is still usable — keys are refreshed lazily).
func NewAuthMiddleware(jwksURL string) (*AuthMiddleware, error) {
	a := &AuthMiddleware{
		jwksURL: jwksURL,
		cache:   &jwksCache{keys: make(map[string]*rsa.PublicKey)},
	}
	if jwksURL != "" {
		if err := a.cache.refresh(jwksURL); err != nil {
			return a, fmt.Errorf("jwks initial fetch: %w", err)
		}
	}
	return a, nil
}

// Authenticate returns a chi-compatible middleware that enforces a valid Bearer token
// and stores the Keycloak subject (sub), role, and permissions in the request context.
func (a *AuthMiddleware) Authenticate() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw := r.Header.Get("Authorization")
			if raw == "" {
				response.Err(w, errUnauthorized)
				return
			}
			parts := strings.SplitN(raw, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				response.Err(w, errUnauthorized)
				return
			}

			claims, err := a.parse(parts[1])
			if err != nil {
				response.Err(w, errUnauthorized)
				return
			}

			sub, _ := claims["sub"].(string)
			if sub == "" {
				response.Err(w, errUnauthorized)
				return
			}

			ctx := r.Context()
			ctx = context.WithValue(ctx, ctxKeyUserID, sub)

			// Primary role from realm_access.roles[0]
			if ra, ok := claims["realm_access"].(map[string]interface{}); ok {
				if rolesRaw, ok := ra["roles"].([]interface{}); ok && len(rolesRaw) > 0 {
					if role, ok := rolesRaw[0].(string); ok {
						ctx = context.WithValue(ctx, ctxKeyRole, strings.ToLower(role))
					}
				}
			}

			// resource_access permissions for the client (azp claim)
			if clientID, ok := claims["azp"].(string); ok && clientID != "" {
				if ra, ok := claims["resource_access"].(map[string]interface{}); ok {
					if client, ok := ra[clientID].(map[string]interface{}); ok {
						if rolesRaw, ok := client["roles"].([]interface{}); ok {
							perms := make([]string, 0, len(rolesRaw))
							for _, r := range rolesRaw {
								if rs, ok := r.(string); ok && rs != "" {
									perms = append(perms, rs)
								}
							}
							ctx = context.WithValue(ctx, ctxKeyPermissions, perms)
						}
					}
				}
			}

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// JWTAuth returns a chi middleware from an AuthMiddleware (convenience wrapper).
func JWTAuth(a *AuthMiddleware) func(http.Handler) http.Handler {
	return a.Authenticate()
}

// RequireRole returns a middleware that allows the request only when the authenticated
// user's primary role matches the given role (case-insensitive).
func (a *AuthMiddleware) RequireRole(role string) func(http.Handler) http.Handler {
	want := strings.ToLower(role)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if GetRole(r.Context()) != want {
				response.Err(w, errForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// GetUserID returns the authenticated user's Keycloak subject (sub claim) from context.
func GetUserID(ctx context.Context) string {
	id, _ := ctx.Value(ctxKeyUserID).(string)
	return id
}

// GetRole returns the authenticated user's primary role (lowercased) from context.
func GetRole(ctx context.Context) string {
	role, _ := ctx.Value(ctxKeyRole).(string)
	return role
}

// GetPermissions returns the resource_access permissions for the authenticated user.
func GetPermissions(ctx context.Context) []string {
	perms, _ := ctx.Value(ctxKeyPermissions).([]string)
	return perms
}

// HasPermission reports whether the authenticated user holds the given permission.
func HasPermission(ctx context.Context, permission string) bool {
	for _, p := range GetPermissions(ctx) {
		if p == permission {
			return true
		}
	}
	return false
}

// ParseToken validates a raw Bearer token and returns the Keycloak subject (sub).
// Used by non-middleware code (e.g. Centrifugo proxy).
func (a *AuthMiddleware) ParseToken(tokenStr string) (string, error) {
	claims, err := a.parse(tokenStr)
	if err != nil {
		return "", err
	}
	sub, _ := claims["sub"].(string)
	if sub == "" {
		return "", fmt.Errorf("missing sub claim")
	}
	return sub, nil
}

// parse validates and extracts claims from a JWT token string.
func (a *AuthMiddleware) parse(tokenStr string) (jwt.MapClaims, error) {
	if a.jwksURL == "" {
		// Dev mode: no signature verification.
		token, _, err := jwt.NewParser().ParseUnverified(tokenStr, jwt.MapClaims{})
		if err != nil {
			return nil, err
		}
		return token.Claims.(jwt.MapClaims), nil
	}

	token, err := jwt.ParseWithClaims(tokenStr, jwt.MapClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		kid, _ := token.Header["kid"].(string)
		return a.cache.get(kid, a.jwksURL)
	})
	if err != nil || !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}
	return token.Claims.(jwt.MapClaims), nil
}

// ---------------------------------------------------------------------------
// JWKS cache
// ---------------------------------------------------------------------------

type jwksCache struct {
	mu          sync.RWMutex
	keys        map[string]*rsa.PublicKey
	lastRefresh time.Time
}

func (c *jwksCache) get(kid string, url string) (*rsa.PublicKey, error) {
	c.mu.RLock()
	key, ok := c.keys[kid]
	c.mu.RUnlock()
	if ok {
		return key, nil
	}
	// Unknown kid — refresh and retry once.
	if err := c.refresh(url); err != nil {
		return nil, err
	}
	c.mu.RLock()
	key, ok = c.keys[kid]
	c.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown key id: %s", kid)
	}
	return key, nil
}

func (c *jwksCache) refresh(url string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	resp, err := (&http.Client{Timeout: 10 * time.Second}).Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var jwks struct {
		Keys []struct {
			Kty string `json:"kty"`
			Kid string `json:"kid"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return err
	}

	for _, k := range jwks.Keys {
		if k.Kty != "RSA" {
			continue
		}
		pub, err := parseRSAPublicKey(k.N, k.E)
		if err != nil {
			continue
		}
		c.keys[k.Kid] = pub
	}
	c.lastRefresh = time.Now()
	return nil
}

func parseRSAPublicKey(nB64, eB64 string) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(nB64)
	if err != nil {
		return nil, err
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(eB64)
	if err != nil {
		return nil, err
	}
	n := new(big.Int).SetBytes(nBytes)
	e := new(big.Int).SetBytes(eBytes)
	return &rsa.PublicKey{N: n, E: int(e.Int64())}, nil
}
