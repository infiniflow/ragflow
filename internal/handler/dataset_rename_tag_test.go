package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/gin-gonic/gin"

	"ragflow/internal/common"
	"ragflow/internal/entity"
)

type renameTagHandlerStub struct {
	ctx    context.Context
	args   []string
	result map[string]interface{}
	code   common.ErrorCode
	err    error
}

func (s *renameTagHandlerStub) RenameTag(ctx context.Context, datasetID, userID, fromTag, toTag string) (map[string]interface{}, common.ErrorCode, error) {
	s.ctx, s.args = ctx, []string{datasetID, userID, fromTag, toTag}
	return s.result, s.code, s.err
}

func TestDatasetsHandlerRenameTagContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name, payload, want string
		code                common.ErrorCode
		err                 error
		called              bool
	}{
		{"success", `{"from_tag":" old ","to_tag":" new "}`, `{"code":0,"data":{"from":" old ","to":" new "}}`, common.CodeSuccess, nil, true},
		{"missing", `{}`, `{"code":102,"message":"Lack of from_tag or to_tag in request body"}`, 0, nil, false},
		{"malformed", `{`, `{"code":102,"message":"Lack of from_tag or to_tag in request body"}`, 0, nil, false},
		{"null body", `null`, `{"code":102,"message":"Lack of from_tag or to_tag in request body"}`, 0, nil, false},
		{"wrong type", `{"from_tag":null,"to_tag":"x"}`, `{"code":101,"message":"from_tag and to_tag must be strings"}`, 0, nil, false},
		{"blank", `{"from_tag":"x","to_tag":" "}`, `{"code":101,"message":"from_tag and to_tag must not be empty"}`, 0, nil, false},
		{"denied", `{"from_tag":" old ","to_tag":" new "}`, `{"code":102,"message":"no authorization"}`, common.CodeDataError, errors.New("no authorization"), true},
		{"backend", `{"from_tag":" old ","to_tag":" new "}`, `{"code":102,"message":"Internal server error"}`, common.CodeServerError, errors.New("private backend details"), true},
		{"canceled", `{"from_tag":" old ","to_tag":" new "}`, `{"code":102,"message":"Internal server error"}`, common.CodeServerError, context.Canceled, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stub := &renameTagHandlerStub{result: map[string]interface{}{"from": " old ", "to": " new "}, code: tc.code, err: tc.err}
			h := NewDatasetsHandler(nil, nil)
			h.renameTagService = stub
			r := gin.New()
			r.PUT("/api/v1/datasets/:dataset_id/tags", func(c *gin.Context) {
				c.Set("user", &entity.User{ID: "user-1"})
				h.RenameTag(c)
			})
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if errors.Is(tc.err, context.Canceled) {
				cancel()
			}
			req := httptest.NewRequest(http.MethodPut, "/api/v1/datasets/kb-1/tags", bytes.NewBufferString(tc.payload)).WithContext(ctx)
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			var got, want interface{}
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal([]byte(tc.want), &want); err != nil {
				t.Fatal(err)
			}
			if rec.Code != http.StatusOK || !reflect.DeepEqual(got, want) {
				t.Fatalf("status=%d body=%s want=%s", rec.Code, rec.Body.String(), tc.want)
			}
			if tc.called {
				if stub.ctx != ctx || !reflect.DeepEqual(stub.args, []string{"kb-1", "user-1", " old ", " new "}) {
					t.Fatalf("forwarded context/arguments: %v %v", stub.ctx, stub.args)
				}
			} else if stub.ctx != nil {
				t.Fatal("invalid payload called service")
			}
		})
	}
}

func TestDatasetsHandlerRenameTagRequiresAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &DatasetsHandler{}
	r := gin.New()
	r.PUT("/api/v1/datasets/:dataset_id/tags", func(c *gin.Context) {
		h.RenameTag(c)
	})

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/datasets/123e4567-e89b-12d3-a456-426614174000/tags", bytes.NewBufferString(`{"from_tag":"a","to_tag":"b"}`))
	req.Header.Set("Content-Type", "application/json")
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

func TestDatasetsHandlerRenameTagRejectsMissingFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &DatasetsHandler{}
	r := gin.New()
	r.PUT("/api/v1/datasets/:dataset_id/tags", func(c *gin.Context) {
		c.Set("user", &entity.User{ID: "user-1"})
		h.RenameTag(c)
	})

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/datasets/123e4567-e89b-12d3-a456-426614174000/tags", bytes.NewBufferString(`{"from_tag":"a"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(resp, req)

	var body struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v body=%s", err, resp.Body.String())
	}
	if body.Code != int(common.CodeDataError) {
		t.Fatalf("code=%d want=%d body=%s", body.Code, common.CodeDataError, resp.Body.String())
	}
}

func TestDatasetsHandlerRenameTagRejectsEmptyFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &DatasetsHandler{}
	r := gin.New()
	r.PUT("/api/v1/datasets/:dataset_id/tags", func(c *gin.Context) {
		c.Set("user", &entity.User{ID: "user-1"})
		h.RenameTag(c)
	})

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/datasets/123e4567-e89b-12d3-a456-426614174000/tags", bytes.NewBufferString(`{"from_tag":" ","to_tag":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(resp, req)

	var body struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v body=%s", err, resp.Body.String())
	}
	if body.Code != int(common.CodeArgumentError) {
		t.Fatalf("code=%d want=%d body=%s", body.Code, common.CodeArgumentError, resp.Body.String())
	}
	if body.Message != "from_tag and to_tag must not be empty" {
		t.Fatalf("message=%q want=%q", body.Message, "from_tag and to_tag must not be empty")
	}
}

func TestDatasetsHandlerRenameTagRejectsNonStringFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &DatasetsHandler{}
	r := gin.New()
	r.PUT("/api/v1/datasets/:dataset_id/tags", func(c *gin.Context) {
		c.Set("user", &entity.User{ID: "user-1"})
		h.RenameTag(c)
	})

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/datasets/123e4567-e89b-12d3-a456-426614174000/tags", bytes.NewBufferString(`{"from_tag":1,"to_tag":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(resp, req)

	var body struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v body=%s", err, resp.Body.String())
	}
	if body.Code != int(common.CodeArgumentError) {
		t.Fatalf("code=%d want=%d body=%s", body.Code, common.CodeArgumentError, resp.Body.String())
	}
	if body.Message != "from_tag and to_tag must be strings" {
		t.Fatalf("message=%q want=%q", body.Message, "from_tag and to_tag must be strings")
	}
}
