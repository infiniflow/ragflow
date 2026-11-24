package file

import (
	"encoding/json"
	"reflect"
	"testing"

	"ragflow/internal/entity"
	"ragflow/internal/storage"
)

func TestWorkspaceCommitSnapshots(t *testing.T) {
	db := newPageCommitTestDB(t)
	if err := db.AutoMigrate(&entity.File{}); err != nil {
		t.Fatalf("migrate files: %v", err)
	}
	// CreateCommit's deletion path writes the shared schema's status column,
	// which is not declared by the Go File model.
	if err := db.Exec("ALTER TABLE file ADD COLUMN status TEXT DEFAULT '1'").Error; err != nil {
		t.Fatalf("add file status column: %v", err)
	}
	for _, id := range []string{"a", "b", "c"} {
		if err := db.Create(&entity.File{ID: id, ParentID: "workspace", Name: id + ".txt", Type: "file"}).Error; err != nil {
			t.Fatalf("create file: %v", err)
		}
	}
	svc := NewFileCommitService()
	var sequence int64
	commit := func(changes ...entity.FileChange) *entity.FileCommit {
		t.Helper()
		c, err := svc.CreateCommit(t.Context(), "workspace", "author", "version", changes)
		if err != nil {
			t.Fatalf("create commit: %v", err)
		}
		// Latest-commit selection orders by milliseconds. Give each fixture a
		// distinct time so this test exercises snapshots, not timestamp ties.
		sequence++
		if err := db.Model(c).Update("create_time", sequence).Error; err != nil {
			t.Fatalf("set commit time: %v", err)
		}
		return c
	}
	assertDiff := func(from, to *entity.FileCommit, expected []string) []entity.DiffEntry {
		t.Helper()
		diff, err := svc.DiffCommits(t.Context(), from.ID, to.ID)
		if err != nil {
			t.Fatalf("diff: %v", err)
		}
		got := make([]string, 0, len(diff))
		for _, entry := range diff {
			got = append(got, entry.FileID+":"+entry.Operation+":"+entry.FileName)
		}
		if !reflect.DeepEqual(got, expected) {
			t.Fatalf("diff %s -> %s = %v, want %v", from.ID, to.ID, got, expected)
		}
		return diff
	}
	assertContent := func(c *entity.FileCommit, fileID, expected string) {
		t.Helper()
		content, err := svc.GetCommitFileContent(t.Context(), "workspace", c.ID, fileID)
		if err != nil || string(content) != expected {
			t.Fatalf("content %s/%s = %q, %v; want %q", c.ID, fileID, content, err, expected)
		}
	}

	c1 := commit(entity.FileChange{FileID: "a", FileName: "a.txt", Operation: "add", Content: "alpha"})
	c2 := commit(entity.FileChange{FileID: "b", FileName: "b.txt", Operation: "add", Content: "beta"})
	diff := assertDiff(c1, c2, []string{"b:add:b.txt"})
	if diff[0].OldHash != nil || diff[0].NewHash == nil || diff[0].NewLocation == nil {
		t.Fatalf("invalid added-file content references: %+v", diff[0])
	}
	assertContent(c2, "a", "alpha")
	assertDiff(c2, c2, []string{})

	c3 := commit(entity.FileChange{FileID: "b", FileName: "b.txt", Operation: "modify", Content: "beta edited"})
	c4 := commit(entity.FileChange{FileID: "c", FileName: "c.txt", Operation: "add", Content: "gamma"})
	diff = assertDiff(c2, c4, []string{"b:modify:b.txt", "c:add:c.txt"})
	if diff[0].OldHash == nil || diff[0].NewHash == nil || *diff[0].OldHash == *diff[0].NewHash {
		t.Fatalf("modified-file hashes: %+v", diff[0])
	}
	assertContent(c2, "b", "beta")
	assertContent(c3, "b", "beta edited")
	assertContent(c4, "a", "alpha")

	c5 := commit(entity.FileChange{FileID: "a", Operation: "delete"})
	diff = assertDiff(c4, c5, []string{"a:delete:a.txt"})
	if diff[0].NewHash != nil || diff[0].OldHash == nil || diff[0].OldLocation == nil {
		t.Fatalf("invalid deleted-file content references: %+v", diff[0])
	}
	if _, err := svc.GetCommitFileContent(t.Context(), "workspace", c5.ID, "a"); err == nil {
		t.Fatal("deleted file must not expose its previous content")
	}
	assertContent(c4, "a", "alpha")

	c6 := commit(entity.FileChange{FileID: "a", FileName: "a.txt", Operation: "add", Content: "restored"})
	assertDiff(c5, c6, []string{"a:add:a.txt"})
	assertDiff(c4, c6, []string{"a:modify:a.txt"})
	assertContent(c6, "a", "restored")

	c7 := commit(entity.FileChange{FileID: "a", Operation: "rename", OldName: "a.txt", NewName: "renamed.txt"})
	c8 := commit(entity.FileChange{FileID: "a", Operation: "rename", OldName: "renamed.txt", NewName: "final.txt"})
	diff = assertDiff(c7, c8, []string{"a:rename:final.txt"})
	if diff[0].OldHash == nil || diff[0].NewHash == nil || *diff[0].OldHash != *diff[0].NewHash {
		t.Fatalf("rename should preserve content hashes: %+v", diff[0])
	}
	assertDiff(c6, c8, []string{"a:rename:final.txt"})
	assertContent(c7, "a", "restored")
	assertContent(c8, "a", "restored")
	assertContent(c1, "a", "alpha")
	if _, err := svc.GetCommitFileContent(t.Context(), "workspace", c1.ID, "b"); err == nil {
		t.Fatal("file absent from historical snapshot must not be readable")
	}
	if _, err := svc.GetCommitFileContent(t.Context(), "other-workspace", c8.ID, "a"); err == nil {
		t.Fatal("commit must belong to the requested storage workspace")
	}
}

func TestWorkspaceCommitSnapshotValidation(t *testing.T) {
	db := newPageCommitTestDB(t)
	svc := NewFileCommitService()
	empty := "{}"
	valid := &entity.FileCommit{ID: "valid", FolderID: "workspace", TreeState: &empty}
	if err := db.Create(valid).Error; err != nil {
		t.Fatalf("create valid snapshot: %v", err)
	}
	for _, tc := range []struct {
		name string
		tree *string
	}{
		{"missing", nil},
		{"malformed", sptr("{")},
		{"null", sptr("null")},
		{"array", sptr("[]")},
		{"null-entry", sptr(`{"a":null}`)},
		{"scalar-entry", sptr(`{"a":"bad"}`)},
		{"invalid-status", sptr(`{"a":{"status":"2"}}`)},
		{"missing-status", sptr(`{"a":{"name":"a.txt"}}`)},
		{"wrong-hash-type", sptr(`{"a":{"status":"1","hash":42}}`)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			invalid := &entity.FileCommit{ID: tc.name, FolderID: "workspace", TreeState: tc.tree}
			if err := db.Create(invalid).Error; err != nil {
				t.Fatalf("create invalid snapshot: %v", err)
			}
			if _, err := svc.DiffCommits(t.Context(), valid.ID, invalid.ID); err == nil {
				t.Fatal("invalid target snapshot must not produce a successful diff")
			}
			if _, err := svc.DiffCommits(t.Context(), invalid.ID, valid.ID); err == nil {
				t.Fatal("invalid source snapshot must not produce a successful diff")
			}
			if _, err := svc.GetCommitFileContent(t.Context(), "workspace", invalid.ID, "a"); err == nil {
				t.Fatal("invalid snapshot must not produce content")
			}
		})
	}
	if _, err := svc.DiffCommits(t.Context(), "unknown", valid.ID); err == nil {
		t.Fatal("unknown source commit must fail")
	}
	if _, err := svc.DiffCommits(t.Context(), valid.ID, "unknown"); err == nil {
		t.Fatal("unknown target commit must fail")
	}
	if _, err := svc.GetCommitFileContent(t.Context(), "workspace", "unknown", "a"); err == nil {
		t.Fatal("unknown content commit must fail")
	}
	other := &entity.FileCommit{ID: "other", FolderID: "other-workspace", TreeState: &empty}
	if err := db.Create(other).Error; err != nil {
		t.Fatalf("create other workspace: %v", err)
	}
	if _, err := svc.DiffCommits(t.Context(), valid.ID, other.ID); err == nil {
		t.Fatal("cross-workspace comparison must fail")
	}
}

func TestWorkspaceCommitContentUsesSnapshotLocation(t *testing.T) {
	db := newPageCommitTestDB(t)
	tree, err := json.Marshal(map[string]entity.TreeNode{
		"a":          {Name: "a.txt", Status: "1", Hash: "content-hash", Location: "captured/object"},
		"no-content": {Name: "empty.txt", Status: "1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	commit := &entity.FileCommit{ID: "snapshot", FolderID: "workspace", TreeState: sptr(string(tree))}
	if err := db.Create(commit).Error; err != nil {
		t.Fatalf("create snapshot: %v", err)
	}
	memory := storage.GetStorageFactory().GetStorage()
	if err := memory.Put(t.Context(), "workspace", "captured/object", []byte("snapshot content")); err != nil {
		t.Fatal(err)
	}
	if err := memory.Put(t.Context(), "workspace", ".objects/content-hash", []byte("wrong object")); err != nil {
		t.Fatal(err)
	}
	svc := NewFileCommitService()
	content, err := svc.GetCommitFileContent(t.Context(), "workspace", commit.ID, "a")
	if err != nil || string(content) != "snapshot content" {
		t.Fatalf("snapshot location content = %q, %v", content, err)
	}
	if _, err := svc.GetCommitFileContent(t.Context(), "workspace", commit.ID, "no-content"); err == nil {
		t.Fatal("entry without captured content must fail")
	}
	if err := memory.Remove(t.Context(), "workspace", "captured/object"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetCommitFileContent(t.Context(), "workspace", commit.ID, "a"); err == nil {
		t.Fatal("missing storage object must fail")
	}
	storage.GetStorageFactory().SetStorage(nil)
	if _, err := svc.GetCommitFileContent(t.Context(), "workspace", commit.ID, "a"); err == nil {
		t.Fatal("uninitialized storage must fail")
	}
}
