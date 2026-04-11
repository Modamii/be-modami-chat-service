package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"be-modami-chat-service/internal/adapter/handler/middleware"
	"be-modami-chat-service/internal/service"
	pkgcentrifugo "be-modami-chat-service/pkg/centrifugo"

	logging "gitlab.com/lifegoeson-libs/pkg-logging"
	"gitlab.com/lifegoeson-libs/pkg-logging/logger"
)

// CentrifugoProxy handles Centrifugo proxy HTTP callbacks.
type CentrifugoProxy struct {
	authMW  *middleware.AuthMiddleware
	chatSvc *service.ChatService
}

// NewCentrifugoProxy creates a new Centrifugo proxy handler.
func NewCentrifugoProxy(authMW *middleware.AuthMiddleware, chatSvc *service.ChatService) *CentrifugoProxy {
	return &CentrifugoProxy{authMW: authMW, chatSvc: chatSvc}
}

// HandleConnect handles Centrifugo connect proxy requests.
// Validates the Keycloak JWT token and returns the user ID (sub claim).
func (p *CentrifugoProxy) HandleConnect(w http.ResponseWriter, r *http.Request) {
	var req pkgcentrifugo.ProxyConnectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		proxyError(w, pkgcentrifugo.ProxyErrorInternal, "invalid request")
		return
	}

	// Extract token from connect data
	var connectData struct {
		Token string `json:"token"`
	}
	if req.Data != nil {
		if err := json.Unmarshal(req.Data, &connectData); err != nil {
			proxyError(w, pkgcentrifugo.ProxyErrorUnauthorized, "invalid connect data")
			return
		}
	}

	if connectData.Token == "" {
		proxyError(w, pkgcentrifugo.ProxyErrorUnauthorized, "missing token")
		return
	}

	sub, err := p.authMW.ParseToken(connectData.Token)
	if err != nil {
		proxyError(w, pkgcentrifugo.ProxyErrorUnauthorized, "invalid token")
		return
	}

	writeProxyJSON(w, pkgcentrifugo.ProxyResponse{
		Result: pkgcentrifugo.ProxyConnectResult{
			User:     sub,
			Channels: []string{"personal:notifications#" + sub},
		},
	})
}

// HandleSubscribe handles Centrifugo subscribe proxy requests.
// Validates that the user has access to the requested channel.
func (p *CentrifugoProxy) HandleSubscribe(w http.ResponseWriter, r *http.Request) {
	var req pkgcentrifugo.ProxySubscribeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		proxyError(w, pkgcentrifugo.ProxyErrorInternal, "invalid request")
		return
	}

	userID := req.User
	channel := req.Channel

	// personal:notifications#<userID> — user can only subscribe to their own
	if strings.HasPrefix(channel, "personal:notifications#") {
		targetUser := strings.TrimPrefix(channel, "personal:notifications#")
		if targetUser != userID {
			proxyError(w, pkgcentrifugo.ProxyErrorForbidden, "forbidden")
			return
		}
		writeProxyJSON(w, pkgcentrifugo.ProxyResponse{
			Result: pkgcentrifugo.ProxySubscribeResult{},
		})
		return
	}

	// conversation:<id> — user must be a member
	if strings.HasPrefix(channel, "conversation:") {
		convID := strings.TrimPrefix(channel, "conversation:")
		_, err := p.chatSvc.GetConversation(r.Context(), convID, userID)
		if err != nil {
			logger.Warn(r.Context(), "subscribe denied",
				logging.String("user", userID),
				logging.String("channel", channel),
			)
			proxyError(w, pkgcentrifugo.ProxyErrorForbidden, "not a member")
			return
		}
		writeProxyJSON(w, pkgcentrifugo.ProxyResponse{
			Result: pkgcentrifugo.ProxySubscribeResult{},
		})
		return
	}

	proxyError(w, pkgcentrifugo.ProxyErrorForbidden, "unknown channel")
}

func proxyError(w http.ResponseWriter, code int, message string) {
	writeProxyJSON(w, pkgcentrifugo.ProxyResponse{
		Error: &pkgcentrifugo.ProxyError{Code: code, Message: message},
	})
}

func writeProxyJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(v)
}
