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
// Reads the authenticated caller's downloads bucket and streams the selected
// file as application/octet-stream.

package handler

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"

	"ragflow/internal/common"
	"ragflow/internal/permission"
)

// DownloadAgentFile GET /api/v1/agents/download?id=<file_id>
func (h *AgentHandler) DownloadAgentFile(c *gin.Context) {
	fileID := c.Query("id")
	if fileID == "" {
		common.RespondError(c, common.CodeArgumentError, "`id` is required.")
		return
	}

	ctx := c.Request.Context()
	blob, err := h.fileService.DownloadAgentFile(ctx, fileID)
	if err != nil {
		if errors.Is(err, permission.ErrUnauthenticated) {
			common.RespondError(c, common.CodeUnauthorized, err.Error())
			return
		}
		common.RespondError(c, common.CodeServerError, err.Error())
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
		common.RespondError(c, common.CodeArgumentError, "invalid file id.")
		return
	}
	c.Header("Content-Disposition", fmt.Sprintf(
		`attachment; filename="%s"; filename*=UTF-8''%s`,
		safe, url.PathEscape(safe),
	))
	c.Data(http.StatusOK, "application/octet-stream", blob)
}
