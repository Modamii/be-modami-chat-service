package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"be-modami-chat-service/internal/domain"
	"be-modami-chat-service/internal/service"
	pkgcentrifugo "be-modami-chat-service/pkg/centrifugo"

	logging "gitlab.com/lifegoeson-libs/pkg-logging"
	"gitlab.com/lifegoeson-libs/pkg-logging/logger"
)

// CentrifugoProxy handles Centrifugo proxy HTTP callbacks.
type CentrifugoProxy struct {
	chatSvc *service.ChatService
}

// NewCentrifugoProxy creates a new Centrifugo proxy handler.
func NewCentrifugoProxy(chatSvc *service.ChatService) *CentrifugoProxy {
	return &CentrifugoProxy{chatSvc: chatSvc}
}

// HandleSubscribe handles Centrifugo subscribe proxy requests.
// Validates that the user has access to the requested channel.
// The user ID is already set by noti-service's connect handler.
func (p *CentrifugoProxy) HandleSubscribe(w http.ResponseWriter, r *http.Request) {
	var req pkgcentrifugo.ProxySubscribeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		proxyError(w, pkgcentrifugo.ProxyErrorInternal, "invalid request")
		return
	}

	userID := req.User
	channel := req.Channel

	// chat:room:<conversationID> — user must be a member of the conversation
	if strings.HasPrefix(channel, "chat:room:") {
		convID := strings.TrimPrefix(channel, "chat:room:")
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

// HandlePublish handles Centrifugo publish proxy requests.
// Receives a publish payload from a client, persists and broadcasts the message.
func (p *CentrifugoProxy) HandlePublish(w http.ResponseWriter, r *http.Request) {
	var req pkgcentrifugo.ProxyPublishRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		proxyError(w, pkgcentrifugo.ProxyErrorInternal, "invalid request")
		return
	}

	userID := req.User
	channel := req.Channel

	if !strings.HasPrefix(channel, "chat:room:") {
		proxyError(w, pkgcentrifugo.ProxyErrorForbidden, "unknown channel")
		return
	}

	convID := strings.TrimPrefix(channel, "chat:room:")

	var payload struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(req.Data, &payload); err != nil {
		proxyError(w, pkgcentrifugo.ProxyErrorInternal, "invalid payload")
		return
	}

	msgType := domain.MessageTypeText
	if payload.Type != "" {
		msgType = domain.MessageType(payload.Type)
	}

	cmd := service.SendMessageCommand{
		ConversationID: convID,
		SenderID:       userID,
		Type:           msgType,
		Content: domain.MessageContent{
			Text: payload.Text,
		},
	}

	if _, err := p.chatSvc.SendMessage(r.Context(), cmd); err != nil {
		logger.Error(r.Context(), "publish proxy: send message failed", err,
			logging.String("user", userID),
			logging.String("channel", channel),
		)
		proxyError(w, pkgcentrifugo.ProxyErrorInternal, "failed to send message")
		return
	}

	writeProxyJSON(w, pkgcentrifugo.ProxyResponse{
		Result: pkgcentrifugo.ProxyPublishResult{},
	})
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
