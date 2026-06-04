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

	"ragflow/internal/common"
)

// DifyHandler serves the /dify/* HTTP routes used by Dify external
// knowledge base connectors. Mirrors api/apps/restful_apis/dify_retrieval_api.py.
type DifyHandler struct{}

// NewDifyHandler creates a Dify handler.
func NewDifyHandler() *DifyHandler {
	return &DifyHandler{}
}

// RetrievalHealth handles GET /api/v1/dify/retrieval/health.
//
// Returns a constant success envelope. Dify probes this endpoint to check
// whether the RAGFlow external-knowledge connector is reachable, so it must
// not gate on auth or dependent infrastructure.
//
// @Summary  Dify retrieval health check
// @Tags     dify
// @Produce  json
// @Success  200 {object} map[string]interface{}
// @Router   /api/v1/dify/retrieval/health [get]
func (h *DifyHandler) RetrievalHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeSuccess,
		"message": "success",
		"data":    true,
	})
}
