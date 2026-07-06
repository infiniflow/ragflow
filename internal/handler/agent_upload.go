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
	"net/http"

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

// UploadAgentFile POST /api/v1/agents/:canvas_id/upload
func (h *AgentHandler) UploadAgentFile(c *gin.Context) {
	user, code, msg := GetUser(c)
	if code != common.CodeSuccess {
		common.ResponseWithCodeData(c, code, nil, msg)
		return
	}
	canvasID := c.Param("canvas_id")
	if canvasID == "" {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil,
			"`canvas_id` is required.")
		return
	}

	// Canvas access check: matches python @_require_canvas_access_async.
	// We deliberately do NOT differentiate "missing" from "forbidden"
	// (LoadCanvasByID collapses both into ErrUserCanvasNotFound) for
	// IDOR mitigation; the user-visible envelope uses OPERATING_ERROR
	// (103) with the python permission message so existing clients can
	// still pattern-match the text.
	if _, err := h.loader.LoadCanvasByID(c.Request.Context(), user.ID, canvasID); err != nil {
		if err == dao.ErrUserCanvasNotFound {
			common.ResponseWithCodeData(c, common.CodeOperatingError, nil, canvasNoAccessMessage)
			return
		}
		common.ResponseWithCodeData(c, common.CodeServerError, nil, err.Error())
		return
	}

	// Hard cap the body before any parsing (security review H3).
	// Without MaxBytesReader, a 1 GB request body is fully drained
	// into memory by ParseMultipartForm before any size check fires.
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, uploadMaxBytes)
	if cl := c.Request.ContentLength; cl > uploadMaxBytes {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil,
			"request body too large.")
		return
	}
	if err := c.Request.ParseMultipartForm(uploadMaxBytes); err != nil {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil,
			"invalid multipart form: "+err.Error())
		return
	}
	defer func() {
		if c.Request.MultipartForm != nil {
			_ = c.Request.MultipartForm.RemoveAll()
		}
	}()
	form := c.Request.MultipartForm
	if form == nil {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil,
			"missing multipart form.")
		return
	}
	files := form.File["file"]

	// URL-import mode: matches python's behaviour exactly
	// (api/apps/restful_apis/agent_api.py:775-783). The url query
	// param is consulted ONLY on the single-file branch; for 0 or
	// >1 files, the url is silently ignored and the request flows
	// into the normal UploadInfos path. We replicate that with a
	// guard that dispatches to UploadFromURL only when both
	// conditions are met.
	if url := c.Query("url"); url != "" && len(files) == 1 {
		uploaded, err := h.fileService.UploadFromURL(user.ID, url)
		if err != nil {
			common.ResponseWithCodeData(c, common.CodeServerError, nil, err.Error())
			return
		}
		c.JSON(200, gin.H{
			"code":    common.CodeSuccess,
			"data":    uploaded,
			"message": "success",
		})
		return
	}

	if len(files) == 0 {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil,
			"`file` field is required.")
		return
	}

	results, err := h.fileService.UploadInfos(user.ID, files)
	if err != nil {
		common.ResponseWithCodeData(c, common.CodeServerError, nil, err.Error())
		return
	}

	// Python parity: 1 file → single dict; >1 → list.
	var payload any
	if len(results) == 1 {
		payload = results[0]
	} else {
		payload = results
	}
	c.JSON(200, gin.H{
		"code":    common.CodeSuccess,
		"data":    payload,
		"message": "success",
	})
}
