// Package httpx holds the request and response helpers shared by the
// controllers: the JSON error and success bodies, mapping use case errors to
// status codes, and reading the current user, path parameters and bodies.
package httpx

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/hilthontt/visper/services/api/domain/apperr"
	"github.com/hilthontt/visper/services/api/domain/model"
	"github.com/hilthontt/visper/services/api/presentation/middlewares"
)

// Error codes of ErrorResponse.
const (
	CodeInvalidRequest = "invalid_request"
	CodeUnauthorized   = "unauthorized"
	CodeForbidden      = "forbidden"
	CodeNotFound       = "not_found"
	CodeGone           = "gone"
	CodeTooLarge       = "too_large"
	CodeInternal       = "internal_error"
)

// internalErrorMessage replaces the message of internal errors, which may
// contain storage details.
const internalErrorMessage = "something went wrong, please try again"

type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}

type SuccessResponse struct {
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// Fail writes an error response.
func Fail(ctx *gin.Context, status int, code, message string) {
	ctx.JSON(status, ErrorResponse{Error: code, Message: message})
}

// BadRequest writes a 400 invalid_request response.
func BadRequest(ctx *gin.Context, message string) {
	Fail(ctx, http.StatusBadRequest, CodeInvalidRequest, message)
}

// Error writes the response for an error returned by a use case, with the
// status code of its kind. Internal errors are recorded on the context (and
// so logged) and answered with a generic message.
func Error(ctx *gin.Context, err error) {
	kind := apperr.KindOf(err)
	status, code := statusOf(kind)
	message := err.Error()
	if kind == apperr.Internal {
		_ = ctx.Error(err)
		message = internalErrorMessage
	}
	Fail(ctx, status, code, message)
}

func statusOf(kind apperr.Kind) (int, string) {
	switch kind {
	case apperr.Invalid:
		return http.StatusBadRequest, CodeInvalidRequest
	case apperr.Unauthenticated:
		return http.StatusUnauthorized, CodeUnauthorized
	case apperr.Forbidden:
		return http.StatusForbidden, CodeForbidden
	case apperr.NotFound:
		return http.StatusNotFound, CodeNotFound
	case apperr.Gone:
		return http.StatusGone, CodeGone
	case apperr.TooLarge:
		return http.StatusRequestEntityTooLarge, CodeTooLarge
	default:
		return http.StatusInternalServerError, CodeInternal
	}
}

// RequireUser returns the user set by middlewares.UserMiddleware, or writes a
// 401 response.
func RequireUser(ctx *gin.Context) (*model.User, bool) {
	user, ok := middlewares.GetUserFromContext(ctx)
	if !ok {
		Fail(ctx, http.StatusUnauthorized, CodeUnauthorized, "user not found in context")
	}
	return user, ok
}

// RequireParam returns a non-empty path parameter, or writes a 400 response
// saying "<label> is required".
func RequireParam(ctx *gin.Context, name, label string) (string, bool) {
	value := ctx.Param(name)
	if value == "" {
		BadRequest(ctx, label+" is required")
	}
	return value, value != ""
}

// RoomID returns the ":id" path parameter, or writes a 400 response.
func RoomID(ctx *gin.Context) (string, bool) {
	return RequireParam(ctx, "id", "room ID")
}

// BindJSON decodes and validates the request body into req, or writes a 400
// response.
func BindJSON(ctx *gin.Context, req any) bool {
	if err := ctx.ShouldBindJSON(req); err != nil {
		BadRequest(ctx, middlewares.TranslateValidationError(err))
		return false
	}
	return true
}
