package api

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humagin"

	"github.com/imlargo/medusa"
	"github.com/imlargo/medusa/apperr"
	"github.com/imlargo/medusa/httperr"
)

// Problem is an error as a route answers it: an RFC 9457 problem, the same
// body medusa.WriteError writes. Its fields are the schema of every error
// response in the document.
type Problem struct {
	Type      string              `json:"type" doc:"A URI that identifies the kind of problem" example:"about:blank"`
	Title     string              `json:"title" doc:"The status text" example:"Conflict"`
	Status    int                 `json:"status" doc:"The HTTP status" example:"409"`
	Detail    string              `json:"detail" doc:"What went wrong, safe to show to a person"`
	Code      string              `json:"code" doc:"A stable, machine-readable code: branch on this" example:"user.email_taken"`
	Errors    []apperr.FieldError `json:"errors,omitempty" doc:"The input fields that were refused, when the problem is invalid input"`
	RequestID string              `json:"request_id,omitempty" doc:"The ID of the request, to quote when reporting the problem"`

	// extensions are the error's metadata, sent as extra members.
	extensions map[string]any
}

func (p *Problem) Error() string { return p.Code + ": " + p.Detail }

// GetStatus tells Huma which status to answer with.
func (p *Problem) GetStatus() int { return p.Status }

// ContentType tells Huma to answer as a problem, not as plain JSON.
func (p *Problem) ContentType(string) string { return httperr.ContentType }

// MarshalJSON writes the problem with its extensions as extra members, as
// httperr does.
func (p *Problem) MarshalJSON() ([]byte, error) {
	return httperr.Problem{
		Type: p.Type, Title: p.Title, Status: p.Status, Detail: p.Detail, Code: p.Code,
		Errors: p.Errors, RequestID: p.RequestID, Extensions: p.extensions,
	}.MarshalJSON()
}

// problemFrom is what a handler's error answers: its apperr problem. Like
// medusa.WriteError, it records the error on the Gin context, where the
// access log and middleware.ReportErrors read why the request failed.
func problemFrom(ctx context.Context, err error) *Problem {
	appErr := apperr.From(err)
	described := httperr.NewProblem(appErr)

	if c := ginFrom(ctx); c != nil && (appErr.Cause() != nil || described.Status >= http.StatusInternalServerError) {
		_ = c.Error(appErr)
	}

	return &Problem{
		Type: described.Type, Title: described.Title, Status: described.Status, Detail: described.Detail,
		Code: described.Code, Errors: described.Errors, RequestID: medusa.RequestIDFrom(ctx),
		extensions: described.Extensions,
	}
}

// Codes of the refusals that come from the transport, before any
// application code runs, and have no apperr kind of their own.
const (
	codeNotAcceptable    = "request.not_acceptable"
	codeUnsupportedMedia = "request.unsupported_media_type"
	codeRejected         = "request.rejected"
)

// newProblem is what Huma answers when it refuses a request itself: input
// that does not validate, a body it cannot read. It maps Huma's status and
// details to the errors Medusa already declares, so a client sees one
// vocabulary whichever layer refused it.
func newProblem(ctx huma.Context, status int, msg string, errs ...error) huma.StatusError {
	var requestContext context.Context = context.Background()
	if ctx != nil {
		requestContext = context.WithValue(ctx.Context(), ginContextKey{}, humagin.Unwrap(ctx))
	}

	switch status {
	case http.StatusUnprocessableEntity:
		return problemFrom(requestContext, medusa.ErrValidation.WithFields(fieldErrors(errs)...))
	case http.StatusBadRequest:
		return problemFrom(requestContext, medusa.ErrMalformedBody.WithFields(fieldErrors(errs)...).WithCause(errors.New(msg)))
	case http.StatusRequestEntityTooLarge:
		return problemFrom(requestContext, medusa.ErrBodyTooLarge.WithCause(errors.New(msg)))
	case http.StatusInternalServerError:
		return problemFrom(requestContext, apperr.ErrInternal.WithCause(errors.Join(append([]error{errors.New(msg)}, errs...)...)))
	}

	// Huma calls this with a zero status to learn the type of its errors.
	code := codeRejected
	switch status {
	case http.StatusNotAcceptable:
		code = codeNotAcceptable
	case http.StatusUnsupportedMediaType:
		code = codeUnsupportedMedia
	}
	return &Problem{
		Type: "about:blank", Title: http.StatusText(status), Status: status, Detail: msg,
		Code: code, RequestID: medusa.RequestIDFrom(requestContext),
	}
}

// fieldErrors turns Huma's error details into the fields a problem lists,
// named the way the client sent them: "body.items[1].price" is
// "items[1].price", "query.limit" is "limit".
func fieldErrors(errs []error) []apperr.FieldError {
	fields := make([]apperr.FieldError, 0, len(errs))
	for _, err := range errs {
		if err == nil {
			continue
		}

		detailer, ok := err.(huma.ErrorDetailer) //nolint:errorlint // Huma hands over its own detail values, unwrapped
		if !ok {
			fields = append(fields, apperr.FieldError{Message: err.Error()})
			continue
		}
		detail := detailer.ErrorDetail()

		field := detail.Location
		for _, source := range []string{"body.", "query.", "path.", "header.", "cookie."} {
			if trimmed, found := strings.CutPrefix(field, source); found {
				field = trimmed
				break
			}
		}
		if field == "body" {
			field = ""
		}
		fields = append(fields, apperr.FieldError{Field: field, Message: detail.Message})
	}
	return fields
}
