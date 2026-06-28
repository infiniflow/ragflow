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

package handler

import (
	"net/http"

	"ragflow/internal/common"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// jsonInternalError logs the original error while returning a generic message
// to avoid exposing internal implementation details in API responses.
func jsonInternalError(c *gin.Context, err error) {
	common.Warn("handler internal error",
		zap.Error(err),
		zap.String("method", c.Request.Method),
		zap.String("path", c.Request.URL.Path),
	)
	jsonError(c, common.CodeServerError, common.CodeServerError.Message())
}

// HandleNoRoute handles requests to undefined routes
func HandleNoRoute(c *gin.Context) {
	// Python parity: GET /api/v1/auth/login/ (an empty OAuth channel) resolves
	// to a Werkzeug MethodNotAllowed in the Python API, which
	// server_error_response renders as HTTP 200 / code 100 with the
	// exception's repr() as the message. gin instead falls through to
	// NoRoute, so emit the same body here to keep the auth error paths
	// byte-for-byte aligned.
	if c.Request.Method == http.MethodGet && c.Request.URL.Path == "/api/v1/auth/login/" {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeExceptionError,
			"data":    nil,
			"message": "<MethodNotAllowed '405: Method Not Allowed'>",
		})
		return
	}

	// Log the request details on server side
	common.Logger.Warn("The requested URL was not found",
		zap.String("method", c.Request.Method),
		zap.String("path", c.Request.URL.Path),
		zap.String("query", c.Request.URL.RawQuery),
		zap.String("remote_addr", c.ClientIP()),
		zap.String("user_agent", c.Request.UserAgent()),
	)

	// Return JSON error response
	c.JSON(http.StatusNotFound, gin.H{
		"code":    404,
		"message": "Not Found: " + c.Request.URL.Path,
		"data":    nil,
		"error":   "Not Found",
	})
}
