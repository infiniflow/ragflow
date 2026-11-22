package handler

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/permission"
)

// fakeDatasetChecker records every resource it is asked to check so the test
// can prove that all omitted datasets are examined, not just the first one.
type fakeDatasetChecker struct {
	calls   []string
	errByID map[string]error
}

func (f *fakeDatasetChecker) FilterResources(ctx context.Context, refs []permission.ResourceRef, op permission.Operation) ([]permission.ResourceRef, error) {
	return nil, nil
}
func (f *fakeDatasetChecker) CheckResource(ctx context.Context, ref permission.ResourceRef, op permission.Operation) error {
	f.calls = append(f.calls, ref.ID)
	return f.errByID[ref.ID]
}

// TestCheckOmittedDatasetsChecksAllOmitted is the RED test for #4: if an
// omitted dataset becomes readable at check time, the helper must still go on
// and check the remaining omitted datasets instead of returning success.
func TestCheckOmittedDatasetsChecksAllOmitted(t *testing.T) {
	refs := []permission.ResourceRef{
		{Kind: permission.ResourceKindDataset, ID: "ds-a"},
		{Kind: permission.ResourceKindDataset, ID: "ds-b"},
		{Kind: permission.ResourceKindDataset, ID: "ds-allowed"},
	}
	allowed := map[string]bool{"ds-allowed": true}
	checker := &fakeDatasetChecker{errByID: map[string]error{
		"ds-b": permission.ErrPermissionDenied,
	}}

	err := checkOmittedDatasets(context.Background(), checker, refs, allowed)
	if !errors.Is(err, permission.ErrPermissionDenied) {
		t.Fatalf("err = %v, want ErrPermissionDenied from the second omitted dataset", err)
	}
	want := []string{"ds-a", "ds-b"}
	if fmt.Sprint(checker.calls) != fmt.Sprint(want) {
		t.Fatalf("checked = %v, want %v (all omitted datasets must be examined)", checker.calls, want)
	}
}

// TestCheckDatasetsReadableBatchFailureKeepsFirstError is the RED test for #3:
// when the shared batch load fails, the helper must fall back to ordered
// single-resource checks so the first request-order error wins, rather than
// letting a later dataset's invalid-permission error reach the response.
func TestCheckDatasetsReadableBatchFailureKeepsFirstError(t *testing.T) {
	db := setupHandlerAccessDB(t)
	orig := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = orig })

	// tenant-2 has no active owner, so a batch load containing its dataset
	// fails with ErrInvalidPermission.
	db.Create(&entity.Tenant{ID: "tenant-2"})
	db.Create(&entity.Knowledgebase{
		ID: "ds-bad-tenant", TenantID: "tenant-2", Name: "bad", EmbdID: "embd-1",
		CreatedBy: "owner-2", Permission: string(entity.TenantPermissionTeam),
		Status: sptr(string(entity.StatusValid)),
	})
	ctx := permission.WithIdentity(context.Background(), "user-1", "tenant-1")
	// ds-missing is first in request order and does not exist.
	err := checkDatasetsReadable(ctx, db, []string{"ds-missing", "ds-bad-tenant"})
	if !errors.Is(err, permission.ErrResourceNotFound) {
		t.Fatalf("err = %v, want ErrResourceNotFound from the first (missing) dataset", err)
	}
}

// TestCheckDatasetRefsReadableChecksAllOmitted is adversarial at the full
// production path: the batch omits two datasets, and the first omitted one
// becomes readable at single-check time. The helper must still examine the
// second omitted dataset and return its denial. This guards the wiring of
// checkOmittedDatasets into the real check flow, not just the helper in isolation.
func TestCheckDatasetRefsReadableChecksAllOmitted(t *testing.T) {
	refs := []permission.ResourceRef{
		{Kind: permission.ResourceKindDataset, ID: "ds-a"},
		{Kind: permission.ResourceKindDataset, ID: "ds-b"},
		{Kind: permission.ResourceKindDataset, ID: "ds-c"},
	}
	checker := &fullFlowFakeChecker{
		// Batch answers only ds-c; ds-a and ds-b are omitted.
		accessible: []permission.ResourceRef{refs[2]},
		errByID: map[string]error{
			"ds-a": nil, // became readable at single-check time
			"ds-b": permission.ErrPermissionDenied,
		},
	}
	err := checkDatasetRefsReadable(context.Background(), checker, refs)
	if !errors.Is(err, permission.ErrPermissionDenied) {
		t.Fatalf("err = %v, want ErrPermissionDenied from the later omitted dataset", err)
	}
}

type fullFlowFakeChecker struct {
	accessible []permission.ResourceRef
	errByID    map[string]error
	calls      []string
}

func (f *fullFlowFakeChecker) FilterResources(ctx context.Context, resources []permission.ResourceRef, op permission.Operation) ([]permission.ResourceRef, error) {
	return f.accessible, nil
}
func (f *fullFlowFakeChecker) CheckResource(ctx context.Context, ref permission.ResourceRef, op permission.Operation) error {
	f.calls = append(f.calls, ref.ID)
	return f.errByID[ref.ID]
}
