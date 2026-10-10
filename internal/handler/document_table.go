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
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"ragflow/internal/common"
	"ragflow/internal/permission"
	"ragflow/internal/service/document"
)

// tableProbeRequestBodyLimit bounds the whole multipart request before it is
// parsed, so an oversized upload is refused without materialising it.
const tableProbeRequestBodyLimit = document.TableProbeMaxFileBytes + 1<<20

// ProbeTableColumns serves POST /datasets/:dataset_id/documents/probe-table.
// It answers with the columns of one candidate file so a client can offer real
// column names when it asks for roles; nothing is stored and no task starts.
func (h *DocumentHandler) ProbeTableColumns(c *gin.Context) {
	datasetID := c.Param("dataset_id")
	userID := c.GetString("user_id")
	ctx := c.Request.Context()

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, tableProbeRequestBodyLimit)
	form, err := c.MultipartForm()
	if err != nil {
		// Only the body that outgrew the reader is a limit; every other way a
		// multipart form fails to parse is a malformed request, and a client
		// branching on data.error must not be told to expect a smaller file.
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			common.ResponseWithCodeData(c, common.CodeResourceExhausted, tableProbeErrorData(document.TableProbeLimit),
				fmt.Sprintf("the probe request exceeds the %d-byte limit", tableProbeRequestBodyLimit))
			return
		}
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil,
			"the probe request could not be read as a multipart form; upload exactly one file")
		return
	}
	files := form.File["file"]
	if len(files) != 1 {
		common.ResponseWithCodeData(c, common.CodeArgumentError, tableProbeErrorData(document.TableProbeLimit),
			fmt.Sprintf("column probing takes exactly one file, got %d", len(files)))
		return
	}
	header := files[0]

	if err := h.datasetService.CheckAccess(ctx, permission.Subject{UserID: userID}, datasetID, permission.OperationRead); err != nil {
		common.ResponseWithCodeData(c, common.CodePermissionError, tableProbeErrorData(document.TableAccessDenied),
			fmt.Sprintf("You don't own the dataset %s.", datasetID))
		return
	}

	file, openErr := header.Open()
	if openErr != nil {
		common.ResponseWithCodeData(c, common.CodeDataError, tableProbeErrorData(document.TableProbeParseFailed),
			fmt.Sprintf("the uploaded file could not be read: %v", openErr))
		return
	}
	defer file.Close()
	// One byte past the limit distinguishes "at the limit" from "over it"
	// without buffering an unbounded file.
	data, readErr := io.ReadAll(io.LimitReader(file, document.TableProbeMaxFileBytes+1))
	if readErr != nil {
		common.ResponseWithCodeData(c, common.CodeDataError, tableProbeErrorData(document.TableProbeParseFailed),
			fmt.Sprintf("the uploaded file could not be read: %v", readErr))
		return
	}

	result, probeErr := h.documentService.ProbeTableColumns(ctx, header.Filename, data)
	if probeErr != nil {
		writeTableProbeError(c, probeErr)
		return
	}
	common.SuccessWithData(c, result, "success")
}

// GetDocumentTableColumns serves
// GET /datasets/:dataset_id/documents/:document_id/table-columns. The columns
// come from the document's stored file, which is not a claim about what the
// index currently holds: a document parsed before a role change still answers
// with what its file contains.
func (h *DocumentHandler) GetDocumentTableColumns(c *gin.Context) {
	datasetID := c.Param("dataset_id")
	documentID := c.Param("document_id")
	userID := c.GetString("user_id")
	ctx := c.Request.Context()

	if err := h.datasetService.CheckAccess(ctx, permission.Subject{UserID: userID}, datasetID, permission.OperationRead); err != nil {
		common.ResponseWithCodeData(c, common.CodePermissionError, tableProbeErrorData(document.TableAccessDenied),
			fmt.Sprintf("You don't own the dataset %s.", datasetID))
		return
	}

	result, err := h.documentService.ProbeDocumentTableColumns(ctx, datasetID, documentID)
	if err != nil {
		writeTableProbeError(c, err)
		return
	}
	common.SuccessWithData(c, result, "success")
}

// writeTableProbeError maps a probe failure onto a transport code plus the
// stable business code in data.error.
func writeTableProbeError(c *gin.Context, err error) {
	var probeErr *document.TableProbeError
	if !errors.As(err, &probeErr) {
		code := common.CodeDataError
		if errors.Is(err, io.ErrUnexpectedEOF) || strings.Contains(err.Error(), "exceeds the maximum allowed size") {
			code = common.CodeResourceExhausted
		}
		common.ResponseWithCodeData(c, code, nil, err.Error())
		return
	}
	common.ResponseWithCodeData(c, tableProbeCode(probeErr.Code), tableProbeErrorData(probeErr.Code), probeErr.Message)
}

func tableProbeCode(business string) common.ErrorCode {
	switch business {
	case "DATASET_ACCESS_DENIED":
		return common.CodePermissionError
	case document.TableProbeUnsupportedFormat:
		return common.CodeArgumentError
	case document.TableProbeHeaderNotFound, document.TableProbeParseFailed:
		return common.CodeDataError
	case document.TableProbeLimit:
		return common.CodeResourceExhausted
	case document.TableProbeTimeout:
		return common.CodeTimeoutError
	default:
		return common.CodeDataError
	}
}

func tableProbeErrorData(business string) map[string]any {
	if business == "" {
		return nil
	}
	return map[string]any{"error": business}
}
