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

// Gap A — `GET /api/v1/agents/download` (Python
// api/apps/restful_apis/agent_api.py:523-530).
//
// Mirrors the python download_agent_file handler:
//   - auth via @login_required → GetUser
//   - tenant injection via @add_tenant_id_to_kwargs → user.TenantID (we use
//     user.ID here because the user/tenant distinction is collapsed in
//     the Go session model; FileService buckets by userID for download
//     retrieval, same as the python tenantID). See service/file.go:1033.
//   - reads `id` from query string and streams raw bytes back as
//     application/octet-stream.

package handler

import (
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"

	"ragflow/internal/common"
)

// DownloadAgentFile GET /api/v1/agents/download?id=<file_id>
func (h *AgentHandler) DownloadAgentFile(c *gin.Context) {
	user, code, msg := GetUser(c)
	if code != common.CodeSuccess {
		common.ResponseWithCodeData(c, code, nil, msg)
		return
	}
	fileID := c.Query("id")
	if fileID == "" {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil,
			"`id` is required.")
		return
	}

	// IDOR note (security review H1): the Go User struct has no
	// TenantID field — the project collapses user and tenant in a
	// single-tenant session model. Python's @add_tenant_id_to_kwargs
	// resolves tenant_id from the session, and the python download
	// endpoint also reads `id` directly from the query string with no
	// per-object ownership check, so this port preserves the python
	// shape. A future per-object ownership check should be added in
	// both the python and Go code paths.
	blob, err := h.fileService.DownloadAgentFile(user.ID, fileID)
	if err != nil {
		common.ResponseWithCodeData(c, common.CodeServerError, nil, err.Error())
		return
	}

	// Sanitize the Content-Disposition value to prevent header
	// injection (security review H2). The Go net/http layer rejects
	// CR/LF in header values, but we sanitize at the source so we
	// don't rely on the implicit defense. `filepath.Base` strips any
	// path elements; url.PathEscape produces an RFC 5987 filename*=
	// value.
	safe := filepath.Base(fileID)
	if safe == "" || safe == "." || safe == "/" || strings.ContainsAny(safe, "\r\n\"") {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil,
			"invalid file id.")
		return
	}
	c.Header("Content-Disposition", fmt.Sprintf(
		`attachment; filename="%s"; filename*=UTF-8''%s`,
		safe, url.PathEscape(safe),
	))
	c.Data(http.StatusOK, "application/octet-stream", blob)
}
