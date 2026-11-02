//go:build integration

package api

import (
	"net/http"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestDatasetListPaginationFilteringAndTotals(t *testing.T) {
	prefix := uniqueName("list")
	ids := make([]string, 3)
	names := make([]string, 3)
	for i, suffix := range []string{"alpha", "bravo", "charlie"} {
		data := createRESTDataset(t, apiObject{"name": prefix + suffix})
		ids[i] = data["id"].(string)
		names[i] = data["name"].(string)
	}
	scoped := strings.Join(ids, ",")
	for _, tc := range []struct {
		name  string
		query apiObject
		want  []string
		total int
	}{
		{"first_page", apiObject{"ids": scoped, "sort": "name:asc", "page_size": 2, "page": 1}, ids[:2], 3},
		{"second_page", apiObject{"ids": scoped, "sort": "name:asc", "page_size": 2, "page": 2}, ids[2:], 3},
		{"past_end", apiObject{"ids": scoped, "sort": "name:asc", "page_size": 2, "page": 3}, []string{}, 3},
		{"descending", apiObject{"ids": scoped, "sort": "name:desc"}, []string{ids[2], ids[1], ids[0]}, 3},
		{"name", apiObject{"ids": scoped, "name": names[1]}, []string{ids[1]}, 1},
		{"keywords", apiObject{"ids": scoped, "keywords": "bravo"}, []string{ids[1]}, 1},
		{"missing_keyword", apiObject{"ids": scoped, "keywords": uniqueName("absent")}, []string{}, 0},
		{"id_name_match", apiObject{"id": ids[0], "name": names[0]}, []string{ids[0]}, 1},
		{"id_name_mismatch", apiObject{"id": ids[0], "name": names[1]}, []string{}, 0},
		{"parser_mismatch", apiObject{"ids": scoped, "parser_id": "qa"}, []string{}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := restOK(t, restCall(t, TestConfig, http.MethodGet, "/datasets", nil, tc.query))
			got := make([]string, 0)
			for _, item := range array(t, result["data"]) {
				got = append(got, object(t, item)["id"].(string))
			}
			if !reflect.DeepEqual(got, tc.want) || result["total_datasets"] != float64(tc.total) {
				t.Fatalf("listed ids=%v total=%v want ids=%v total=%d", got, result["total_datasets"], tc.want, tc.total)
			}
		})
	}
	for _, query := range []apiObject{{"page": 0}, {"page": -1}, {"page": "a"}, {"page_size": 0}, {"page_size": "a"}, {"orderby": "unknown"}, {"orderby": "CREATE_TIME"}, {"desc": "unknown"}, {"id": ""}, {"id": "not_uuid"}, {"ids": ids[0] + "," + ids[0]}, {"ids": ids[0] + ",invalid"}, {"unknown_field": "x"}} {
		restCode(t, restCall(t, TestConfig, http.MethodGet, "/datasets", nil, query), 200, 101)
	}
	restCode(t, restCall(t, TestConfig, http.MethodGet, "/datasets", nil, apiObject{"id": "00000000000040008000000000000000"}), 200, 404)
	restCode(t, restCall(t, TestConfig, http.MethodGet, "/datasets", nil, apiObject{"id": ids[0], "ids": scoped}), 200, 102)
}

func TestDatasetDeleteValidationAtomicityAndIsolation(t *testing.T) {
	first := createRESTDataset(t, nil)
	second := createRESTDataset(t, nil)
	untouched := createRESTDataset(t, nil)
	firstID, secondID := first["id"].(string), second["id"].(string)
	missingID := "00000000000040008000000000000000"
	for _, tc := range []struct {
		name string
		body apiObject
		code int
	}{
		{"empty", apiObject{"ids": []string{}}, 0},
		{"null", apiObject{"ids": nil}, 0},
		{"unset", apiObject{}, 0},
		{"duplicate", apiObject{"ids": []string{firstID, firstID}}, 101},
		{"malformed_id", apiObject{"ids": []string{"not_uuid"}}, 101},
		{"type_error", apiObject{"ids": "bad"}, 102},
		{"missing_id", apiObject{"ids": []string{missingID}}, 403},
		{"invalid_first", apiObject{"ids": []string{missingID, firstID, secondID}}, 403},
		{"invalid_middle", apiObject{"ids": []string{firstID, missingID, secondID}}, 403},
		{"invalid_last", apiObject{"ids": []string{firstID, secondID, missingID}}, 403},
		{"extra", apiObject{"unknown_field": true}, 101},
	} {
		t.Run(tc.name, func(t *testing.T) {
			restCode(t, restCall(t, TestConfig, http.MethodDelete, "/datasets", tc.body, nil), 200, tc.code)
			requireDatasetFields(t, firstID, apiObject{"name": first["name"]})
			requireDatasetFields(t, secondID, apiObject{"name": second["name"]})
		})
	}
	deleted := object(t, restOK(t, restCall(t, TestConfig, http.MethodDelete, "/datasets", apiObject{"ids": []string{firstID, secondID}}, nil))["data"])
	if deleted["success_count"] != float64(2) || len(array(t, deleted["errors"])) != 0 {
		t.Fatalf("batch result=%v", deleted)
	}
	requireDatasetAbsent(t, firstID)
	requireDatasetAbsent(t, secondID)
	restCode(t, restCall(t, TestConfig, http.MethodDelete, "/datasets", apiObject{"ids": []string{firstID, secondID}}, nil), 200, 403)
	requireDatasetFields(t, untouched["id"].(string), apiObject{"name": untouched["name"]})
}

func TestDatasetAuthorizationPreservesResources(t *testing.T) {
	dataset := createRESTDataset(t, nil)
	id := dataset["id"].(string)
	for _, key := range []string{"", TestConfig.InvalidAPIKey} {
		config := *TestConfig
		config.APIKey = key
		for _, tc := range []struct {
			method, path string
			body         any
		}{
			{http.MethodGet, "/datasets", nil}, {http.MethodGet, "/datasets/" + id, nil},
			{http.MethodPost, "/datasets", apiObject{"name": uniqueName("unauthorized"), "parse_type": 1, "parser_id": "general"}},
			{http.MethodPut, "/datasets/" + id, apiObject{"name": "unauthorized"}},
			{http.MethodDelete, "/datasets", apiObject{"ids": []string{id}}},
			{http.MethodGet, "/datasets/" + id + "/metadata/config", nil},
			{http.MethodPut, "/datasets/" + id + "/metadata/config", apiObject{"enabled": true}},
		} {
			result := restCall(t, &config, tc.method, tc.path, tc.body, nil)
			if tc.method == http.MethodPost {
				if data, ok := result.JSON["data"].(apiObject); ok {
					if createdID, ok := data["id"].(string); ok && createdID != "" {
						cleanupResource(t, TestConfig, "datasets", createdID)
					}
				}
			}
			restCode(t, result, 401, 401)
		}
		requireDatasetFields(t, id, apiObject{"name": dataset["name"], "parser_config": dataset["parser_config"]})
	}
}

func TestDatasetConcurrentReadsAndUpdates(t *testing.T) {
	dataset := createRESTDataset(t, nil)
	id := dataset["id"].(string)
	// A small deterministic set proves each request completes and the final
	// value is one complete committed update; hundreds add load, not coverage.
	names := make([]string, 4)
	var wg sync.WaitGroup
	for i := range names {
		names[i] = uniqueName("concurrent")
		wg.Go(func() {
			result := restCall(t, TestConfig, http.MethodPut, "/datasets/"+id, apiObject{"name": names[i]}, nil)
			restCode(t, result, 200, 0)
			if object(t, result.JSON["data"])["name"] != names[i] {
				t.Errorf("update response did not identify the submitted name")
			}
		})
	}
	wg.Wait()
	final := getRESTDataset(t, id)["name"]
	found := false
	for _, name := range names {
		found = found || final == name
	}
	if !found {
		t.Fatalf("torn final name=%v", final)
	}
	for range 4 {
		wg.Go(func() { requireDatasetFields(t, id, apiObject{"name": final}) })
	}
	wg.Wait()
}

func TestDatasetConcurrentCreationAndDeletion(t *testing.T) {
	ids := make([]string, 4)
	names := make([]string, len(ids))
	var wg sync.WaitGroup
	for i := range ids {
		names[i] = uniqueName("concurrent_create")
		wg.Go(func() {
			dataset := createRESTDataset(t, apiObject{"name": names[i]})
			ids[i] = dataset["id"].(string)
			if dataset["name"] != names[i] {
				t.Errorf("concurrent create returned name=%v want=%s", dataset["name"], names[i])
			}
		})
	}
	wg.Wait()
	seen := map[string]bool{}
	for i, id := range ids {
		if id == "" || seen[id] {
			t.Fatalf("concurrent creates did not return distinct IDs: %v", ids)
		}
		seen[id] = true
		requireDatasetFields(t, id, apiObject{"name": names[i]})
		wg.Go(func() {
			restOK(t, restCall(t, TestConfig, http.MethodDelete, "/datasets", apiObject{"ids": []string{id}}, nil))
			requireDatasetAbsent(t, id)
		})
	}
	wg.Wait()
}
