package common

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// ResponseOptions supplies optional envelope metadata.
type ResponseOptions struct {
	Total  any
	Detail any
}

type response struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
	Data    any       `json:"data,omitempty"`
	Total   any       `json:"total,omitempty"`
	Detail  any       `json:"detail,omitempty"`
}

// RespondSuccessList writes a successful response with list data.
func RespondSuccessList[T any](c *gin.Context, data []T, message string, options ...ResponseOptions) {
	Respond(c, http.StatusOK, CodeSuccess, data, message, options...)
}

// RespondSuccess writes a successful response with optional data.
func RespondSuccess(c *gin.Context, data any, message string, options ...ResponseOptions) {
	Respond(c, http.StatusOK, CodeSuccess, data, message, options...)
}

// RespondError writes an error code and message without data.
func RespondError(c *gin.Context, code ErrorCode, message string, options ...ResponseOptions) {
	Respond(c, http.StatusOK, code, nil, message, options...)
}

// Respond writes a response with an HTTP status and application code. A zero
// HTTP status defaults to 200. Use RespondSuccess or RespondError for HTTP 200
// responses, and Respond for responses that need another HTTP status.
func Respond(c *gin.Context, httpStatus int, code ErrorCode, data any, message string, options ...ResponseOptions) {
	if message == "" {
		message = code.Message()
		if code == CodeSuccess {
			message = "success"
		}
	}

	if httpStatus == 0 {
		httpStatus = http.StatusOK
	}
	body := response{Code: code, Message: message, Data: data}
	for _, option := range options {
		if option.Total != nil {
			body.Total = option.Total
		}
		if option.Detail != nil {
			body.Detail = option.Detail
		}
	}
	c.JSON(httpStatus, body)
}
