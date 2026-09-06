package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/entity"

	"github.com/gin-gonic/gin"
)

const tagDatasetID = "123e4567-e89b-12d3-a456-426614174000"

type fakeDatasetTagsService struct {
	ctx        context.Context
	calls      int
	datasetID  string
	datasetIDs []string
	userID     string
	fromTag    string
	toTag      string
	pairs      [][2]interface{}
	tags       []map[string]interface{}
	rename     map[string]interface{}
	code       common.ErrorCode
	err        error
}

func (f *fakeDatasetTagsService) ListTags(ctx context.Context, datasetID, userID string) ([][2]interface{}, common.ErrorCode, error) {
	f.ctx, f.datasetID, f.userID = ctx, datasetID, userID
	f.calls++
	return f.pairs, f.code, f.err
}

func (f *fakeDatasetTagsService) AggregateTags(ctx context.Context, datasetIDs []string, userID string) ([]map[string]interface{}, common.ErrorCode, error) {
	f.ctx, f.datasetIDs, f.userID = ctx, datasetIDs, userID
	f.calls++
	return f.tags, f.code, f.err
}

func (f *fakeDatasetTagsService) RenameTag(ctx context.Context, datasetID, userID, fromTag, toTag string) (map[string]interface{}, common.ErrorCode, error) {
	f.ctx, f.datasetID, f.userID = ctx, datasetID, userID
	f.fromTag, f.toTag = fromTag, toTag
	f.calls++
	return f.rename, f.code, f.err
}

func newDatasetTagsRouter(fake datasetTagsService, authenticated bool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := NewDatasetsHandler(nil, nil)
	h.datasetTagsService = fake
	r := gin.New()
	if authenticated {
		r.Use(func(c *gin.Context) {
			c.Set("user", &entity.User{ID: "user-1"})
		})
	}
	r.GET("/api/v1/datasets/:dataset_id/tags", h.ListTags)
	r.PUT("/api/v1/datasets/:dataset_id/tags", h.RenameTag)
	r.GET("/api/v1/datasets/tags/aggregation", h.AggregateTags)
	return r
}

func datasetTagRequest(ctx context.Context, method, target, payload string) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(payload)).WithContext(ctx)
	if payload != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	return req
}

func assertDatasetTagResponse(t *testing.T, rec *httptest.ResponseRecorder, expected map[string]interface{}) {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("HTTP status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v, body = %s", err, rec.Body.String())
	}
	wantJSON, err := json.Marshal(expected)
	if err != nil {
		t.Fatalf("encode expected response: %v", err)
	}
	var want interface{}
	if err := json.Unmarshal(wantJSON, &want); err != nil {
		t.Fatalf("decode expected response: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("response = %s, want %s", rec.Body.String(), wantJSON)
	}
}

func datasetTagEndpoints() []struct{ name, method, target, payload string } {
	return []struct{ name, method, target, payload string }{
		{"list", http.MethodGet, "/api/v1/datasets/" + tagDatasetID + "/tags", ""},
		{"aggregate", http.MethodGet, "/api/v1/datasets/tags/aggregation?dataset_ids=" + tagDatasetID, ""},
		{"rename", http.MethodPut, "/api/v1/datasets/" + tagDatasetID + "/tags", `{"from_tag":"old","to_tag":"new"}`},
	}
}

func TestDatasetsHandlerTagsRequiresAuth(t *testing.T) {
	for _, endpoint := range datasetTagEndpoints() {
		t.Run(endpoint.name, func(t *testing.T) {
			fake := &fakeDatasetTagsService{}
			r := newDatasetTagsRouter(fake, false)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, datasetTagRequest(context.Background(), endpoint.method, endpoint.target, endpoint.payload))
			assertDatasetTagResponse(t, rec, map[string]interface{}{
				"code": common.CodeUnauthorized, "message": "User not found",
			})
			if fake.calls != 0 {
				t.Fatalf("unauthenticated request called service %d times", fake.calls)
			}
		})
	}
}

func TestDatasetsHandlerTagsServiceErrors(t *testing.T) {
	cases := []struct {
		name    string
		code    common.ErrorCode
		err     error
		want    common.ErrorCode
		message string
	}{
		{"unauthorized dataset", common.CodeDataError, errors.New("no authorization"), common.CodeDataError, "no authorization"},
		{"invalid dataset", common.CodeDataError, errors.New("invalid Dataset ID"), common.CodeDataError, "invalid Dataset ID"},
		{"invalid argument", common.CodeArgumentError, errors.New("invalid tag"), common.CodeArgumentError, "invalid tag"},
		{"store failure", common.CodeServerError, errors.New("private database connection details"), common.CodeDataError, "Internal server error"},
	}
	for _, endpoint := range datasetTagEndpoints() {
		for _, tc := range cases {
			t.Run(endpoint.name+"/"+tc.name, func(t *testing.T) {
				fake := &fakeDatasetTagsService{code: tc.code, err: tc.err}
				r := newDatasetTagsRouter(fake, true)
				rec := httptest.NewRecorder()
				r.ServeHTTP(rec, datasetTagRequest(context.Background(), endpoint.method, endpoint.target, endpoint.payload))
				assertDatasetTagResponse(t, rec, map[string]interface{}{"code": tc.want, "message": tc.message})
				if fake.calls != 1 || fake.userID != "user-1" {
					t.Fatalf("service calls = %d, user = %q", fake.calls, fake.userID)
				}
			})
		}
	}
}

func TestDatasetsHandlerTagsForwardsCanceledContext(t *testing.T) {
	for _, endpoint := range datasetTagEndpoints() {
		t.Run(endpoint.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			fake := &fakeDatasetTagsService{code: common.CodeServerError, err: context.Canceled}
			r := newDatasetTagsRouter(fake, true)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, datasetTagRequest(ctx, endpoint.method, endpoint.target, endpoint.payload))
			assertDatasetTagResponse(t, rec, map[string]interface{}{
				"code": common.CodeDataError, "message": "Internal server error",
			})
			if fake.calls != 1 || fake.ctx != ctx || !errors.Is(fake.ctx.Err(), context.Canceled) {
				t.Fatalf("request context was not forwarded: calls = %d, ctx = %v", fake.calls, fake.ctx)
			}
		})
	}
}
