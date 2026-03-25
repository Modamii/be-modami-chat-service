package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"be-modami-chat-service/internal/delivery/http/middleware"
	"be-modami-chat-service/internal/delivery/http/response"
	"be-modami-chat-service/internal/domain"
	"be-modami-chat-service/internal/service"

	"github.com/go-chi/chi/v5"
)

// Handler handles HTTP requests for the chat service.
type Handler struct {
	chatSvc *service.ChatService
}

// NewHandler creates a new HTTP handler.
func NewHandler(chatSvc *service.ChatService) *Handler {
	return &Handler{chatSvc: chatSvc}
}

// RegisterRoutes registers all chat routes on the given router.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Route("/conversations", func(r chi.Router) {
		r.Get("/", h.ListConversations)
		r.Post("/direct", h.CreateDirectChat)
		r.Post("/group", h.CreateGroup)

		r.Route("/{conversationID}", func(r chi.Router) {
			r.Get("/", h.GetConversation)
			r.Get("/messages", h.GetMessages)
			r.Post("/messages", h.SendMessage)
			r.Post("/read", h.MarkAsRead)
			r.Post("/typing", h.HandleTyping)
			r.Post("/participants", h.AddParticipant)
			r.Delete("/participants/{userID}", h.RemoveParticipant)
		})
	})

	r.Route("/messages/{messageID}", func(r chi.Router) {
		r.Put("/", h.EditMessage)
		r.Delete("/", h.DeleteMessage)
		r.Post("/reactions", h.AddReaction)
		r.Delete("/reactions/{emoji}", h.RemoveReaction)
	})

	r.Get("/unread", h.GetUnreadCounts)
}

// --- Request/Response types ---

type sendMessageRequest struct {
	Type     string            `json:"type" validate:"required,oneof=text image video file audio"`
	Text     string            `json:"text"`
	Mentions []string          `json:"mentions,omitempty"`
	Media    *domain.MediaInfo `json:"media,omitempty"`
	ReplyTo  *struct {
		MessageID      string `json:"message_id" validate:"required"`
		SenderID       string `json:"sender_id"`
		ContentPreview string `json:"content_preview"`
	} `json:"reply_to,omitempty"`
}

type createDirectChatRequest struct {
	UserID string `json:"user_id" validate:"required"`
}

type createGroupRequest struct {
	Name      string   `json:"name" validate:"required,max=100"`
	MemberIDs []string `json:"member_ids" validate:"required,min=1,max=49"`
}

type markAsReadRequest struct {
	MessageID string `json:"message_id" validate:"required"`
}

type typingRequest struct {
	Typing bool `json:"typing"`
}

type addParticipantRequest struct {
	UserID string `json:"user_id" validate:"required"`
}

type addReactionRequest struct {
	Emoji string `json:"emoji" validate:"required"`
}

type editMessageRequest struct {
	Text string `json:"text" validate:"required,max=4096"`
}

// --- Handlers ---

func (h *Handler) ListConversations(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	page := parsePageRequest(r)

	result, err := h.chatSvc.GetConversations(r.Context(), userID, page)
	if err != nil {
		handleError(w, err)
		return
	}

	resp := response.PagedResponse{
		Data:    result.Items,
		HasMore: result.HasMore,
	}
	if result.NextCursor != nil {
		resp.NextCursor = encodeCursor(result.NextCursor)
	}
	response.JSON(w, http.StatusOK, resp)
}

func (h *Handler) GetConversation(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	convID := chi.URLParam(r, "conversationID")

	conv, err := h.chatSvc.GetConversation(r.Context(), convID, userID)
	if err != nil {
		handleError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, conv)
}

func (h *Handler) CreateDirectChat(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	var req createDirectChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	conv, err := h.chatSvc.CreateDirectChat(r.Context(), service.CreateDirectChatCommand{
		UserID1: userID,
		UserID2: req.UserID,
	})
	if err != nil {
		handleError(w, err)
		return
	}
	response.JSON(w, http.StatusCreated, conv)
}

func (h *Handler) CreateGroup(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	var req createGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	conv, err := h.chatSvc.CreateGroup(r.Context(), service.CreateGroupCommand{
		Name:      req.Name,
		CreatedBy: userID,
		MemberIDs: req.MemberIDs,
	})
	if err != nil {
		handleError(w, err)
		return
	}
	response.JSON(w, http.StatusCreated, conv)
}

func (h *Handler) GetMessages(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	convID := chi.URLParam(r, "conversationID")
	page := parsePageRequest(r)

	result, err := h.chatSvc.GetMessages(r.Context(), convID, userID, page)
	if err != nil {
		handleError(w, err)
		return
	}

	resp := response.PagedResponse{
		Data:    result.Items,
		HasMore: result.HasMore,
	}
	if result.NextCursor != nil {
		resp.NextCursor = encodeCursor(result.NextCursor)
	}
	response.JSON(w, http.StatusOK, resp)
}

func (h *Handler) SendMessage(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	convID := chi.URLParam(r, "conversationID")

	var req sendMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	cmd := service.SendMessageCommand{
		ConversationID: convID,
		SenderID:       userID,
		Type:           domain.MessageType(req.Type),
		Content: domain.MessageContent{
			Text:     req.Text,
			Mentions: req.Mentions,
		},
		Media: req.Media,
	}

	if req.ReplyTo != nil {
		cmd.ReplyTo = &domain.ReplyInfo{
			MessageID:      req.ReplyTo.MessageID,
			SenderID:       req.ReplyTo.SenderID,
			ContentPreview: req.ReplyTo.ContentPreview,
		}
	}

	msg, err := h.chatSvc.SendMessage(r.Context(), cmd)
	if err != nil {
		handleError(w, err)
		return
	}
	response.JSON(w, http.StatusCreated, msg)
}

func (h *Handler) MarkAsRead(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	convID := chi.URLParam(r, "conversationID")

	var req markAsReadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.chatSvc.MarkAsRead(r.Context(), convID, userID, req.MessageID); err != nil {
		handleError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, nil)
}

func (h *Handler) HandleTyping(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	convID := chi.URLParam(r, "conversationID")

	var req typingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.chatSvc.HandleTyping(r.Context(), convID, userID, req.Typing); err != nil {
		handleError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, nil)
}

func (h *Handler) AddParticipant(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	convID := chi.URLParam(r, "conversationID")

	var req addParticipantRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.chatSvc.AddParticipant(r.Context(), convID, userID, req.UserID); err != nil {
		handleError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, nil)
}

func (h *Handler) RemoveParticipant(w http.ResponseWriter, r *http.Request) {
	requesterID := middleware.GetUserID(r.Context())
	convID := chi.URLParam(r, "conversationID")
	userID := chi.URLParam(r, "userID")

	if err := h.chatSvc.RemoveParticipant(r.Context(), convID, requesterID, userID); err != nil {
		handleError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, nil)
}

func (h *Handler) EditMessage(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	msgID := chi.URLParam(r, "messageID")

	var req editMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	msg, err := h.chatSvc.EditMessage(r.Context(), msgID, userID, req.Text)
	if err != nil {
		handleError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, msg)
}

func (h *Handler) DeleteMessage(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	msgID := chi.URLParam(r, "messageID")

	if err := h.chatSvc.DeleteMessage(r.Context(), msgID, userID); err != nil {
		handleError(w, err)
		return
	}
	response.JSON(w, http.StatusNoContent, nil)
}

func (h *Handler) AddReaction(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	msgID := chi.URLParam(r, "messageID")

	var req addReactionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.chatSvc.AddReaction(r.Context(), msgID, userID, req.Emoji); err != nil {
		handleError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, nil)
}

func (h *Handler) RemoveReaction(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	msgID := chi.URLParam(r, "messageID")
	emoji := chi.URLParam(r, "emoji")

	if err := h.chatSvc.RemoveReaction(r.Context(), msgID, userID, emoji); err != nil {
		handleError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, nil)
}

func (h *Handler) GetUnreadCounts(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	counts, err := h.chatSvc.GetUnreadCounts(r.Context(), userID)
	if err != nil {
		handleError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, counts)
}

// --- Helpers ---

func parsePageRequest(r *http.Request) domain.PageRequest {
	page := domain.PageRequest{}

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil {
			page.Limit = limit
		}
	}

	if cursorStr := r.URL.Query().Get("cursor"); cursorStr != "" {
		page.Cursor = decodeCursor(cursorStr)
	}

	return page
}

func encodeCursor(c *domain.Cursor) string {
	return c.CreatedAt.Format(time.RFC3339Nano) + "|" + c.ID
}

func decodeCursor(s string) *domain.Cursor {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '|' {
			t, err := time.Parse(time.RFC3339Nano, s[:i])
			if err != nil {
				return nil
			}
			return &domain.Cursor{
				CreatedAt: t,
				ID:        s[i+1:],
			}
		}
	}
	return nil
}

func handleError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		response.Error(w, http.StatusNotFound, "not found")
	case errors.Is(err, domain.ErrNotChatMember):
		response.Error(w, http.StatusForbidden, "not a member of this chat")
	case errors.Is(err, domain.ErrForbidden):
		response.Error(w, http.StatusForbidden, "forbidden")
	case errors.Is(err, domain.ErrUnauthorized):
		response.Error(w, http.StatusUnauthorized, "unauthorized")
	case errors.Is(err, domain.ErrRateLimited):
		response.Error(w, http.StatusTooManyRequests, "rate limited")
	case errors.Is(err, domain.ErrAlreadyMember):
		response.Error(w, http.StatusConflict, "user is already a member")
	case errors.Is(err, domain.ErrGroupFull):
		response.Error(w, http.StatusConflict, "group is full")
	case errors.Is(err, domain.ErrDuplicateMessage):
		response.Error(w, http.StatusConflict, "duplicate message")
	case errors.Is(err, domain.ErrReactionLimitReached):
		response.Error(w, http.StatusConflict, "reaction limit reached")
	case errors.Is(err, domain.ErrMessageDeleted):
		response.Error(w, http.StatusGone, "message has been deleted")
	case domain.IsValidationError(err):
		response.Error(w, http.StatusBadRequest, err.Error())
	default:
		response.Error(w, http.StatusInternalServerError, "internal server error")
	}
}
