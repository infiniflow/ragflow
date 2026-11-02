package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/service/document"

	"github.com/gin-gonic/gin"
)

func (f *fakeDocumentService) ApplyDatasetParserConfig(ctx context.Context, userID, datasetID string) (*document.ApplyDatasetParserConfigResponse, common.ErrorCode, error) {
	return &document.ApplyDatasetParserConfigResponse{DatasetID: datasetID}, common.CodeSuccess, nil
}

type applyParserConfigService struct {
	fakeDocumentService
	called bool
	code   common.ErrorCode
	err    error
	ctx    context.Context
}

func (s *applyParserConfigService) ApplyDatasetParserConfig(ctx context.Context, userID, datasetID string) (*document.ApplyDatasetParserConfigResponse, common.ErrorCode, error) {
	s.called = true
	s.ctx = ctx
	if userID != "user-1" || datasetID != "kb-1" {
		return nil, common.CodeArgumentError, errors.New("wrong user or dataset")
	}
	return &document.ApplyDatasetParserConfigResponse{DatasetID: datasetID, TotalDocuments: 3, UpdatedDocuments: 2, UnchangedDocuments: 1}, s.code, s.err
}

func TestApplyDatasetParserConfigHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name string
		auth bool
		code common.ErrorCode
		err  error
	}{
		{"success without body", true, common.CodeSuccess, nil},
		{"active ingestion", true, common.CodeConflict, errors.New("stop parsing before applying settings")},
		{"unauthenticated", false, common.CodeSuccess, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := &applyParserConfigService{code: tc.code, err: tc.err}
			h := NewDocumentHandler(svc, nil, nil)
			r := gin.New()
			r.POST("/api/v1/datasets/:dataset_id/documents/apply-parser-config", func(c *gin.Context) {
				if tc.auth {
					c.Set("user", &entity.User{ID: "user-1"})
				}
				h.ApplyDatasetParserConfig(c)
			})
			req := httptest.NewRequest(http.MethodPost, "/api/v1/datasets/kb-1/documents/apply-parser-config", nil)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			var body struct {
				Code common.ErrorCode                           `json:"code"`
				Data *document.ApplyDatasetParserConfigResponse `json:"data"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if svc.called != tc.auth {
				t.Fatalf("called=%t auth=%t", svc.called, tc.auth)
			}
			if !tc.auth {
				if body.Code == common.CodeSuccess {
					t.Fatal("unauthenticated request succeeded")
				}
				return
			}
			if body.Code != tc.code || svc.ctx != req.Context() {
				t.Fatalf("response=%s context propagated=%t", rec.Body.String(), svc.ctx == req.Context())
			}
			if tc.err == nil && (body.Data == nil || body.Data.UpdatedDocuments != 2 || body.Data.UnchangedDocuments != 1) {
				t.Fatalf("response=%s", rec.Body.String())
			}
		})
	}
}
