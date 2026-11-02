package handlers

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/imlargo/sse"

	"github.com/imlargo/medusa"
	"github.com/imlargo/medusa/auth"
	"github.com/imlargo/medusa/httperr"
)

// EventsHandler streams server-sent events to authenticated users.
//
// The whole transport (headers, flushing, heartbeats, write deadlines,
// reconnection and replay) belongs to the sse package. What is left here is the
// only part that is application-specific: deciding who may read which topics.
type EventsHandler struct {
	broker    *sse.Broker
	lifecycle *sse.Lifecycle
	jwt       *auth.JWT[int64]
}

// NewEventsHandler wires a handler onto an existing broker. The lifecycle is
// what lets shutdown drain open streams instead of severing them.
func NewEventsHandler(broker *sse.Broker, lifecycle *sse.Lifecycle, jwtAuth *auth.JWT[int64]) *EventsHandler {
	return &EventsHandler{
		broker:    broker,
		lifecycle: lifecycle,
		jwt:       jwtAuth,
	}
}

// UserTopic is the topic a single user's events are published to.
// Publishing to user.42.notifications reaches exactly user 42.
func UserTopic(userID int64, kind string) sse.Topic {
	return sse.MustTopic(fmt.Sprintf("user.%d.%s", userID, kind))
}

// Stream is the SSE endpoint: GET /v1/events
//
// It is registered without the JWT middleware on purpose. The library's
// Authorizer runs on the raw request, which is what lets it authenticate,
// decide the subscriber's filters in one place, and set a Deadline from the
// token's own expiry, so a session ends when the credential does instead of
// outliving it.
func (h *EventsHandler) Stream() gin.HandlerFunc {
	return gin.WrapH(h.broker.Handler(
		sse.WithAuthorizer(h.authorize),
		sse.WithLifecycle(h.lifecycle),
	))
}

// authorize resolves the caller's identity from their token and confines them to
// their own topics. A subscriber asking for anything outside user.<id>.> gets it
// reported as denied rather than silently dropped.
func (h *EventsHandler) authorize(r *http.Request) (sse.Grant, error) {
	token, err := bearerToken(r)
	if err != nil {
		return sse.Grant{}, sse.Unauthorized(err.Error())
	}

	claims, err := h.jwt.Parse(token)
	if err != nil {
		return sse.Grant{}, sse.Unauthorized("invalid or expired token")
	}

	grant := sse.Grant{
		Identity: fmt.Sprintf("user:%d", claims.Subject),
		Filters:  []sse.Filter{sse.MustFilter(fmt.Sprintf("user.%d.>", claims.Subject))},
		// End the stream when the access token expires. The client reconnects
		// with a fresh token and resumes from its cursor, so nothing is missed.
		Deadline: claims.ExpiresAt,
	}

	return grant, nil
}

// Publish sends a notification to one user: POST /v1/events/publish
//
// It exists to make the endpoint above easy to try out; a real application would
// publish from the service that owns the event, not from an HTTP handler.
func (h *EventsHandler) Publish(ctx *medusa.Context, in *PublishEventRequest) (*PublishEventResponse, error) {
	userID, ok := auth.SubjectFrom[int64](ctx)
	if !ok {
		return nil, httperr.Unauthorized("authentication required")
	}

	offset, err := h.broker.Publish(ctx.Ctx(), UserTopic(userID, "notifications"), in.Data, sse.Name(in.Event))
	if err != nil {
		return nil, httperr.InternalWithMessage("could not publish the event", err)
	}

	return &PublishEventResponse{Offset: uint64(offset)}, nil
}

// PublishEventRequest is the body of a publish call.
type PublishEventRequest struct {
	Event string `json:"event" binding:"required"`
	Data  any    `json:"data"  binding:"required"`
}

// PublishEventResponse reports where the event landed in the log.
type PublishEventResponse struct {
	Offset uint64 `json:"offset"`
}

// bearerToken reads the token from the Authorization header, falling back to an
// access_token query parameter.
//
// The fallback exists because the browser EventSource API cannot set headers.
// A token in a URL is easy to leak through logs and referrers, so prefer the
// header wherever the client can send one.
func bearerToken(r *http.Request) (string, error) {
	if header := r.Header.Get("Authorization"); header != "" {
		scheme, token, found := strings.Cut(header, " ")
		if !found || !strings.EqualFold(scheme, "bearer") {
			return "", fmt.Errorf("authorization header must be in format 'Bearer <token>'")
		}
		return strings.TrimSpace(token), nil
	}

	if token := r.URL.Query().Get("access_token"); token != "" {
		return token, nil
	}

	return "", fmt.Errorf("authorization header is missing")
}
