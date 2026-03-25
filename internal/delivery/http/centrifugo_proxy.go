package http

import (
	"encoding/json"
	"net/http"
	"strings"

	"be-modami-chat-service/internal/delivery/http/response"
	"be-modami-chat-service/internal/service"
	pkgcentrifugo "be-modami-chat-service/pkg/centrifugo"
	"be-modami-chat-service/pkg/jwt"

	"github.com/rs/zerolog/log"
)

// CentrifugoProxy handles Centrifugo proxy HTTP callbacks.
type CentrifugoProxy struct {
	jwtSvc  *jwt.Service
	chatSvc *service.ChatService
}

// NewCentrifugoProxy creates a new Centrifugo proxy handler.
func NewCentrifugoProxy(jwtSvc *jwt.Service, chatSvc *service.ChatService) *CentrifugoProxy {
	return &CentrifugoProxy{jwtSvc: jwtSvc, chatSvc: chatSvc}
}

// HandleConnect handles Centrifugo connect proxy requests.
// Validates the JWT token and returns the user ID.
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

	claims, err := p.jwtSvc.ValidateToken(connectData.Token)
	if err != nil {
		proxyError(w, pkgcentrifugo.ProxyErrorUnauthorized, "invalid token")
		return
	}

	result := pkgcentrifugo.ProxyConnectResult{
		User: claims.UserID,
	}

	response.JSON(w, http.StatusOK, pkgcentrifugo.ProxyResponse{Result: result})
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
		response.JSON(w, http.StatusOK, pkgcentrifugo.ProxyResponse{
			Result: pkgcentrifugo.ProxySubscribeResult{},
		})
		return
	}

	// conversation:<id> — user must be a member
	if strings.HasPrefix(channel, "conversation:") {
		convID := strings.TrimPrefix(channel, "conversation:")
		_, err := p.chatSvc.GetConversation(r.Context(), convID, userID)
		if err != nil {
			log.Warn().Err(err).Str("user", userID).Str("channel", channel).Msg("subscribe denied")
			proxyError(w, pkgcentrifugo.ProxyErrorForbidden, "not a member")
			return
		}
		response.JSON(w, http.StatusOK, pkgcentrifugo.ProxyResponse{
			Result: pkgcentrifugo.ProxySubscribeResult{},
		})
		return
	}

	proxyError(w, pkgcentrifugo.ProxyErrorForbidden, "unknown channel")
}

func proxyError(w http.ResponseWriter, code int, message string) {
	response.JSON(w, http.StatusOK, pkgcentrifugo.ProxyResponse{
		Error: &pkgcentrifugo.ProxyError{Code: code, Message: message},
	})
}
