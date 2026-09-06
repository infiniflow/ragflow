package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"ragflow/internal/common"
)

func TestDatasetsHandlerAggregateTags(t *testing.T) {
	for _, tc := range []struct {
		name string
		data []map[string]interface{}
	}{
		{"tags", []map[string]interface{}{{"value": "alpha", "count": 2}}},
		{"empty", []map[string]interface{}{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeDatasetTagsService{tags: tc.data}
			r := newDatasetTagsRouter(fake, true)
			rec := httptest.NewRecorder()
			ids := "," + tagDatasetID + ",, " + tagDatasetID + " ,"
			r.ServeHTTP(rec, datasetTagRequest(context.Background(), http.MethodGet,
				"/api/v1/datasets/tags/aggregation?dataset_ids="+url.QueryEscape(ids), ""))
			assertDatasetTagResponse(t, rec, map[string]interface{}{"code": common.CodeSuccess, "data": tc.data})
			if fake.calls != 1 || fake.userID != "user-1" || !reflect.DeepEqual(fake.datasetIDs, []string{tagDatasetID, " " + tagDatasetID + " "}) {
				t.Fatalf("service calls = %d, user = %q, dataset IDs = %v", fake.calls, fake.userID, fake.datasetIDs)
			}
		})
	}
}

func TestDatasetsHandlerAggregateTagsRequiresDatasetIDs(t *testing.T) {
	for _, query := range []string{"", "?dataset_ids=,,,"} {
		t.Run(query, func(t *testing.T) {
			fake := &fakeDatasetTagsService{}
			r := newDatasetTagsRouter(fake, true)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, datasetTagRequest(context.Background(), http.MethodGet,
				"/api/v1/datasets/tags/aggregation"+query, ""))
			assertDatasetTagResponse(t, rec, map[string]interface{}{
				"code": common.CodeDataError, "message": "Lack of dataset_ids in query parameters",
			})
			if fake.calls != 0 {
				t.Fatalf("invalid request called service %d times", fake.calls)
			}
		})
	}
}

func TestDatasetsHandlerAggregateTagsIDLimit(t *testing.T) {
	for _, tc := range []struct {
		name  string
		count int
	}{
		{"maximum accepted", 100},
		{"maximum exceeded", 101},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ids := make([]string, tc.count)
			for i := range ids {
				ids[i] = tagDatasetID
			}
			fake := &fakeDatasetTagsService{tags: []map[string]interface{}{}}
			r := newDatasetTagsRouter(fake, true)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, datasetTagRequest(context.Background(), http.MethodGet,
				"/api/v1/datasets/tags/aggregation?dataset_ids="+strings.Join(ids, ","), ""))
			if tc.count == 100 {
				assertDatasetTagResponse(t, rec, map[string]interface{}{"code": common.CodeSuccess, "data": fake.tags})
				if fake.calls != 1 || !reflect.DeepEqual(fake.datasetIDs, ids) {
					t.Fatalf("service calls = %d, dataset IDs = %v", fake.calls, fake.datasetIDs)
				}
			} else {
				assertDatasetTagResponse(t, rec, map[string]interface{}{
					"code": common.CodeArgumentError, "message": "dataset_ids must contain at most 100 IDs",
				})
				if fake.calls != 0 {
					t.Fatalf("oversized request called service %d times", fake.calls)
				}
			}
		})
	}
}
