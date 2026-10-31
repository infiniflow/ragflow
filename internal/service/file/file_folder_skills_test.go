package file

import (
	"testing"
)

func folderNames(files []map[string]interface{}) []string {
	names := make([]string, 0, len(files))
	for _, f := range files {
		name, _ := f["name"].(string)
		names = append(names, name)
	}
	return names
}

func hasFolderNamed(files []map[string]interface{}, name string) bool {
	for _, f := range files {
		if f["name"] == name {
			return true
		}
	}
	return false
}

func countFoldersNamed(files []map[string]interface{}, name string) int {
	count := 0
	for _, f := range files {
		if f["name"] == name {
			count++
		}
	}
	return count
}

// The Skills page resolves its top-level folder from the root listing
// (web/src/pages/skills/hooks.ts, ensureSkillsFolder) and the Files page
// renders that folder as the entry into it. Hiding it from the same listing
// leaves the entry unreachable and makes the client create a duplicate
// "skills (1)" folder on every visit.
func TestListFiles_RootListingIncludesSkillsFolder(t *testing.T) {
	setupFolderTestDB(t)
	svc := testFileService()
	ctx := fileTestContext(t, "tenant-1", "tenant-1")

	// Two listings: the initializer must not fork a second row, and both
	// responses must expose the folder the clients resolve by name.
	for i := 0; i < 2; i++ {
		resp, err := svc.ListFilesForTenant(ctx, "tenant-1", "tenant-1", "", 1, 15, nil, "")
		if err != nil {
			t.Fatalf("ListFilesForTenant failed: %v", err)
		}
		if !hasFolderNamed(resp.Files, SkillsFolderName) {
			t.Fatalf("root listing %d omitted %q; got %v", i, SkillsFolderName, folderNames(resp.Files))
		}
		if got := countFoldersNamed(resp.Files, SkillsFolderName); got != 1 {
			t.Fatalf("root listing %d returned %d %q folders; got %v", i, got, SkillsFolderName, folderNames(resp.Files))
		}
	}
}
