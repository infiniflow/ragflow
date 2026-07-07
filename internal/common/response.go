//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package common

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type response struct {
	Code    int         `json:"code"`
	Message interface{} `json:"message"`
	Data    interface{} `json:"data"`
}

// errorResponse error response
type errorResponse struct {
	Code    int         `json:"code"`
	Message interface{} `json:"message"`
}

// SuccessWithData returns success response with data
func SuccessWithData(c *gin.Context, data interface{}, message interface{}) {
	c.JSON(http.StatusOK, response{
		Code:    int(CodeSuccess),
		Data:    data,
		Message: message,
	})
}

// SuccessNoMessage returns success response without message
func SuccessNoMessage(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, response{
		Code: int(CodeSuccess),
		Data: data,
	})
}

// SuccessNoData returns success response without data
func SuccessNoData(c *gin.Context, message interface{}) {
	c.JSON(http.StatusOK, response{
		Code:    int(CodeSuccess),
		Data:    nil,
		Message: message,
	})
}

// SuccessWithMessage returns success response with message only
func SuccessWithMessage(c *gin.Context, message string) {
	c.JSON(http.StatusOK, response{
		Code:    int(CodeSuccess),
		Message: message,
	})
}

// ErrorWithCode returns error response with code and message
func ErrorWithCode(c *gin.Context, code int, message string) {
	c.JSON(http.StatusOK, errorResponse{
		Code:    code,
		Message: message,
	})
}

func ResponseWithCodeData(c *gin.Context, code ErrorCode, data interface{}, message string) {
	c.JSON(http.StatusOK, response{
		Code:    int(code),
		Data:    data,
		Message: message,
	})
}

func ResponseWithHttpCodeData(c *gin.Context, httpCode int, code ErrorCode, data interface{}, message string) {
	c.JSON(httpCode, response{
		Code:    int(code),
		Data:    data,
		Message: message,
	})
}
