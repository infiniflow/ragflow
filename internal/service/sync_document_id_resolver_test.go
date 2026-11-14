package service

import (
	"context"
	"testing"
)

type resolverFingerprintStore struct {
	DocumentStore
	fingerprints map[string]string
}

func (s resolverFingerprintStore) GetFingerprintsByIDs(context.Context, string, string, []string) (map[string]string, error) {
	return s.fingerprints, nil
}

func TestDocumentIDResolverUsesChosenDocumentsFingerprint(t *testing.T) {
	legacyID := Hash128("connector:source")
	newID := Hash128("kb:connector:source")
	for _, tc := range []struct {
		name         string
		fingerprints map[string]string
		wantID       string
		wantHash     string
	}{
		{"pending legacy", map[string]string{legacyID: "", newID: "completed-new-hash"}, legacyID, ""},
		{"completed legacy", map[string]string{legacyID: "legacy-hash", newID: "new-hash"}, legacyID, "legacy-hash"},
		{"new document", map[string]string{newID: "new-hash"}, newID, "new-hash"},
		{"missing document", map[string]string{}, newID, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resolver := NewDocumentIDResolver(resolverFingerprintStore{fingerprints: tc.fingerprints})
			got, err := resolver.Resolve(t.Context(), "kb", "connector", "github_connector", "source")
			if err != nil || got.DocID != tc.wantID || got.StoredFingerprint != tc.wantHash {
				t.Fatalf("resolved = %+v, err = %v; want id = %s hash = %q", got, err, tc.wantID, tc.wantHash)
			}
		})
	}
}
