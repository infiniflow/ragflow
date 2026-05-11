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

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"ragflow/internal/logger"
)

// HandleNoRoute handles requests to undefined routes
func HandleNoRoute(c *gin.Context) {
	// Log the request details on server side
	logger.Logger.Warn("The requested URL was not found",
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
