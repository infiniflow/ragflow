package file

import (
	"context"
	"testing"

	"ragflow/internal/entity"

	"gorm.io/gorm"
)

// newCommitAuthTestDB reuses the page-commit harness and adds the file table.
func newCommitAuthTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := newPageCommitTestDB(t)
	if err := db.AutoMigrate(&entity.File{}); err != nil {
		t.Fatalf("auto migrate file: %v", err)
	}
	return db
}

type commitAuthFixture struct {
	db      *gorm.DB
	fileSub *entity.File
}

func newCommitAuthFixture(t *testing.T) *commitAuthFixture {
	t.Helper()
	f := &commitAuthFixture{db: newCommitAuthTestDB(t)}
	mk := func(id, parentID, tenantID, name, fileType, location string, size int64) *entity.File {
		row := &entity.File{
			ID:        id,
			ParentID:  parentID,
			TenantID:  tenantID,
			CreatedBy: tenantID,
			Name:      name,
			Type:      fileType,
			Size:      size,
		}
		if location != "" {
			row.Location = &location
		}
		if err := f.db.Create(row).Error; err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
		return row
	}
	// tenant-a owns two sibling workspaces; tenant-b owns a third one.
	mk("folder-a", "folder-a", "tenant-a", "workspace-a", "folder", "", 0)
	mk("file-a", "folder-a", "tenant-a", "a.txt", "txt", "live/a.txt", 3)
	mk("sub-a", "folder-a", "tenant-a", "sub", "folder", "", 0)
	f.fileSub = mk("file-sub", "sub-a", "tenant-a", "nested.txt", "txt", "live/nested.txt", 3)
	mk("folder-b", "folder-b", "tenant-a", "workspace-b", "folder", "", 0)
	mk("file-b", "folder-b", "tenant-a", "b.txt", "txt", "live/b.txt", 3)
	mk("folder-c", "folder-c", "tenant-b", "workspace-c", "folder", "", 0)
	mk("file-c", "folder-c", "tenant-b", "c.txt", "txt", "live/c.txt", 3)
	return f
}

func (f *commitAuthFixture) assertUntouched(t *testing.T, id, wantName, wantLocation string) {
	t.Helper()
	var row entity.File
	if err := f.db.Where("id = ?", id).First(&row).Error; err != nil {
		t.Fatalf("reload %s: %v", id, err)
	}
	if row.Name != wantName {
		t.Fatalf("file %s name = %q, want %q", id, row.Name, wantName)
	}
	if row.Location == nil || *row.Location != wantLocation {
		t.Fatalf("file %s location = %v, want %q", id, row.Location, wantLocation)
	}
	if row.Size != 3 {
		t.Fatalf("file %s size = %d, want 3", id, row.Size)
	}
	var commits int64
	if err := f.db.Model(&entity.FileCommit{}).Count(&commits).Error; err != nil {
		t.Fatalf("count commits: %v", err)
	}
	if commits != 0 {
		t.Fatalf("rejected commit was persisted: %d rows", commits)
	}
}

// A commit only proves the caller may use the target folder; every
// files[].file_id is independent input, so the service must reject IDs that
// live outside that folder tree instead of writing those rows (IDOR/CWE-639).
func TestCreateCommitRejectsFilesOutsideFolder(t *testing.T) {
	f := newCommitAuthFixture(t)
	svc := NewFileCommitService()
	ctx := context.Background()

	cases := []struct {
		name         string
		change       entity.FileChange
		victim       string
		wantName     string
		wantLocation string
	}{
		{
			name:         "modify file of another tenant",
			change:       entity.FileChange{FileID: "file-c", Operation: "modify", FileName: "c.txt", Content: "pwned"},
			victim:       "file-c",
			wantName:     "c.txt",
			wantLocation: "live/c.txt",
		},
		{
			name:         "modify file of a sibling workspace",
			change:       entity.FileChange{FileID: "file-b", Operation: "modify", FileName: "b.txt", Content: "pwned"},
			victim:       "file-b",
			wantName:     "b.txt",
			wantLocation: "live/b.txt",
		},
		{
			name:         "rename file of a sibling workspace",
			change:       entity.FileChange{FileID: "file-b", Operation: "rename", OldName: "b.txt", NewName: "pwned.txt"},
			victim:       "file-b",
			wantName:     "b.txt",
			wantLocation: "live/b.txt",
		},
		{
			name:         "add an unknown file id",
			change:       entity.FileChange{FileID: "no-such-file", Operation: "add", FileName: "ghost.txt", Content: "pwned"},
			victim:       "file-a",
			wantName:     "a.txt",
			wantLocation: "live/a.txt",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := svc.CreateCommit(ctx, "folder-a", "user-a", "attempt", []entity.FileChange{tc.change}); err == nil {
				t.Fatalf("expected CreateCommit to reject %s", tc.change.FileID)
			}
			f.assertUntouched(t, tc.victim, tc.wantName, tc.wantLocation)
		})
	}
}

func TestCreateCommitAcceptsFilesInsideFolderTree(t *testing.T) {
	f := newCommitAuthFixture(t)
	svc := NewFileCommitService()
	ctx := context.Background()

	commit, err := svc.CreateCommit(ctx, "folder-a", "user-a", "edit", []entity.FileChange{
		{FileID: "file-a", Operation: "modify", FileName: "a.txt", Content: "hello"},
		{FileID: "file-sub", Operation: "modify", FileName: "nested.txt", Content: "nested hello"},
	})
	if err != nil {
		t.Fatalf("CreateCommit: %v", err)
	}
	if commit.FileCount != 2 {
		t.Fatalf("expected 2 changed files, got %d", commit.FileCount)
	}
	for id, content := range map[string]string{"file-a": "hello", "file-sub": "nested hello"} {
		var row entity.File
		if err := f.db.Where("id = ?", id).First(&row).Error; err != nil {
			t.Fatalf("reload %s: %v", id, err)
		}
		if row.Size != int64(len(content)) {
			t.Fatalf("file %s size = %d, want %d", id, row.Size, len(content))
		}
		if row.Location == nil || *row.Location == "" {
			t.Fatalf("file %s location was not updated", id)
		}
	}
}

func TestCreateCommitRejectsMissingFolder(t *testing.T) {
	f := newCommitAuthFixture(t)
	svc := NewFileCommitService()

	if _, err := svc.CreateCommit(context.Background(), "no-such-folder", "user-a", "edit", []entity.FileChange{
		{FileID: "file-a", Operation: "modify", FileName: "a.txt", Content: "hello"},
	}); err == nil {
		t.Fatal("expected CreateCommit to reject an unknown folder")
	}
	var commits int64
	if err := f.db.Model(&entity.FileCommit{}).Count(&commits).Error; err != nil {
		t.Fatalf("count commits: %v", err)
	}
	if commits != 0 {
		t.Fatalf("commit was persisted for an unknown folder: %d rows", commits)
	}
}
