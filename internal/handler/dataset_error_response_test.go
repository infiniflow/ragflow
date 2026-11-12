//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"ragflow/internal/common"
	"ragflow/internal/entity"
)

// Dataset errors omit absent data. Malformed requests must receive exactly
// one JSON response, including when parseJSONRequestObject already answered.

func newDatasetErrorResponseRouter(method, path string, handlerFn func(*gin.Context)) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Handle(method, path, func(c *gin.Context) {
		c.Set("user", &entity.User{ID: "user-1"})
		handlerFn(c)
	})
	return r
}

func assertDatasetErrorBodyOmitsData(t *testing.T, r *gin.Engine, method, target, body string, wantCode common.ErrorCode) map[string]interface{} {
	t.Helper()

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(method, target, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(resp.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("response body is not one JSON object: %v body=%s", err, resp.Body.String())
	}
	if gotCode, ok := parsed["code"].(float64); !ok || int(gotCode) != int(wantCode) {
		t.Fatalf("code=%v want=%d body=%s", parsed["code"], int(wantCode), resp.Body.String())
	}
	if _, ok := parsed["data"]; ok {
		t.Fatalf("error body must omit absent data: body=%s", resp.Body.String())
	}
	if _, ok := parsed["message"].(string); !ok {
		t.Fatalf("error body lost the `message` key: body=%s", resp.Body.String())
	}
	return parsed
}

func TestCreateDatasetExtraFieldOmitsData(t *testing.T) {
	r := newDatasetErrorResponseRouter(http.MethodPost, "/api/v1/datasets", func(c *gin.Context) {
		h := &DatasetsHandler{}
		h.CreateDataset(c)
	})

	body := assertDatasetErrorBodyOmitsData(t, r, http.MethodPost, "/api/v1/datasets",
		`{"name":"kb","ext":{}}`, common.CodeArgumentError)
	if body["message"] != "extra inputs are not permitted: ext" {
		t.Fatalf("message=%v", body["message"])
	}
}

func TestCreateDatasetUnknownFieldOmitsData(t *testing.T) {
	r := newDatasetErrorResponseRouter(http.MethodPost, "/api/v1/datasets", func(c *gin.Context) {
		h := &DatasetsHandler{}
		h.CreateDataset(c)
	})

	body := assertDatasetErrorBodyOmitsData(t, r, http.MethodPost, "/api/v1/datasets",
		`{"name":"kb","bogus_field":1}`, common.CodeArgumentError)
	if body["message"] != "extra inputs are not permitted: bogus_field" {
		t.Fatalf("message=%v", body["message"])
	}
}

func TestCreateDatasetMalformedJSONOmitsData(t *testing.T) {
	r := newDatasetErrorResponseRouter(http.MethodPost, "/api/v1/datasets", func(c *gin.Context) {
		h := &DatasetsHandler{}
		h.CreateDataset(c)
	})

	assertDatasetErrorBodyOmitsData(t, r, http.MethodPost, "/api/v1/datasets",
		`{"name":`, common.CodeArgumentError)
}

func TestCreateDatasetMissingNameOmitsData(t *testing.T) {
	r := newDatasetErrorResponseRouter(http.MethodPost, "/api/v1/datasets", func(c *gin.Context) {
		h := &DatasetsHandler{}
		h.CreateDataset(c)
	})

	assertDatasetErrorBodyOmitsData(t, r, http.MethodPost, "/api/v1/datasets",
		`{"parser_config":{}}`, common.CodeDataError)
}

func TestUpdateDatasetRespondsExactlyOnceOnMalformedRequest(t *testing.T) {
	r := newDatasetErrorResponseRouter(http.MethodPut, "/api/v1/datasets/:dataset_id", func(c *gin.Context) {
		h := &DatasetsHandler{}
		h.UpdateDataset(c)
	})

	// A wrong content type fails inside parseJSONRequestObject, which already
	// writes the error response. The handler must not write a second one.
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/datasets/123e4567-e89b-12d3-a456-426614174000", bytes.NewBufferString(`{"name":"kb"}`))
	req.Header.Set("Content-Type", "text/plain")
	r.ServeHTTP(resp, req)

	var parsed map[string]interface{}
	if err := json.Unmarshal(resp.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("response body is not one JSON object (a second response was appended): %v body=%s", err, resp.Body.String())
	}
	if gotCode, ok := parsed["code"].(float64); !ok || int(gotCode) != int(common.CodeArgumentError) {
		t.Fatalf("code=%v want=%d body=%s", parsed["code"], int(common.CodeArgumentError), resp.Body.String())
	}
	if _, ok := parsed["data"]; ok {
		t.Fatalf("error body must omit absent data: body=%s", resp.Body.String())
	}
}

func TestDeleteDatasetsExtraFieldOmitsData(t *testing.T) {
	r := newDatasetErrorResponseRouter(http.MethodDelete, "/api/v1/datasets", func(c *gin.Context) {
		h := &DatasetsHandler{}
		h.DeleteDatasets(c)
	})

	body := assertDatasetErrorBodyOmitsData(t, r, http.MethodDelete, "/api/v1/datasets",
		`{"ids":["a"],"bogus":1}`, common.CodeArgumentError)
	if body["message"] != "extra inputs are not permitted: bogus" {
		t.Fatalf("message=%v", body["message"])
	}
}

func TestDeleteDatasetsDuplicateIDsOmitsData(t *testing.T) {
	r := newDatasetErrorResponseRouter(http.MethodDelete, "/api/v1/datasets", func(c *gin.Context) {
		h := &DatasetsHandler{}
		h.DeleteDatasets(c)
	})

	body := assertDatasetErrorBodyOmitsData(t, r, http.MethodDelete, "/api/v1/datasets",
		`{"ids":["a","a"]}`, common.CodeArgumentError)
	if body["message"] != "duplicate ids: 'a'" {
		t.Fatalf("message=%v", body["message"])
	}
}

func TestDeleteDatasetsRespondsExactlyOnceOnMalformedRequest(t *testing.T) {
	r := newDatasetErrorResponseRouter(http.MethodDelete, "/api/v1/datasets", func(c *gin.Context) {
		h := &DatasetsHandler{}
		h.DeleteDatasets(c)
	})

	// A wrong content type fails inside parseJSONRequestObject, which already
	// writes the error response. The handler must not write a second one.
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/datasets", bytes.NewBufferString(`{"ids":["a"]}`))
	req.Header.Set("Content-Type", "text/plain")
	r.ServeHTTP(resp, req)

	var parsed map[string]interface{}
	if err := json.Unmarshal(resp.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("response body is not one JSON object (a second response was appended): %v body=%s", err, resp.Body.String())
	}
	if _, ok := parsed["data"]; ok {
		t.Fatalf("error body must omit absent data: body=%s", resp.Body.String())
	}
}
