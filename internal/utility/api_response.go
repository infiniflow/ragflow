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

package utility

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"ragflow/internal/common"
)

// WriteAPIResponse writes the minimal shared JSON response shape used by dataset APIs.
func WriteAPIResponse(c *gin.Context, code common.ErrorCode, message string, data interface{}) {
	WriteAPIResponseWithExtras(c, code, message, data, nil)
}

// WriteAPIResponseWithExtras writes the shared JSON response shape with optional top-level fields.
func WriteAPIResponseWithExtras(c *gin.Context, code common.ErrorCode, message string, data interface{}, extras map[string]interface{}) {
	response := gin.H{"code": code}
	if code == common.CodeSuccess {
		if data != nil {
			response["data"] = data
		}
	} else if message != "" {
		response["message"] = message
	}
	for key, value := range extras {
		response[key] = value
	}

	c.JSON(http.StatusOK, response)
}

// WriteAPISuccess writes a success response with optional data.
func WriteAPISuccess(c *gin.Context, data interface{}) {
	WriteAPIResponse(c, common.CodeSuccess, "", data)
}

// WriteAPISuccessWithExtras writes a success response with optional top-level fields.
func WriteAPISuccessWithExtras(c *gin.Context, data interface{}, extras map[string]interface{}) {
	WriteAPIResponseWithExtras(c, common.CodeSuccess, "", data, extras)
}

// WriteAPIError writes an error response with code and message.
func WriteAPIError(c *gin.Context, code common.ErrorCode, message string) {
	WriteAPIResponse(c, code, message, nil)
}
