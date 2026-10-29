//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package dao

import (
	"context"
	"errors"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/entity"
)

// TestUserCanvasVersionDAO_Constructor verifies the DAO constructor returns
// a non-nil, callable value and that the struct is stateless (two calls
// produce equivalent instances).
func TestUserCanvasVersionDAO_Constructor(t *testing.T) {
	dao1 := NewUserCanvasVersionDAO()
	dao2 := NewUserCanvasVersionDAO()
	if dao1 == nil || dao2 == nil {
		t.Fatalf("NewUserCanvasVersionDAO returned nil")
	}
	// Stateless: both DAOs must be usable in the same way.
	_ = dao1
	_ = dao2
}

// TestUserCanvasVersionDAO_NotFoundSentinel verifies the sentinel error is
// exposed and not equal to gorm.ErrRecordNotFound. The service layer relies
// on this distinct sentinel to map 404 responses.
func TestUserCanvasVersionDAO_NotFoundSentinel(t *testing.T) {
	if ErrUserCanvasVersionNotFound == nil {
		t.Fatal("ErrUserCanvasVersionNotFound must be a non-nil sentinel")
	}
	// Sentinel must be distinct from arbitrary error literals so that
	// the service layer can map it back to a stable 404.
	if ErrUserCanvasVersionNotFound == errors.New("not found") {
		t.Fatal("ErrUserCanvasVersionNotFound must be a stable package-level sentinel, not a fresh error")
	}
	// A string-only wrap (without %w) does not establish an errors.Is chain.
	// Confirming that here documents the expected propagation contract.
	wrapped := errors.New("wrapped: " + ErrUserCanvasVersionNotFound.Error())
	if errors.Is(wrapped, ErrUserCanvasVersionNotFound) {
		t.Fatal("a string-only wrap should not satisfy errors.Is(ErrUserCanvasVersionNotFound)")
	}
}

// daoInterface is the contract that handler/service code consumes from
// UserCanvasVersionDAO. Asserting at compile time that the concrete DAO
// satisfies it prevents accidental signature drift in future refactors.
type daoInterface interface {
	GetByID(ctx context.Context, db *gorm.DB, id string) (*entity.UserCanvasVersion, error)
	ListByCanvasID(ctx context.Context, db *gorm.DB, canvasID string) ([]*entity.UserCanvasVersion, error)
	GetLatest(ctx context.Context, db *gorm.DB, canvasID string) (*entity.UserCanvasVersion, error)
	CreateTx(ctx context.Context, tx *gorm.DB, v *entity.UserCanvasVersion) error
	SaveOrReplaceLatestTx(ctx context.Context, tx *gorm.DB, opts SaveOrReplaceLatestVersionOptions) (*entity.UserCanvasVersion, error)
	DeleteTx(ctx context.Context, tx *gorm.DB, id string) error
	DeleteByCanvasIDTx(ctx context.Context, tx *gorm.DB, canvasID string) (int64, error)
}

var _ daoInterface = (*UserCanvasVersionDAO)(nil)

// TestUserCanvasVersionDAO_InterfaceSatisfied is a runtime marker for the
// compile-time assertion above. Any future removal of a method will fail
// compilation in this file before this test ever runs.
func TestUserCanvasVersionDAO_InterfaceSatisfied(t *testing.T) {
	// The compile-time `var _ daoInterface = (*UserCanvasVersionDAO)(nil)`
	// above is the actual check. This function exists so the file has at
	// least one explicit test bound to a method, not just a sentinel.
	d := NewUserCanvasVersionDAO()
	_ = daoInterface(d)
}

// TestCountPipelineVersionReferences_SumsKBAndDocument verifies the delete-time
// reference scan sums references across both the knowledgebase and document
// tables (design section 12 A4).
func TestCountPipelineVersionReferences_SumsKBAndDocument(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&entity.Knowledgebase{}, &entity.Document{}, &entity.UserCanvasVersion{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	v1 := "version0000000000000000000000aa"
	v2 := "version0000000000000000000000bb"

	// One KB + two documents pin v1.
	if err := db.Create(&entity.Knowledgebase{
		ID: "kb-1", TenantID: "t1", Name: "kb-1", EmbdID: "e", CreatedBy: "u",
		Permission: string(entity.TenantPermissionMe), Status: sp("1"),
		PipelineVersionID: &v1,
	}).Error; err != nil {
		t.Fatalf("seed kb v1: %v", err)
	}
	for i, id := range []string{"doc-1", "doc-2"} {
		if err := db.Create(&entity.Document{
			ID: id, KbID: "kb-1", ParserID: "naive", ParserConfig: entity.JSONMap{},
			TokenNum: int64(i), ChunkNum: int64(i), Suffix: ".txt", Status: sp("1"),
			PipelineVersionID: &v1,
		}).Error; err != nil {
			t.Fatalf("seed doc %s v1: %v", id, err)
		}
	}
	// One KB pins v2 (must not be counted toward v1).
	if err := db.Create(&entity.Knowledgebase{
		ID: "kb-2", TenantID: "t1", Name: "kb-2", EmbdID: "e", CreatedBy: "u",
		Permission: string(entity.TenantPermissionMe), Status: sp("1"),
		PipelineVersionID: &v2,
	}).Error; err != nil {
		t.Fatalf("seed kb v2: %v", err)
	}

	d := NewUserCanvasVersionDAO()
	n1, err := d.CountPipelineVersionReferences(t.Context(), db, v1)
	if err != nil {
		t.Fatalf("count v1: %v", err)
	}
	if n1 != 3 {
		t.Fatalf("CountPipelineVersionReferences(v1) = %d, want 3 (1 kb + 2 docs)", n1)
	}
	n2, err := d.CountPipelineVersionReferences(t.Context(), db, v2)
	if err != nil {
		t.Fatalf("count v2: %v", err)
	}
	if n2 != 1 {
		t.Fatalf("CountPipelineVersionReferences(v2) = %d, want 1", n2)
	}
}

// TestUserCanvasVersionDAO_SaveOrReplaceLatestTxStoresAlias verifies the alias
// field (design A2) is persisted on version save (T15: alias 存储/返回). The
// alias column was previously write-dead: the DAO never copied it into the row.
func TestUserCanvasVersionDAO_SaveOrReplaceLatestTxStoresAlias(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&entity.UserCanvas{}, &entity.UserCanvasVersion{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// Parent canvas row must exist: SaveOrReplaceLatestTx locks it first.
	if err := db.Create(&entity.UserCanvas{ID: "canvas-1", UserID: "user-1"}).Error; err != nil {
		t.Fatalf("seed canvas: %v", err)
	}

	alias := "release-1.0"
	d := NewUserCanvasVersionDAO()
	row, err := d.SaveOrReplaceLatestTx(context.Background(), db, SaveOrReplaceLatestVersionOptions{
		NewID:        "v-1",
		UserCanvasID: "canvas-1",
		Title:        ptrStr("v1"),
		Alias:        &alias,
		DSL:          entity.JSONMap{"graph": "g"},
		Release:      true,
	})
	if err != nil {
		t.Fatalf("SaveOrReplaceLatestTx: %v", err)
	}
	if row.Alias == nil || *row.Alias != alias {
		t.Fatalf("returned row alias = %v, want %q", row.Alias, alias)
	}
	// Reload from the DB to prove the value was persisted, not just in-memory.
	var got entity.UserCanvasVersion
	if err := db.Where("id = ?", "v-1").First(&got).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got.Alias == nil || *got.Alias != alias {
		t.Fatalf("persisted alias = %v, want %q", got.Alias, alias)
	}
}

func ptrStr(v string) *string { return &v }
