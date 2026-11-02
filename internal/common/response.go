package common

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// ResponseOptions supplies optional HTTP status and envelope metadata.
// A zero HTTPStatus uses HTTP 200.
type ResponseOptions struct {
	HTTPStatus int
	Total      any
	Detail     any
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
	Respond(c, CodeSuccess, data, message, options...)
}

// RespondSuccess writes a successful response with optional object data.
func RespondSuccess(c *gin.Context, data any, message string, options ...ResponseOptions) {
	Respond(c, CodeSuccess, data, message, options...)
}

// RespondError writes an error code and message without data.
func RespondError(c *gin.Context, code ErrorCode, message string, options ...ResponseOptions) {
	Respond(c, code, nil, message, options...)
}

// Respond writes a response with the supplied code and data. Use it for scalar
// or mixed data, and for errors that carry a payload.
func Respond(c *gin.Context, code ErrorCode, data any, message string, options ...ResponseOptions) {
	if message == "" {
		message = code.Message()
		if code == CodeSuccess {
			message = "success"
		}
	}

	status := http.StatusOK
	body := response{Code: code, Message: message, Data: data}
	for _, option := range options {
		if option.HTTPStatus != 0 {
			status = option.HTTPStatus
		}
		if option.Total != nil {
			body.Total = option.Total
		}
		if option.Detail != nil {
			body.Detail = option.Detail
		}
	}
	c.JSON(status, body)
}
