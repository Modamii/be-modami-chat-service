package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"be-modami-chat-service/internal/adapter/handler/middleware"
	"be-modami-chat-service/internal/domain"
	"be-modami-chat-service/internal/service"

	"github.com/go-chi/chi/v5"
	"gitlab.com/lifegoeson-libs/pkg-gokit/apperror"
	"gitlab.com/lifegoeson-libs/pkg-gokit/response"
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

// ---------------------------------------------------------------------------
// Request / Response types (also used as Swagger schema sources)
// ---------------------------------------------------------------------------

type sendMessageRequest struct {
	// Message type: text, image, video, file, or audio.
	Type string `json:"type" example:"text"`
	// Plain text body. Required when type=text.
	Text string `json:"text" example:"Hello!"`
	// Optional list of mentioned user IDs.
	Mentions []string `json:"mentions,omitempty" example:"user_abc,user_xyz"`
	// Media attachment. Required when type != text.
	Media *mediaInfoRequest `json:"media,omitempty"`
	// Optional reply reference.
	ReplyTo *replyToRequest `json:"reply_to,omitempty"`
}

type mediaInfoRequest struct {
	URL          string `json:"url"            example:"https://cdn.example.com/photo.jpg"`
	ThumbnailURL string `json:"thumbnail_url"  example:"https://cdn.example.com/photo_thumb.jpg"`
	MIMEType     string `json:"mime_type"      example:"image/jpeg"`
	FileSize     int64  `json:"file_size"      example:"204800"`
	FileName     string `json:"file_name"      example:"photo.jpg"`
	Width        int    `json:"width"          example:"1280"`
	Height       int    `json:"height"         example:"720"`
}

type replyToRequest struct {
	MessageID      string `json:"message_id"      example:"66a1b2c3d4e5f6a7b8c9d0e1"`
	SenderID       string `json:"sender_id"       example:"user_abc"`
	ContentPreview string `json:"content_preview" example:"How are you?"`
}

type createDirectChatRequest struct {
	UserID string `json:"user_id" example:"user_xyz"`
}

type createGroupRequest struct {
	Name      string   `json:"name"       example:"Team Alpha"`
	MemberIDs []string `json:"member_ids" example:"user_abc,user_xyz"`
}

type markAsReadRequest struct {
	MessageID string `json:"message_id" example:"66a1b2c3d4e5f6a7b8c9d0e1"`
}

type typingRequest struct {
	Typing bool `json:"typing" example:"true"`
}

type addParticipantRequest struct {
	UserID string `json:"user_id" example:"user_xyz"`
}

type addReactionRequest struct {
	Emoji string `json:"emoji" example:"👍"`
}

type editMessageRequest struct {
	Text string `json:"text" example:"Updated message text"`
}

// cursorPagedData wraps cursor-paginated results for the standard response envelope.
type cursorPagedData struct {
	Items      any    `json:"items"`
	NextCursor string `json:"next_cursor,omitempty"`
	HasMore    bool   `json:"has_more"`
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

// ListConversations godoc
//
//	@Summary      List conversations
//	@Description  Returns the caller's conversations ordered by most-recently updated.
//	@Tags         Conversations
//	@Produce      json
//	@Security     BearerAuth
//	@Param        cursor  query     string  false  "Pagination cursor from previous response"
//	@Param        limit   query     int     false  "Page size (default 50, max 100)"
//	@Success      200     {object}  response.Response{data=cursorPagedData}
//	@Failure      401     {object}  response.Response
//	@Failure      500     {object}  response.Response
//	@Router       /conversations [get]
func (h *Handler) ListConversations(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	page := parsePageRequest(r)

	result, err := h.chatSvc.GetConversations(r.Context(), userID, page)
	if err != nil {
		handleError(w, err)
		return
	}

	data := cursorPagedData{
		Items:   result.Items,
		HasMore: result.HasMore,
	}
	if result.NextCursor != nil {
		data.NextCursor = encodeCursor(result.NextCursor)
	}
	response.OK(w, data)
}

// GetConversation godoc
//
//	@Summary      Get a conversation
//	@Description  Returns a single conversation. The caller must be a member.
//	@Tags         Conversations
//	@Produce      json
//	@Security     BearerAuth
//	@Param        conversationID  path      string  true  "Conversation ID"
//	@Success      200             {object}  response.Response{data=domain.Conversation}
//	@Failure      401             {object}  response.Response
//	@Failure      403             {object}  response.Response  "Not a member"
//	@Failure      404             {object}  response.Response
//	@Router       /conversations/{conversationID} [get]
func (h *Handler) GetConversation(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	convID := chi.URLParam(r, "conversationID")

	conv, err := h.chatSvc.GetConversation(r.Context(), convID, userID)
	if err != nil {
		handleError(w, err)
		return
	}
	response.OK(w, conv)
}

// CreateDirectChat godoc
//
//	@Summary      Create or get a direct chat
//	@Description  Opens a 1-to-1 conversation with another user. Idempotent: returns the existing conversation if one already exists.
//	@Tags         Conversations
//	@Accept       json
//	@Produce      json
//	@Security     BearerAuth
//	@Param        body  body      createDirectChatRequest  true  "Target user"
//	@Success      201   {object}  response.Response{data=domain.Conversation}
//	@Failure      400   {object}  response.Response
//	@Failure      401   {object}  response.Response
//	@Router       /conversations/direct [post]
func (h *Handler) CreateDirectChat(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	var req createDirectChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "invalid request body")
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
	response.Created(w, conv)
}

// CreateGroup godoc
//
//	@Summary      Create a group conversation
//	@Description  Creates a new group chat. The caller is added as admin. Requires at least 1 additional member (max 49).
//	@Tags         Conversations
//	@Accept       json
//	@Produce      json
//	@Security     BearerAuth
//	@Param        body  body      createGroupRequest  true  "Group details"
//	@Success      201   {object}  response.Response{data=domain.Conversation}
//	@Failure      400   {object}  response.Response
//	@Failure      401   {object}  response.Response
//	@Failure      409   {object}  response.Response  "Group is full"
//	@Router       /conversations/group [post]
func (h *Handler) CreateGroup(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	var req createGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "invalid request body")
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
	response.Created(w, conv)
}

// GetMessages godoc
//
//	@Summary      List messages in a conversation
//	@Description  Returns messages ordered newest-first. Use the returned cursor for pagination. The caller must be a member.
//	@Tags         Messages
//	@Produce      json
//	@Security     BearerAuth
//	@Param        conversationID  path      string  true   "Conversation ID"
//	@Param        cursor          query     string  false  "Pagination cursor"
//	@Param        limit           query     int     false  "Page size (default 50, max 100)"
//	@Success      200             {object}  response.Response{data=cursorPagedData}
//	@Failure      401             {object}  response.Response
//	@Failure      403             {object}  response.Response  "Not a member"
//	@Failure      404             {object}  response.Response
//	@Router       /conversations/{conversationID}/messages [get]
func (h *Handler) GetMessages(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	convID := chi.URLParam(r, "conversationID")
	page := parsePageRequest(r)

	result, err := h.chatSvc.GetMessages(r.Context(), convID, userID, page)
	if err != nil {
		handleError(w, err)
		return
	}

	data := cursorPagedData{
		Items:   result.Items,
		HasMore: result.HasMore,
	}
	if result.NextCursor != nil {
		data.NextCursor = encodeCursor(result.NextCursor)
	}
	response.OK(w, data)
}

// SendMessage godoc
//
//	@Summary      Send a message
//	@Description  Sends a message to a conversation. The caller must be a member. Subject to per-user rate limiting.
//	@Tags         Messages
//	@Accept       json
//	@Produce      json
//	@Security     BearerAuth
//	@Param        conversationID  path      string             true  "Conversation ID"
//	@Param        body            body      sendMessageRequest true  "Message payload"
//	@Success      201             {object}  response.Response{data=domain.Message}
//	@Failure      400             {object}  response.Response
//	@Failure      401             {object}  response.Response
//	@Failure      403             {object}  response.Response  "Not a member or admin-only conversation"
//	@Failure      404             {object}  response.Response
//	@Failure      409             {object}  response.Response  "Duplicate message"
//	@Failure      429             {object}  response.Response  "Rate limited"
//	@Router       /conversations/{conversationID}/messages [post]
func (h *Handler) SendMessage(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	convID := chi.URLParam(r, "conversationID")

	var req sendMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "invalid request body")
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
	}

	if req.Media != nil {
		cmd.Media = &domain.MediaInfo{
			URL:          req.Media.URL,
			ThumbnailURL: req.Media.ThumbnailURL,
			MIMEType:     req.Media.MIMEType,
			FileSize:     req.Media.FileSize,
			FileName:     req.Media.FileName,
			Width:        req.Media.Width,
			Height:       req.Media.Height,
		}
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
	response.Created(w, msg)
}

// MarkAsRead godoc
//
//	@Summary      Mark messages as read
//	@Description  Updates the caller's read cursor to the given message. Clears the unread count for this conversation.
//	@Tags         Messages
//	@Accept       json
//	@Produce      json
//	@Security     BearerAuth
//	@Param        conversationID  path      string           true  "Conversation ID"
//	@Param        body            body      markAsReadRequest true  "Last read message"
//	@Success      204
//	@Failure      400  {object}  response.Response
//	@Failure      401  {object}  response.Response
//	@Failure      403  {object}  response.Response
//	@Failure      404  {object}  response.Response
//	@Router       /conversations/{conversationID}/read [post]
func (h *Handler) MarkAsRead(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	convID := chi.URLParam(r, "conversationID")

	var req markAsReadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "invalid request body")
		return
	}

	if err := h.chatSvc.MarkAsRead(r.Context(), convID, userID, req.MessageID); err != nil {
		handleError(w, err)
		return
	}
	response.NoContent(w)
}

// HandleTyping godoc
//
//	@Summary      Send typing indicator
//	@Description  Broadcasts a typing event to other conversation members via Centrifugo.
//	@Tags         Presence
//	@Accept       json
//	@Produce      json
//	@Security     BearerAuth
//	@Param        conversationID  path      string         true  "Conversation ID"
//	@Param        body            body      typingRequest  true  "Typing state"
//	@Success      204
//	@Failure      400  {object}  response.Response
//	@Failure      401  {object}  response.Response
//	@Failure      403  {object}  response.Response
//	@Router       /conversations/{conversationID}/typing [post]
func (h *Handler) HandleTyping(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	convID := chi.URLParam(r, "conversationID")

	var req typingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "invalid request body")
		return
	}

	if err := h.chatSvc.HandleTyping(r.Context(), convID, userID, req.Typing); err != nil {
		handleError(w, err)
		return
	}
	response.NoContent(w)
}

// AddParticipant godoc
//
//	@Summary      Add a participant to a group
//	@Description  Adds a user to a group conversation. Requires admin role. Max group size is 50.
//	@Tags         Participants
//	@Accept       json
//	@Produce      json
//	@Security     BearerAuth
//	@Param        conversationID  path      string                 true  "Conversation ID"
//	@Param        body            body      addParticipantRequest  true  "User to add"
//	@Success      204
//	@Failure      400  {object}  response.Response
//	@Failure      401  {object}  response.Response
//	@Failure      403  {object}  response.Response  "Not an admin"
//	@Failure      404  {object}  response.Response
//	@Failure      409  {object}  response.Response  "Already a member or group is full"
//	@Router       /conversations/{conversationID}/participants [post]
func (h *Handler) AddParticipant(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	convID := chi.URLParam(r, "conversationID")

	var req addParticipantRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "invalid request body")
		return
	}

	if err := h.chatSvc.AddParticipant(r.Context(), convID, userID, req.UserID); err != nil {
		handleError(w, err)
		return
	}
	response.NoContent(w)
}

// RemoveParticipant godoc
//
//	@Summary      Remove a participant from a group
//	@Description  Removes a user from a group. An admin can remove any member; a member can remove themselves (leave).
//	@Tags         Participants
//	@Produce      json
//	@Security     BearerAuth
//	@Param        conversationID  path      string  true  "Conversation ID"
//	@Param        userID          path      string  true  "User ID to remove"
//	@Success      204
//	@Failure      401  {object}  response.Response
//	@Failure      403  {object}  response.Response
//	@Failure      404  {object}  response.Response
//	@Router       /conversations/{conversationID}/participants/{userID} [delete]
func (h *Handler) RemoveParticipant(w http.ResponseWriter, r *http.Request) {
	requesterID := middleware.GetUserID(r.Context())
	convID := chi.URLParam(r, "conversationID")
	userID := chi.URLParam(r, "userID")

	if err := h.chatSvc.RemoveParticipant(r.Context(), convID, requesterID, userID); err != nil {
		handleError(w, err)
		return
	}
	response.NoContent(w)
}

// EditMessage godoc
//
//	@Summary      Edit a message
//	@Description  Updates the text of a message. Only the original sender may edit. Only text messages can be edited.
//	@Tags         Messages
//	@Accept       json
//	@Produce      json
//	@Security     BearerAuth
//	@Param        messageID  path      string              true  "Message ID"
//	@Param        body       body      editMessageRequest  true  "New text"
//	@Success      200        {object}  response.Response{data=domain.Message}
//	@Failure      400        {object}  response.Response
//	@Failure      401        {object}  response.Response
//	@Failure      403        {object}  response.Response  "Not the sender"
//	@Failure      404        {object}  response.Response
//	@Failure      410        {object}  response.Response  "Message has been deleted"
//	@Router       /messages/{messageID} [put]
func (h *Handler) EditMessage(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	msgID := chi.URLParam(r, "messageID")

	var req editMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "invalid request body")
		return
	}

	msg, err := h.chatSvc.EditMessage(r.Context(), msgID, userID, req.Text)
	if err != nil {
		handleError(w, err)
		return
	}
	response.OK(w, msg)
}

// DeleteMessage godoc
//
//	@Summary      Delete a message
//	@Description  Soft-deletes a message (content is cleared; the record remains). Only the original sender may delete.
//	@Tags         Messages
//	@Produce      json
//	@Security     BearerAuth
//	@Param        messageID  path  string  true  "Message ID"
//	@Success      204
//	@Failure      401  {object}  response.Response
//	@Failure      403  {object}  response.Response
//	@Failure      404  {object}  response.Response
//	@Router       /messages/{messageID} [delete]
func (h *Handler) DeleteMessage(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	msgID := chi.URLParam(r, "messageID")

	if err := h.chatSvc.DeleteMessage(r.Context(), msgID, userID); err != nil {
		handleError(w, err)
		return
	}
	response.NoContent(w)
}

// AddReaction godoc
//
//	@Summary      Add an emoji reaction
//	@Description  Reacts to a message with an emoji. A message may have at most 20 distinct emojis and 50 users per emoji.
//	@Tags         Reactions
//	@Accept       json
//	@Produce      json
//	@Security     BearerAuth
//	@Param        messageID  path      string              true  "Message ID"
//	@Param        body       body      addReactionRequest  true  "Emoji"
//	@Success      204
//	@Failure      400  {object}  response.Response
//	@Failure      401  {object}  response.Response
//	@Failure      403  {object}  response.Response
//	@Failure      404  {object}  response.Response
//	@Failure      409  {object}  response.Response  "Reaction limit reached"
//	@Router       /messages/{messageID}/reactions [post]
func (h *Handler) AddReaction(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	msgID := chi.URLParam(r, "messageID")

	var req addReactionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "invalid request body")
		return
	}

	if err := h.chatSvc.AddReaction(r.Context(), msgID, userID, req.Emoji); err != nil {
		handleError(w, err)
		return
	}
	response.NoContent(w)
}

// RemoveReaction godoc
//
//	@Summary      Remove an emoji reaction
//	@Description  Removes the caller's reaction with the given emoji from a message.
//	@Tags         Reactions
//	@Produce      json
//	@Security     BearerAuth
//	@Param        messageID  path  string  true  "Message ID"
//	@Param        emoji      path  string  true  "URL-encoded emoji character"
//	@Success      204
//	@Failure      401  {object}  response.Response
//	@Failure      403  {object}  response.Response
//	@Failure      404  {object}  response.Response
//	@Router       /messages/{messageID}/reactions/{emoji} [delete]
func (h *Handler) RemoveReaction(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	msgID := chi.URLParam(r, "messageID")
	emoji := chi.URLParam(r, "emoji")

	if err := h.chatSvc.RemoveReaction(r.Context(), msgID, userID, emoji); err != nil {
		handleError(w, err)
		return
	}
	response.NoContent(w)
}

// GetUnreadCounts godoc
//
//	@Summary      Get unread message counts
//	@Description  Returns a map of conversation_id → unread_count for the caller. Counts are served from Redis cache.
//	@Tags         Messages
//	@Produce      json
//	@Security     BearerAuth
//	@Success      200  {object}  response.Response{data=map[string]int64}  "conversation_id to unread count"
//	@Failure      401  {object}  response.Response
//	@Router       /unread [get]
func (h *Handler) GetUnreadCounts(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	counts, err := h.chatSvc.GetUnreadCounts(r.Context(), userID)
	if err != nil {
		handleError(w, err)
		return
	}
	response.OK(w, counts)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

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
		response.Err(w, apperror.New(apperror.CodeNotFound, "not found"))
	case errors.Is(err, domain.ErrNotChatMember):
		response.Err(w, apperror.New(apperror.CodeForbidden, "not a member of this chat"))
	case errors.Is(err, domain.ErrForbidden):
		response.Err(w, apperror.New(apperror.CodeForbidden, "forbidden"))
	case errors.Is(err, domain.ErrUnauthorized):
		response.Err(w, apperror.New(apperror.CodeUnauthorized, "unauthorized"))
	case errors.Is(err, domain.ErrRateLimited):
		response.Err(w, apperror.New(apperror.CodeTooManyRequest, "rate limited"))
	case errors.Is(err, domain.ErrAlreadyMember):
		response.Err(w, apperror.New(apperror.CodeConflict, "user is already a member"))
	case errors.Is(err, domain.ErrGroupFull):
		response.Err(w, apperror.New(apperror.CodeConflict, "group is full"))
	case errors.Is(err, domain.ErrDuplicateMessage):
		response.Err(w, apperror.New(apperror.CodeConflict, "duplicate message"))
	case errors.Is(err, domain.ErrReactionLimitReached):
		response.Err(w, apperror.New(apperror.CodeConflict, "reaction limit reached"))
	case errors.Is(err, domain.ErrMessageDeleted):
		response.Err(w, apperror.New(apperror.CodeGone, "message has been deleted"))
	case domain.IsValidationError(err):
		response.Err(w, apperror.New(apperror.CodeBadRequest, err.Error()))
	default:
		response.InternalError(w, "internal server error")
	}
}
