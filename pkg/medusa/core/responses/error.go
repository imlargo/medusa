package responses

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// ErrorCode represents a machine-readable error code for API errors.
// These codes help clients programmatically handle different error scenarios.
type ErrorCode string

// Standard error codes used throughout the application.
// These codes should remain stable across API versions for backward compatibility.
const (
	// ErrBindJson indicates a JSON binding/validation error.
	// This typically occurs when request body cannot be parsed or validated.
	ErrBindJson ErrorCode = "BIND_JSON"

	// ErrNotFound indicates a requested resource was not found.
	// Returns HTTP 404 status code.
	ErrNotFound ErrorCode = "NOT_FOUND"

	// ErrInternalServer indicates an unexpected server error.
	// Returns HTTP 500 status code. Details should not expose sensitive information.
	ErrInternalServer ErrorCode = "INTERNAL_SERVER_ERROR"

	// ErrBadRequest indicates the request was malformed or contained invalid parameters.
	// Returns HTTP 400 status code.
	ErrBadRequest ErrorCode = "BAD_REQUEST"

	// ErrToManyRequests indicates the client has exceeded the rate limit.
	// Returns HTTP 429 status code. Response should include retry-after information.
	ErrToManyRequests ErrorCode = "TOO_MANY_REQUESTS"

	// ErrUnauthorized indicates authentication is required or has failed.
	// Returns HTTP 401 status code.
	ErrUnauthorized ErrorCode = "UNAUTHORIZED"
)

// ErrorResponse represents a standardized error HTTP response.
// It includes the HTTP status code, a machine-readable error code,
// a human-readable error message, and optional error details.
//
// Error Response Format:
//
//	{
//	    "status": 400,
//	    "code": "BAD_REQUEST",
//	    "error": "Invalid email format",
//	    "details": {...}
//	}
type ErrorResponse struct {
	Status  int         `json:"status"`            // HTTP status code (e.g., 400, 404, 500)
	Code    ErrorCode   `json:"code"`              // Machine-readable error code
	Error   string      `json:"error"`             // Human-readable error message
	Details interface{} `json:"details,omitempty"` // Optional error details (e.g., validation errors)
}

// ErrorBindJson writes a 400 Bad Request response for JSON binding errors.
// Use this when request body cannot be parsed or validated against the expected structure.
//
// Example:
//
//	var req CreateUserRequest
//	if err := c.ShouldBindJSON(&req); err != nil {
//	    responses.ErrorBindJson(c, err)
//	    return
//	}
func ErrorBindJson(c *gin.Context, err error) {
	WriteErrorResponse(c, http.StatusBadRequest, ErrBindJson, err.Error(), nil)
}

// ErrorNotFound writes a 404 Not Found response for a specific model/resource.
// Use this when a requested resource does not exist.
//
// Example:
//
//	user, err := db.FindUser(id)
//	if err != nil {
//	    responses.ErrorNotFound(c, "user")
//	    return
//	}
func ErrorNotFound(c *gin.Context, model string) {
	WriteErrorResponse(c, http.StatusNotFound, ErrNotFound, model+" not found", nil)
}

// ErrorInternalServer writes a 500 Internal Server Error response.
// Use this for unexpected server errors. Be careful not to expose sensitive details.
//
// Example:
//
//	if err := db.Save(user); err != nil {
//	    log.Error("Database error", zap.Error(err))
//	    responses.ErrorInternalServer(c, nil)
//	    return
//	}
func ErrorInternalServer(c *gin.Context, details interface{}) {
	WriteErrorResponse(c, http.StatusInternalServerError, ErrInternalServer, "internal server error", details)
}

// ErrorInternalServerWithMessage writes a 500 Internal Server Error response with a custom message.
// Use this when you want to provide more context about the error while still indicating a server error.
//
// Example:
//
//	responses.ErrorInternalServerWithMessage(c, "Failed to process payment", nil)
func ErrorInternalServerWithMessage(c *gin.Context, message string, details interface{}) {
	WriteErrorResponse(c, http.StatusInternalServerError, ErrInternalServer, message, details)
}

// ErrorBadRequest writes a 400 Bad Request response with a custom message.
// Use this for client errors where the request is malformed or contains invalid data.
//
// Example:
//
//	if age < 18 {
//	    responses.ErrorBadRequest(c, "User must be at least 18 years old")
//	    return
//	}
func ErrorBadRequest(c *gin.Context, message string) {
	WriteErrorResponse(c, http.StatusBadRequest, ErrBadRequest, message, nil)
}

// ErrorTooManyRequests writes a 429 Too Many Requests response.
// Use this when a client has exceeded the rate limit. The message should include
// information about when to retry.
//
// Example:
//
//	responses.ErrorTooManyRequests(c, "Rate limit exceeded. Try again in 60 seconds")
func ErrorTooManyRequests(c *gin.Context, message string) {
	WriteErrorResponse(c, http.StatusTooManyRequests, ErrToManyRequests, message, nil)
}

// ErrorUnauthorized writes a 401 Unauthorized response.
// Use this when authentication is required but missing or invalid.
//
// Example:
//
//	token := c.GetHeader("Authorization")
//	if token == "" {
//	    responses.ErrorUnauthorized(c, "Authorization header is required")
//	    return
//	}
func ErrorUnauthorized(c *gin.Context, message string) {
	WriteErrorResponse(c, http.StatusUnauthorized, ErrUnauthorized, message, nil)
}

// WriteErrorResponse writes an error JSON response with the given parameters.
// This is a low-level function; prefer using the helper functions like ErrorBadRequest
// or ErrorNotFound for common use cases.
//
// Parameters:
//   - c: The Gin context
//   - status: HTTP status code (e.g., http.StatusBadRequest)
//   - code: Machine-readable error code
//   - message: Human-readable error message
//   - details: Optional error details (can be nil)
func WriteErrorResponse(c *gin.Context, status int, code ErrorCode, message string, details interface{}) {
	c.JSON(status, ErrorResponse{
		Status:  status,
		Code:    code,
		Error:   message,
		Details: details,
	})
}
