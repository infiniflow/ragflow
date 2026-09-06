package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"ragflow/internal/common"
)

func TestDatasetsHandlerListTags(t *testing.T) {
	for _, tc := range []struct {
		name string
		data [][2]interface{}
	}{
		{"pairs", [][2]interface{}{{"beta", 5}, {"alpha", 2}}},
		{"empty", [][2]interface{}{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeDatasetTagsService{pairs: tc.data}
			r := newDatasetTagsRouter(fake, true)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, datasetTagRequest(context.Background(), http.MethodGet,
				"/api/v1/datasets/"+tagDatasetID+"/tags", ""))
			assertDatasetTagResponse(t, rec, map[string]interface{}{"code": common.CodeSuccess, "data": tc.data})
			if fake.calls != 1 || fake.datasetID != tagDatasetID || fake.userID != "user-1" {
				t.Fatalf("service calls = %d, dataset = %q, user = %q", fake.calls, fake.datasetID, fake.userID)
			}
		})
	}
}
