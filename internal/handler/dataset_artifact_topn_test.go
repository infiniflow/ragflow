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

package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	dataset "ragflow/internal/service/dataset"
)

// setupArtifactGraphDB migrates the minimal schema needed for
// DatasetArtifactHandler.datasetOwner to resolve a knowledgebase owned by
// the test user.
func setupArtifactGraphDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+url.QueryEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{
		TranslateError: true,
	})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&entity.Knowledgebase{}); err != nil {
		t.Fatalf("failed to migrate test schema: %v", err)
	}
	// The DSN is a shared named in-memory database keyed on the test name, so the
	// pool owns the data: leaving it open keeps the name alive after the test ends
	// and a -count=2 re-run reuses it, hitting duplicate-key failures on insert
	// before any handler assertion runs. Close the underlying pool, not just the
	// wrapper, and keep restoring the package-level handle.
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("failed to open sqlite pool: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	origDB := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = origDB })
	return db
}

func insertArtifactGraphKB(t *testing.T, db *gorm.DB, kbID, ownerID string) {
	t.Helper()
	status := string(entity.StatusValid)
	if err := db.Create(&entity.Knowledgebase{
		ID:         kbID,
		TenantID:   ownerID,
		Name:       "artifact-graph-kb",
		EmbdID:     "BAAI/bge-large-zh-v1.5@Builtin",
		CreatedBy:  ownerID,
		Permission: string(entity.TenantPermissionMe),
		Status:     &status,
	}).Error; err != nil {
		t.Fatalf("insert knowledgebase: %v", err)
	}
}

// artifactGraphResponse mirrors the JSON envelope produced by
// common.ErrorWithCode / common.SuccessWithData for the wiki-graph endpoint.
type artifactGraphResponse struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}

// callGetArtifactGraph runs GET /datasets/{dataset_id}/artifacts/graph with
// the given query and returns the parsed envelope. The svc field is left nil
// — the handler only touches it after the top_n validation has accepted the
// request, so the invalid-input tests below stay decoupled from Elasticsearch.
func callGetArtifactGraph(t *testing.T, datasetID, query string) (int, artifactGraphResponse) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/datasets/"+datasetID+"/artifacts/graph"+query, nil)
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Set("user", &entity.User{ID: "user-1"})
	c.Params = gin.Params{{Key: "dataset_id", Value: datasetID}}
	h := &DatasetArtifactHandler{
		datasetSvc: dataset.NewDatasetService(),
	}
	h.GetArtifactGraph(c)
	var body artifactGraphResponse
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v body=%s", err, w.Body.String())
	}
	return w.Code, body
}

// TestArtifactGraph_InvalidTopNArgError pins the regression for #20006:
// a non-empty top_n that is not a parseable integer must surface as
// CodeArgumentError instead of silently falling through to the default
// entity budget.
func TestArtifactGraph_InvalidTopNArgError(t *testing.T) {
	db := setupArtifactGraphDB(t)
	// The handler's datasetOwner resolves the dataset via the dash-less
	// normalized UUID (NormalizeDatasetID), and Accessible uses the raw path
	// value, so the test passes the dash-less form and stores it that way
	// too.
	const kbID = "11111111111141118111111111111111"
	insertArtifactGraphKB(t, db, kbID, "user-1")

	cases := []struct {
		name     string
		query    string
		badToken string
	}{
		{name: "top_n=not-a-number", query: "?top_n=not-a-number", badToken: "not-a-number"},
		{name: "top_n=1.5 fractional", query: "?top_n=1.5", badToken: "1.5"},
		{name: "top_n=12abc mixed", query: "?top_n=12abc", badToken: "12abc"},
		{name: "top_n=hex literal", query: "?top_n=0x10", badToken: "0x10"},
		{name: "topN camelCase alias", query: "?topN=letters", badToken: "letters"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, body := callGetArtifactGraph(t, kbID, tc.query)
			if status != http.StatusOK {
				t.Fatalf("status=%d want 200, body=%+v", status, body)
			}
			if body.Code != int(common.CodeArgumentError) {
				t.Fatalf("code=%d want %d (CodeArgumentError); message=%q",
					body.Code, common.CodeArgumentError, body.Message)
			}
			if !strings.Contains(body.Message, "top_n must be an integer") {
				t.Fatalf("message=%q must mention top_n", body.Message)
			}
			if !strings.Contains(body.Message, tc.badToken) {
				t.Fatalf("message=%q must echo the offending value %q",
					body.Message, tc.badToken)
			}
		})
	}
}

// TestArtifactGraph_MissingTopNReachesService verifies that omitting top_n
// does not trigger the new validation: the request must pass through to the
// service layer. We don't have a real Elasticsearch here, so the call is
// expected to fail with a service-side error (CodeDataError) — but never with
// CodeArgumentError, which would mean the validation fired on empty input.
func TestArtifactGraph_MissingTopNReachesService(t *testing.T) {
	db := setupArtifactGraphDB(t)
	const kbID = "22222222222242228222222222222222"
	insertArtifactGraphKB(t, db, kbID, "user-1")

	status, body := callGetArtifactGraph(t, kbID, "")
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200, body=%+v", status, body)
	}
	if body.Code == int(common.CodeArgumentError) {
		t.Fatalf("missing top_n must not surface as CodeArgumentError; got message=%q",
			body.Message)
	}
}
