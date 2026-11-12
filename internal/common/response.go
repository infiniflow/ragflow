package common

import (
	"net/http"
	"reflect"

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

// RespondSuccessList writes a successful response with list data and an optional total.
// Supply at most one total; omitting it leaves total out of the response.
func RespondSuccessList[T any](c *gin.Context, data []T, message string, total ...int64) {
	var options ResponseOptions
	if len(total) > 0 {
		options.Total = total[0]
	}
	Respond(c, http.StatusOK, CodeSuccess, data, message, options)
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
// Data accepts structs, maps, slices, arrays, and pointers to those types.
// Other values and typed nils are silently omitted without pre-encoding data.
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
	body := response{Code: code, Message: message}
	value := reflect.ValueOf(data)
	for value.IsValid() && value.Kind() == reflect.Pointer {
		value = value.Elem()
	}
	if value.IsValid() {
		switch value.Kind() {
		case reflect.Struct, reflect.Array:
			body.Data = data
		case reflect.Map, reflect.Slice:
			if !value.IsNil() {
				body.Data = data
			}
		}
	}
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
