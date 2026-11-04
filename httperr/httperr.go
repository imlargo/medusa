// Package httperr reports application errors over HTTP as RFC 9457 problem
// details (application/problem+json):
//
//	HTTP/1.1 409 Conflict
//	Content-Type: application/problem+json
//
//	{
//	  "type": "about:blank",
//	  "title": "Conflict",
//	  "status": 409,
//	  "detail": "a user with this email already exists",
//	  "code": "user.email_taken",
//	  "request_id": "..."
//	}
//
// The status comes from the error's apperr.Kind; code is the stable code
// clients branch on, and errors lists invalid fields. Error metadata
// (apperr.Error.WithMeta) is added as extra members.
//
// It depends only on the standard library and apperr. The medusa package
// renders problems for Gin; Write serves plain net/http handlers.
package httperr

import (
	"encoding/json"
	"maps"
	"net/http"

	"github.com/imlargo/medusa/apperr"
)

// ContentType is the media type of every problem response.
const ContentType = "application/problem+json"

// StatusClientClosedRequest is the status for a request the client abandoned
// before the server answered. It is nginx's convention; there is no IANA code
// for it. Nobody reads the response, but the status keeps these requests out
// of 5xx dashboards and alerts.
const StatusClientClosedRequest = 499

// typeBlank is the RFC 9457 type for problems without their own
// documentation page: the status and title say what happened, and code
// carries the rule.
const typeBlank = "about:blank"

var statusByKind = map[apperr.Kind]int{
	apperr.KindInternal:           http.StatusInternalServerError,
	apperr.KindInvalid:            http.StatusBadRequest,
	apperr.KindNotFound:           http.StatusNotFound,
	apperr.KindConflict:           http.StatusConflict,
	apperr.KindFailedPrecondition: http.StatusUnprocessableEntity,
	apperr.KindUnauthenticated:    http.StatusUnauthorized,
	apperr.KindPermissionDenied:   http.StatusForbidden,
	apperr.KindTooLarge:           http.StatusRequestEntityTooLarge,
	apperr.KindResourceExhausted:  http.StatusTooManyRequests,
	apperr.KindUnavailable:        http.StatusServiceUnavailable,
	apperr.KindCanceled:           StatusClientClosedRequest,
}

// Status is the HTTP status for kind. An unknown kind is a 500.
func Status(kind apperr.Kind) int {
	if status, ok := statusByKind[kind]; ok {
		return status
	}
	return http.StatusInternalServerError
}

// Problem is the response body of every failed request.
type Problem struct {
	Type      string              `json:"type"`
	Title     string              `json:"title"`
	Status    int                 `json:"status"`
	Detail    string              `json:"detail"`
	Code      string              `json:"code"`
	Errors    []apperr.FieldError `json:"errors,omitempty"`
	RequestID string              `json:"request_id,omitempty"`

	// Extensions are the error's metadata, sent as extra members. A key
	// that collides with a member above is dropped.
	Extensions map[string]any `json:"-"`
}

// NewProblem describes err as a problem. An error that is not an
// *apperr.Error is reported as apperr.From reports it: an internal error, a
// cancellation or a timeout. NewProblem(nil) is the zero Problem.
func NewProblem(err error) Problem {
	appErr := apperr.From(err)
	if appErr == nil {
		return Problem{}
	}

	status := Status(appErr.Kind())
	title := http.StatusText(status)
	if status == StatusClientClosedRequest {
		title = "Client Closed Request"
	}

	return Problem{
		Type:       typeBlank,
		Title:      title,
		Status:     status,
		Detail:     appErr.Message(),
		Code:       appErr.Code(),
		Errors:     appErr.Fields(),
		Extensions: appErr.Meta(),
	}
}

// MarshalJSON writes the members, then the extensions that do not collide
// with them.
func (p Problem) MarshalJSON() ([]byte, error) {
	type members Problem
	base, err := json.Marshal(members(p))
	if err != nil || len(p.Extensions) == 0 {
		return base, err
	}

	merged := make(map[string]any, len(p.Extensions)+8)
	maps.Copy(merged, p.Extensions)

	var fixed map[string]json.RawMessage
	if err := json.Unmarshal(base, &fixed); err != nil {
		return nil, err
	}
	for k, v := range fixed {
		merged[k] = v
	}

	return json.Marshal(merged)
}

// Write sends p as the response. Use it from plain net/http handlers; Gin
// handlers return their error and let the medusa adapters render it.
func Write(w http.ResponseWriter, p Problem) error {
	body, err := json.Marshal(p)
	if err != nil {
		return err
	}

	w.Header().Set("Content-Type", ContentType)
	w.WriteHeader(p.Status)
	_, err = w.Write(body)
	return err
}
