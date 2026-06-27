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

// Gap D — `GET /api/v1/agents/attachments/<attachment_id>/download`
// (Python api/apps/restful_apis/agent_api.py:2368).
//
// Mirrors the python download_agent_attachment handler:
//   - auth via @login_required → GetUser
//   - reads `attachment_id` from the URL path (NOT a query string)
//   - default `ext` query parameter is "markdown"
//   - uses utility.CONTENT_TYPE_MAP to pick the content type, falling
//     back to "application/<ext>" for unknown extensions
//   - streams raw bytes back with a sanitized Content-Disposition

package handler

import (
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"

	"ragflow/internal/common"
	"ragflow/internal/utility"
)

// agentAttachmentFileService is the subset of FileService used by
// the attachment-download handler.
type agentAttachmentFileService interface {
	DownloadAgentFile(tenantID, location string) ([]byte, error)
}

// DownloadAttachment GET /api/v1/agents/attachments/<attachment_id>/download
func (h *AgentHandler) DownloadAttachment(c *gin.Context) {
	user, code, msg := GetUser(c)
	if code != common.CodeSuccess {
		jsonError(c, code, msg)
		return
	}
	attachmentID := c.Param("attachment_id")
	if attachmentID == "" {
		jsonError(c, common.CodeArgumentError, "`attachment_id` is required.")
		return
	}
	// Note (review F9): the plan explicitly defers attachment-id
	// shape validation to the storage layer. The python download
	// endpoint at api/apps/restful_apis/agent_api.py:2368 and the
	// existing Go DownloadAgentFile path rely on storage lookup +
	// header sanitization; we DO NOT gate on UUID here because
	// attachment IDs in storage are not guaranteed UUIDs and the
	// review found no evidence of a UUID invariant. The
	// filepath.Base + CR/LF/quote check below is the only defensive
	// layer and runs BEFORE the file-service call so an unsafe id
	// never crosses the service boundary.
	safe := filepath.Base(attachmentID)
	if safe == "" || safe == "." || safe == "/" || strings.ContainsAny(safe, "\r\n\"") {
		jsonError(c, common.CodeArgumentError, "invalid attachment id.")
		return
	}

	// Normalize the ext query once. A blank or dotted input like
	// `?ext=` or `?ext=.pdf` would otherwise produce a malformed
	// MIME type like `application/` or `application/.pdf`. Trim
	// whitespace, lowercase, strip any leading dot, then fall back
	// to markdown when the value is empty.
	ext := strings.ToLower(strings.TrimSpace(c.DefaultQuery("ext", "markdown")))
	ext = strings.TrimPrefix(ext, ".")
	if ext == "" {
		ext = "markdown"
	}

	// IDOR note: the Go User struct collapses user/tenant into one
	// identifier (same model as the python download_agent_file
	// endpoint at agent_api.py:523-530). The python attachment
	// endpoint relies on the storage bucket's tenant scoping for
	// authorisation. The Go port preserves that shape.
	if h.fileService == nil {
		jsonError(c, common.CodeServerError, "file service not configured")
		return
	}
	blob, err := h.fileService.DownloadAgentFile(user.ID, attachmentID)
	if err != nil {
		// Mirror agent_download.go error mapping — DAO/transport
		// errors collapse to a generic 102 so we don't leak storage
		// internals in the response body.
		jsonError(c, common.CodeDataError, "Attachment not found!")
		return
	}

	contentType := utility.CONTENT_TYPE_MAP[ext]
	if contentType == "" {
		// Fallback for unknown extensions — keep the wire shape
		// consistent with the python handler.
		contentType = "application/" + ext
	}
	c.Header("Content-Disposition", fmt.Sprintf(
		`attachment; filename="%s"; filename*=UTF-8''%s`,
		safe, url.PathEscape(safe),
	))
	c.Data(http.StatusOK, contentType, blob)
}
