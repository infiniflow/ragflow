package ingestion

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestMigrationGuardRemoved keeps the Go-only comment cleanup permanent: the
// build-time migration-anchor guard and its per-package boilerplate tests were
// migration-time scaffolding that outlived their purpose, and must not return.
func TestMigrationGuardRemoved(t *testing.T) {
	// A directory-level check catches the guard package being reintroduced.
	guardDir := filepath.Join(".", "internal", "migrationguard")
	if _, err := os.Stat(guardDir); !os.IsNotExist(err) {
		t.Errorf("migrationguard package still present at %s (stat err=%v)", guardDir, err)
	}

	// Rebuild the import path in pieces so this test does not match its own source.
	guardImport := "ragflow/internal/ingestion/internal/" + "migrationguard"

	var leftovers []string
	err := filepath.WalkDir(".", func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		if d.Name() == "migration_comments_test.go" {
			leftovers = append(leftovers, path+" (boilerplate test)")
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		if strings.Contains(string(data), guardImport) {
			leftovers = append(leftovers, path+" (imports guard)")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk ingestion tree: %v", err)
	}
	if len(leftovers) > 0 {
		t.Errorf("migration guard artifacts remain:\n%s", strings.Join(leftovers, "\n"))
	}
}
