package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"ragflow/internal/common"
	"ragflow/internal/entity"
)

func newAggregateTagsHandlerRouter(authenticated bool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := &DatasetsHandler{}
	r := gin.New()
	r.GET("/api/v1/datasets/tags/aggregation", func(c *gin.Context) {
		if authenticated {
			c.Set("user", &entity.User{ID: "user-1"})
		}
		h.AggregateTags(c)
	})
	return r
}

type aggregateTagsStub struct {
	ctx    context.Context
	ids    []string
	userID string
	tags   []map[string]interface{}
	code   common.ErrorCode
	err    error
}

func (s *aggregateTagsStub) AggregateTags(ctx context.Context, ids []string, userID string) ([]map[string]interface{}, common.ErrorCode, error) {
	s.ctx, s.ids, s.userID = ctx, ids, userID
	return s.tags, s.code, s.err
}

func TestDatasetsHandlerAggregateTagsContract(t *testing.T) {
	for _, tc := range []struct {
		name, ids string
		stub      aggregateTagsStub
		want      string
		called    bool
	}{
		{"success", ",id-1,,id-2,", aggregateTagsStub{tags: []map[string]interface{}{{"value": "finance", "count": 2}}}, `{"code":0,"data":[{"count":2,"value":"finance"}]}`, true},
		{"empty", "id-1", aggregateTagsStub{tags: []map[string]interface{}{}}, `{"code":0,"data":[]}`, true},
		{"max IDs", strings.Repeat("id-1,", 100), aggregateTagsStub{tags: []map[string]interface{}{}}, `{"code":0,"data":[]}`, true},
		{"too many IDs", strings.Repeat("id-1,", 101), aggregateTagsStub{}, `{"code":101,"message":"dataset_ids must contain at most 100 IDs"}`, false},
		{"missing IDs", ",,,", aggregateTagsStub{}, `{"code":102,"message":"Lack of dataset_ids in query parameters"}`, false},
		{"no access", "id-1", aggregateTagsStub{code: common.CodeDataError, err: errors.New("no authorization")}, `{"code":102,"message":"no authorization"}`, true},
		{"backend failure", "id-1", aggregateTagsStub{code: common.CodeServerError, err: errors.New("private backend details")}, `{"code":102,"message":"Internal server error"}`, true},
		{"canceled", "id-1", aggregateTagsStub{code: common.CodeServerError, err: context.Canceled}, `{"code":102,"message":"Internal server error"}`, true},
		{"whitespace ID", " ", aggregateTagsStub{code: common.CodeDataError, err: errors.New("No authorization for dataset ' '")}, `{"code":102,"message":"No authorization for dataset ' '"}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			h := NewDatasetsHandler(nil, nil)
			h.aggregateTagsService = &tc.stub
			r := gin.New()
			r.GET("/api/v1/datasets/tags/aggregation", func(c *gin.Context) { c.Set("user", &entity.User{ID: "user-1"}); h.AggregateTags(c) })
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if tc.name == "canceled" {
				cancel()
			}
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/datasets/tags/aggregation?dataset_ids="+url.QueryEscape(tc.ids), nil).WithContext(ctx))
			if rec.Code != http.StatusOK || rec.Body.String() != tc.want {
				t.Fatalf("status=%d response=%s want=%s", rec.Code, rec.Body.String(), tc.want)
			}
			if !tc.called {
				if tc.stub.ctx != nil {
					t.Fatal("invalid request reached service")
				}
				return
			}
			var wantIDs []string
			for _, id := range strings.Split(tc.ids, ",") {
				if id != "" {
					wantIDs = append(wantIDs, id)
				}
			}
			if tc.stub.ctx != ctx || tc.stub.userID != "user-1" || !reflect.DeepEqual(tc.stub.ids, wantIDs) {
				t.Fatal("request context, identity or IDs were not forwarded")
			}
		})
	}
}

func TestDatasetsHandlerAggregateTagsRequiresDatasetIDs(t *testing.T) {
	r := newAggregateTagsHandlerRouter(true)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/datasets/tags/aggregation", nil)
	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}

	var body struct {
		Code    int         `json:"code"`
		Data    interface{} `json:"data"`
		Message string      `json:"message"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v body=%s", err, resp.Body.String())
	}
	if body.Code != int(common.CodeDataError) {
		t.Fatalf("code=%d want=%d body=%s", body.Code, common.CodeDataError, resp.Body.String())
	}
	if body.Message != "Lack of dataset_ids in query parameters" {
		t.Fatalf("message=%q want=%q", body.Message, "Lack of dataset_ids in query parameters")
	}
	if body.Data != nil {
		t.Fatalf("data=%v want nil", body.Data)
	}
}

func TestDatasetsHandlerAggregateTagsRequiresAuth(t *testing.T) {
	r := newAggregateTagsHandlerRouter(false)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/datasets/tags/aggregation?dataset_ids=123e4567-e89b-12d3-a456-426614174000", nil)
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
