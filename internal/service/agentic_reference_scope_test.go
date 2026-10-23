package service

import (
	"context"
	"errors"
	"slices"
	"testing"

	"ragflow/internal/permission"
)

// The knowledge binding is configuration, not proof of access: an id the caller
// may not read must not scope the citation lookup, or the payload would carry
// chunks from a dataset this user cannot open. The check runs through the same
// predicate the retrieval paths use, with the calling user as the subject.
func TestFilterAuthorizedDatasetIDsDropsUnauthorizedScopes(t *testing.T) {
	var checked []string
	check := func(_ context.Context, subject permission.Subject, datasetID string, operation permission.Operation) error {
		if subject.UserID != "user-1" {
			t.Fatalf("subject = %#v, want the calling user", subject)
		}
		if operation != permission.OperationUse {
			t.Fatalf("operation = %v, want use", operation)
		}
		checked = append(checked, datasetID)
		if datasetID == "kb-denied" {
			return errors.New("only the owner of dataset kb-denied is authorized for this operation")
		}
		return nil
	}

	got := filterAuthorizedDatasetIDs(context.Background(), check, "user-1",
		[]string{"kb-ok", "kb-denied", " kb-ok ", "", "kb-other"})
	if want := []string{"kb-ok", "kb-other"}; !slices.Equal(got, want) {
		t.Fatalf("scope = %v, want %v", got, want)
	}
	// Blank and repeated entries are not re-checked: the scope is a set, and a
	// second query for the same id buys nothing.
	if want := []string{"kb-ok", "kb-denied", "kb-other"}; !slices.Equal(checked, want) {
		t.Fatalf("checked = %v, want %v", checked, want)
	}
}

// Every id denied leaves the scope empty rather than falling back to an
// unfiltered lookup: with no scope fetchChunksByIDs resolves nothing, so the
// citations stay unresolved instead of resolving outside the caller's reach.
func TestFilterAuthorizedDatasetIDsLeavesAnEmptyScopeWhenAllDenied(t *testing.T) {
	deny := func(context.Context, permission.Subject, string, permission.Operation) error {
		return errors.New("denied")
	}
	got := filterAuthorizedDatasetIDs(context.Background(), deny, "user-1", []string{"kb-a", "kb-b"})
	if len(got) != 0 {
		t.Fatalf("scope = %v, want nothing authorized", got)
	}
}
