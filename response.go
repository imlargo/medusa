package medusa

import (
	"math"
	"net/http"
	"reflect"

	"github.com/gin-gonic/gin"

	"github.com/imlargo/medusa/httperr"
)

// The response format. There is exactly one way to send a success and one way
// to send a failure:
//
//	{"status": 200, "success": true, "message": "ok", "data": {...}}
//	{"status": 404, "code": "NOT_FOUND", "error": "user not found", "request_id": "..."}
//
// Handlers rarely call the writers directly: they return their result and the
// Handle adapters render it.

// Default messages for the common success cases.
const (
	MessageOK      = "ok"
	MessageCreated = "created"
	MessageUpdated = "updated"
	MessageDeleted = "deleted"
)

// SuccessResponse is the JSON body of every successful request.
type SuccessResponse struct {
	Status    int    `json:"status"`
	Success   bool   `json:"success"`
	Message   string `json:"message,omitempty"`
	Data      any    `json:"data,omitempty"`
	RequestID string `json:"request_id,omitempty"`
}

// ErrorResponse is the JSON body of every failed request.
type ErrorResponse struct {
	Status    int          `json:"status"`
	Code      httperr.Code `json:"code"`
	Error     string       `json:"error"`
	Details   any          `json:"details,omitempty"`
	RequestID string       `json:"request_id,omitempty"`
}

// PagedResponse is the JSON body of a paginated list.
type PagedResponse[T any] struct {
	Status     int            `json:"status"`
	Success    bool           `json:"success"`
	Data       []T            `json:"data"`
	Pagination PaginationMeta `json:"pagination"`
	RequestID  string         `json:"request_id,omitempty"`
}

// PaginationMeta describes where the returned page sits in the full result set.
type PaginationMeta struct {
	Page       int   `json:"page"`
	PageSize   int   `json:"page_size"`
	TotalItems int64 `json:"total_items"`
	TotalPages int   `json:"total_pages"`
	HasNext    bool  `json:"has_next"`
	HasPrev    bool  `json:"has_prev"`
}

// WriteSuccess renders a successful response. The status and message spell
// out the intent at the call site; the Context helpers wrap the usual
// combinations. A nil slice is sent as [] so list endpoints always answer
// with a list.
func WriteSuccess(c *gin.Context, status int, message string, data any) {
	c.JSON(status, SuccessResponse{
		Status:    status,
		Success:   true,
		Message:   message,
		Data:      emptyListIfNil(data),
		RequestID: requestID(c),
	})
}

// emptyListIfNil turns a nil slice into an empty one. A query that matches no
// rows returns a nil slice, which encodes as null, and clients that iterate
// the field break on it.
func emptyListIfNil(data any) any {
	v := reflect.ValueOf(data)
	if v.Kind() == reflect.Slice && v.IsNil() {
		return reflect.MakeSlice(v.Type(), 0, 0).Interface()
	}
	return data
}

// WriteNoContent replies 204 with an empty body.
func WriteNoContent(c *gin.Context) {
	c.Status(http.StatusNoContent)
}

// WriteError renders err, deriving the status from it. Any error is accepted;
// unclassified ones become a 500.
//
// An internal cause is also added to Gin's error list so the access log can
// record why the request failed. That is the only place it appears: it is
// never serialized to the client.
func WriteError(c *gin.Context, err error) {
	appErr := httperr.From(err)
	if appErr == nil {
		return
	}

	if appErr.Internal != nil {
		// Recorded for the access log, not for the response body.
		_ = c.Error(appErr.Internal)
	}

	c.JSON(appErr.Status, ErrorResponse{
		Status:    appErr.Status,
		Code:      appErr.Code,
		Error:     appErr.Message,
		Details:   appErr.Details,
		RequestID: requestID(c),
	})
}

// AbortWithError renders err and stops the middleware chain. Middleware should
// use this; a handler that returns its error does not need it.
func AbortWithError(c *gin.Context, err error) {
	WriteError(c, err)
	c.Abort()
}

// OK replies 200 with data.
func (c *Context) OK(data any) {
	WriteSuccess(c.Context, http.StatusOK, MessageOK, data)
}

// Created replies 201 with the newly created resource.
func (c *Context) Created(data any) {
	WriteSuccess(c.Context, http.StatusCreated, MessageCreated, data)
}

// Updated replies 200 with the updated resource.
func (c *Context) Updated(data any) {
	WriteSuccess(c.Context, http.StatusOK, MessageUpdated, data)
}

// Deleted replies 200 confirming a deletion.
func (c *Context) Deleted() {
	WriteSuccess(c.Context, http.StatusOK, MessageDeleted, nil)
}

// NoContent replies 204 with an empty body.
func (c *Context) NoContent() {
	WriteNoContent(c.Context)
}

// Fail renders err as the response, deriving the status from the error.
//
// It is not called Error because gin.Context.Error means "record this error
// for later"; shadowing it with a method that writes the response would trap
// anyone porting Gin code.
func (c *Context) Fail(err error) {
	WriteError(c.Context, err)
}

// AbortWithFailure renders err and stops the middleware chain.
func (c *Context) AbortWithFailure(err error) {
	AbortWithError(c.Context, err)
}

// Paged replies 200 with one page of results and the metadata to walk the
// rest. An empty page is sent as [], never null. It is a function rather than
// a method because Go methods cannot have type parameters.
func Paged[T any](c *Context, data []T, page Pagination, totalItems int64) {
	if data == nil {
		data = []T{}
	}

	pageSize := page.GetPageSize(DefaultPageSize)
	currentPage := page.GetPage()
	totalPages := int(math.Ceil(float64(totalItems) / float64(pageSize)))

	c.JSON(http.StatusOK, PagedResponse[T]{
		Status:  http.StatusOK,
		Success: true,
		Data:    data,
		Pagination: PaginationMeta{
			Page:       currentPage,
			PageSize:   pageSize,
			TotalItems: totalItems,
			TotalPages: totalPages,
			HasNext:    currentPage < totalPages,
			HasPrev:    currentPage > 1,
		},
		RequestID: c.RequestID(),
	})
}
