package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"ragflow/internal/common"
	"ragflow/internal/entity"
)

func TestDatasetsHandlerListTagsRequiresAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &DatasetsHandler{}
	r := gin.New()
	r.GET("/api/v1/datasets/:dataset_id/tags", func(c *gin.Context) {
		h.ListTags(c)
	})

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/datasets/123e4567-e89b-12d3-a456-426614174000/tags", nil)
	r.ServeHTTP(resp, req)

	var body struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v body=%s", err, resp.Body.String())
	}
	if body.Code != int(common.CodeUnauthorized) {
		t.Fatalf("code=%d want=%d body=%s", body.Code, common.CodeUnauthorized, resp.Body.String())
	}
	if body.Message != "User not found" {
		t.Fatalf("message=%q want=%q", body.Message, "User not found")
	}
}

type listTagsStub struct {
	ctx        context.Context
	id, userID string
	pairs      [][2]interface{}
	code       common.ErrorCode
	err        error
}

func (s *listTagsStub) ListTags(ctx context.Context, id, userID string) ([][2]interface{}, common.ErrorCode, error) {
	s.ctx, s.id, s.userID = ctx, id, userID
	return s.pairs, s.code, s.err
}

func TestDatasetsHandlerListTagsContract(t *testing.T) {
	for _, tc := range []struct {
		name string
		stub listTagsStub
		want string
	}{
		{"pairs", listTagsStub{pairs: [][2]interface{}{{"finance", 2}}}, `{"code":0,"data":[["finance",2]]}`},
		{"empty", listTagsStub{pairs: [][2]interface{}{}}, `{"code":0,"data":[]}`},
		{"access denied", listTagsStub{code: common.CodeDataError, err: errors.New("no authorization")}, `{"code":102,"message":"no authorization"}`},
		{"backend error", listTagsStub{code: common.CodeServerError, err: errors.New("private backend details")}, `{"code":102,"message":"Internal server error"}`},
		{"canceled", listTagsStub{code: common.CodeServerError, err: context.Canceled}, `{"code":102,"message":"Internal server error"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			h := NewDatasetsHandler(nil, nil)
			h.listTagsService = &tc.stub
			r := gin.New()
			r.GET("/api/v1/datasets/:dataset_id/tags", func(c *gin.Context) {
				c.Set("user", &entity.User{ID: "user-1"})
				h.ListTags(c)
			})
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if tc.name == "canceled" {
				cancel()
			}
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/datasets/dataset-1/tags", nil).WithContext(ctx))
			if rec.Code != http.StatusOK || rec.Body.String() != tc.want {
				t.Fatalf("status=%d response=%s want=%s", rec.Code, rec.Body.String(), tc.want)
			}
			if tc.stub.id != "dataset-1" || tc.stub.userID != "user-1" || tc.stub.ctx != ctx {
				t.Fatal("request identity or context was not forwarded")
			}
		})
	}
}
