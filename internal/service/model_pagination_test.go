package service

import (
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
)

func TestListAllModelsPagination(t *testing.T) {
	catalog, err := dao.GetModelProviderManager().ListAllModels()
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog) <= common.MaxPageSize {
		t.Fatalf("catalog has %d models, need more than a page", len(catalog))
	}
	svc := &ModelProviderService{}
	for _, tt := range []struct {
		name             string
		page, size, want int
	}{
		{"first", 1, 100, 100},
		{"last", (len(catalog) + 99) / 100, 100, (len(catalog)-1)%100 + 1},
		{"past end", len(catalog) + 1, 100, 0},
		{"default size", 1, common.DefaultPageSize, common.DefaultPageSize},
	} {
		t.Run(tt.name, func(t *testing.T) {
			models, err := svc.ListAllModels(tt.page, tt.size)
			if err != nil || len(models) != tt.want {
				t.Fatalf("count/error = %d/%v, want %d/nil", len(models), err, tt.want)
			}
		})
	}
}
