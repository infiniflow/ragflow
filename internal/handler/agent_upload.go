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

// Gap B — `POST /api/v1/agents/<agent_id>/upload` (Python
// api/apps/restful_apis/agent_api.py:761-790).
//
// Mirrors the python upload_agent_file handler:
//   - @_require_canvas_access_async  → loader.LoadCanvasByID (returns
//     ErrUserCanvasNotFound for both missing and forbidden; we map that
//     to CodeOperatingError (103) with the python permission message
//     instead of the chat-path 103 "canvas not found.")
//   - single file  + ?url=  → FileService.upload_info(tenant, file, url)
//     via UploadFromURL (URL-import mode)
//   - single file  + no url  → FileService.upload_info(tenant, file)
//     via UploadInfos with a one-element slice
//   - multi file  + any url  → FileService.upload_info * N
//     via UploadInfos (Python ignores ?url= on the multi-file path)
//
// 64 MB upload cap (D3 default).

package handler

import (
	"context"
	"errors"
	"mime/multipart"

	"github.com/gin-gonic/gin"

	"ragflow/internal/common"
	"ragflow/internal/dao"
)

// uploadMaxBytes caps the multipart form body at 64 MB. The python
// reference relies on Quart/werkzeug defaults which are well above
// this; we set it explicitly so the cap is auditable and stable across
// test environments.
const uploadMaxBytes int64 = 64 << 20 // 64 MiB

// canvasNoAccessMessage mirrors the python permission error
// (api/apps/restful_apis/agent_api.py:78,89). Kept identical to
// python so existing clients can pattern-match the message text.
const canvasNoAccessMessage = "Make sure you have permission to access the agent."

// CanvasLoader loads a canvas by ID.
type CanvasLoader = canvasLoader

// AgentFileService manages agent file uploads.
type AgentFileService = agentFileService

// NewAgentHandlerForTest creates an AgentHandler for testing with mock dependencies.
func NewAgentHandlerForTest(loader CanvasLoader, fileService AgentFileService) *AgentHandler {
	return &AgentHandler{loader: loader, fileService: fileService}
}

// UploadAgentFileReq defines the multipart upload contract with path param, query param, and form files.
type UploadAgentFileReq struct {
	CanvasID string                  `uri:"canvas_id" doc:"Canvas ID"`
	URL      string                  `query:"url" form:"-" doc:"Optional URL to import file from (single-file only)"`
	Files    []*multipart.FileHeader `form:"file" doc:"Files to upload"`
}

// RequireCanvasAccess returns a Gin middleware that validates the user's access to the canvas
// from the :canvas_id path parameter BEFORE any body reading or multipart parsing occurs.
func (h *AgentHandler) RequireCanvasAccess() gin.HandlerFunc {
	return func(c *gin.Context) {
		if h == nil || h.loader == nil {
			c.Next()
			return
		}
		canvasID := c.Param("canvas_id")
		if canvasID == "" {
			common.RespondError(c, common.CodeArgumentError, "`canvas_id` is required.")
			c.Abort()
			return
		}

		if _, err := h.loader.LoadCanvasByID(c.Request.Context(), canvasID); err != nil {
			if errors.Is(err, dao.ErrUserCanvasNotFound) {
				common.RespondError(c, common.CodeOperatingError, canvasNoAccessMessage)
				c.Abort()
				return
			}
			common.RespondError(c, common.CodeServerError, err.Error())
			c.Abort()
			return
		}
		c.Next()
	}
}

// UploadAgentFile handles POST /api/v1/agents/:canvas_id/upload as a typed route.
func (h *AgentHandler) UploadAgentFile(ctx context.Context, req *UploadAgentFileReq) (any, error) {
	if req.CanvasID == "" {
		return nil, common.NewCodedError(common.CodeArgumentError, "`canvas_id` is required.")
	}

	if _, err := h.loader.LoadCanvasByID(ctx, req.CanvasID); err != nil {
		if errors.Is(err, dao.ErrUserCanvasNotFound) {
			return nil, common.NewCodedError(common.CodeOperatingError, canvasNoAccessMessage)
		}
		return nil, common.NewCodedError(common.CodeServerError, err.Error())
	}

	files := req.Files

	// URL-import mode: matches python's behaviour exactly.
	// The url query param is consulted ONLY on the single-file branch; for 0 or
	// >1 files, the url is silently ignored and the request flows into UploadInfos.
	if req.URL != "" && len(files) == 1 {
		uploaded, err := h.fileService.UploadFromURL(ctx, req.URL)
		if err != nil {
			return nil, common.NewCodedError(common.CodeServerError, err.Error())
		}
		return uploaded, nil
	}

	if len(files) == 0 {
		return nil, common.NewCodedError(common.CodeArgumentError, "`file` field is required.")
	}

	results, err := h.fileService.UploadInfos(ctx, files)
	if err != nil {
		return nil, common.NewCodedError(common.CodeServerError, err.Error())
	}

	if len(results) == 1 {
		return results[0], nil
	}
	return results, nil
}
