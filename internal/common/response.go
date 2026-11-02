package common

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Response is the standard API envelope. Data must encode as an object, array,
// or null. HTTPStatus defaults to 200 and is not part of the JSON body.
type Response struct {
	Code       ErrorCode `json:"code"`
	Message    string    `json:"message"`
	Data       any       `json:"data"`
	Total      any       `json:"total,omitempty"`
	Detail     any       `json:"detail,omitempty"`
	HTTPStatus int       `json:"-"`
}

// Respond writes an API response, rejecting scalar data before writing headers.
func Respond(c *gin.Context, response Response) {
	status := response.HTTPStatus
	if status == 0 {
		status = http.StatusOK
	}
	if response.Message == "" {
		response.Message = response.Code.Message()
		if response.Code == CodeSuccess {
			response.Message = "success"
		}
	}

	data, err := json.Marshal(response.Data)
	if err == nil && (data[0] == '{' || data[0] == '[' || string(data) == "null") {
		response.Data = json.RawMessage(data)
		var body []byte
		body, err = json.Marshal(response)
		if err == nil {
			c.Data(status, "application/json; charset=utf-8", body)
			return
		}
	}

	Warn("response payload must contain serializable object, array, or null data",
		zap.String("data_type", fmt.Sprintf("%T", response.Data)),
	)
	c.JSON(http.StatusInternalServerError, Response{
		Code:    CodeServerError,
		Message: CodeServerError.Message(),
	})
}
