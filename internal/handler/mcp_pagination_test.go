package handler

import (
	"fmt"
	"testing"
)

func fakeDatasetPager(total int, pageSize int) (func(page, size int) ([]map[string]interface{}, int64, error), *[]int) {
	pagesRequested := []int{}
	page := func(p, size int) ([]map[string]interface{}, int64, error) {
		pagesRequested = append(pagesRequested, p)
		start := (p - 1) * size
		end := start + size
		if end > total {
			end = total
		}
		if start >= total {
			return nil, int64(total), nil
		}
		data := make([]map[string]interface{}, 0, end-start)
		for i := start; i < end; i++ {
			data = append(data, map[string]interface{}{"id": fmt.Sprintf("dataset-%d", i)})
		}
		return data, int64(total), nil
	}
	return page, &pagesRequested
}

func TestFetchAllDatasetIDsStopsAtTotalWithoutEmptyPage(t *testing.T) {
	const pageSize = 100
	tests := []struct {
		name        string
		total       int
		wantPages   []int
		wantIDCount int
	}{
		{"exact multiple of page size", 200, []int{1, 2}, 200},
		{"non multiple of page size", 250, []int{1, 2, 3}, 250},
		{"single full page", 100, []int{1}, 100},
		{"single short page", 42, []int{1}, 42},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page, pagesRequested := fakeDatasetPager(tt.total, pageSize)

			ids, err := fetchAllDatasetIDs(page, pageSize)

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(ids) != tt.wantIDCount {
				t.Fatalf("got %d ids, want %d", len(ids), tt.wantIDCount)
			}
			if len(*pagesRequested) != len(tt.wantPages) {
				t.Fatalf("requested pages %v, want %v", *pagesRequested, tt.wantPages)
			}
			for i, p := range tt.wantPages {
				if (*pagesRequested)[i] != p {
					t.Fatalf("requested pages %v, want %v", *pagesRequested, tt.wantPages)
				}
			}
		})
	}
}

func TestFetchAllDatasetIDsPropagatesErrors(t *testing.T) {
	wantErr := fmt.Errorf("backend unavailable")
	page := func(page, size int) ([]map[string]interface{}, int64, error) {
		return nil, 0, wantErr
	}

	if _, err := fetchAllDatasetIDs(page, 100); err == nil {
		t.Fatal("expected the listPage error to propagate")
	}
}

func TestFetchAllDatasetIDsSkipsMalformedRows(t *testing.T) {
	page := func(p, size int) ([]map[string]interface{}, int64, error) {
		return []map[string]interface{}{
			{"id": "dataset-1"},
			{"id": ""},               // empty id skipped
			{"id": 42},               // non-string id skipped
			{"description": "no id"}, // missing id skipped
		}, 4, nil
	}

	ids, err := fetchAllDatasetIDs(page, 100)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 1 || ids[0] != "dataset-1" {
		t.Fatalf("got %v, want [dataset-1]", ids)
	}
}
