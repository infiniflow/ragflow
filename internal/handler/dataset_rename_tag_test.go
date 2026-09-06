package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"ragflow/internal/common"
)

func TestDatasetsHandlerRenameTagValidation(t *testing.T) {
	for _, tc := range []struct {
		name, payload string
		code          common.ErrorCode
		message       string
	}{
		{"missing body", "", common.CodeDataError, "Lack of from_tag or to_tag in request body"},
		{"malformed JSON", `{"from_tag":`, common.CodeDataError, "Lack of from_tag or to_tag in request body"},
		{"null body", `null`, common.CodeDataError, "Lack of from_tag or to_tag in request body"},
		{"array body", `[]`, common.CodeDataError, "Lack of from_tag or to_tag in request body"},
		{"missing from", `{"to_tag":"new"}`, common.CodeDataError, "Lack of from_tag or to_tag in request body"},
		{"missing to", `{"from_tag":"old"}`, common.CodeDataError, "Lack of from_tag or to_tag in request body"},
		{"numeric from", `{"from_tag":1,"to_tag":"new"}`, common.CodeArgumentError, "from_tag and to_tag must be strings"},
		{"null to", `{"from_tag":"old","to_tag":null}`, common.CodeArgumentError, "from_tag and to_tag must be strings"},
		{"empty from", `{"from_tag":"","to_tag":"new"}`, common.CodeArgumentError, "from_tag and to_tag must not be empty"},
		{"whitespace to", `{"from_tag":"old","to_tag":" \t "}`, common.CodeArgumentError, "from_tag and to_tag must not be empty"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeDatasetTagsService{}
			r := newDatasetTagsRouter(fake, true)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, datasetTagRequest(context.Background(), http.MethodPut,
				"/api/v1/datasets/"+tagDatasetID+"/tags", tc.payload))
			assertDatasetTagResponse(t, rec, map[string]interface{}{"code": tc.code, "message": tc.message})
			if fake.calls != 0 {
				t.Fatalf("invalid request called service %d times", fake.calls)
			}
		})
	}
}

func TestDatasetsHandlerRenameTagPreservesWhitespace(t *testing.T) {
	expected := map[string]interface{}{"from": " old ", "to": " new "}
	fake := &fakeDatasetTagsService{rename: expected}
	r := newDatasetTagsRouter(fake, true)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, datasetTagRequest(context.Background(), http.MethodPut,
		"/api/v1/datasets/"+tagDatasetID+"/tags", `{"from_tag":" old ","to_tag":" new "}`))
	assertDatasetTagResponse(t, rec, map[string]interface{}{"code": common.CodeSuccess, "data": expected})
	if fake.calls != 1 || fake.datasetID != tagDatasetID || fake.userID != "user-1" || fake.fromTag != " old " || fake.toTag != " new " {
		t.Fatalf("unexpected service arguments: %#v", fake)
	}
}
