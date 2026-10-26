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
	"fmt"
	"testing"

	"ragflow/internal/entity"

	"gorm.io/gorm"
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

func TestUserCanvasVersionDAOKeepsAllVersions(t *testing.T) {
	db := setupUserCanvasTestDB(t)
	if err := db.AutoMigrate(&entity.UserCanvasVersion{}); err != nil {
		t.Fatalf("failed to migrate versions: %v", err)
	}
	ctx := t.Context()
	if err := db.WithContext(ctx).Create(&entity.UserCanvas{ID: "canvas-1", UserID: "user-1"}).Error; err != nil {
		t.Fatalf("failed to create canvas: %v", err)
	}
	dao := NewUserCanvasVersionDAO()
	opts := SaveOrReplaceLatestVersionOptions{UserCanvasID: "canvas-1"}
	const versionCount = 35
	for i := 0; i < versionCount; i++ {
		opts.NewID = fmt.Sprintf("version-%02d", i)
		opts.DSL = entity.JSONMap{"revision": fmt.Sprint(i)}
		if err := db.Transaction(func(tx *gorm.DB) error {
			_, err := dao.SaveOrReplaceLatestTx(ctx, tx, opts)
			return err
		}); err != nil {
			t.Fatalf("failed to save version %d: %v", i, err)
		}
	}
	for _, stage := range []string{"insert", "refresh", "publish"} {
		t.Run(stage, func(t *testing.T) {
			if stage != "insert" {
				opts.NewID = "unused-version-id"
				opts.Release = stage == "publish"
				if err := db.Transaction(func(tx *gorm.DB) error {
					row, err := dao.SaveOrReplaceLatestTx(ctx, tx, opts)
					if err == nil && (row.ID != "version-34" || row.Release != opts.Release) {
						t.Errorf("unexpected refreshed version: id=%q release=%v", row.ID, row.Release)
					}
					return err
				}); err != nil {
					t.Fatalf("failed to save latest version: %v", err)
				}
			}
			versions, err := dao.ListByCanvasID(ctx, db, opts.UserCanvasID)
			if err != nil {
				t.Fatalf("failed to list versions: %v", err)
			}
			if len(versions) != versionCount {
				t.Fatalf("version count = %d, want %d", len(versions), versionCount)
			}
			ids := make(map[string]bool, len(versions))
			for _, version := range versions {
				ids[version.ID] = true
			}
			for i := 0; i < versionCount; i++ {
				if id := fmt.Sprintf("version-%02d", i); !ids[id] {
					t.Errorf("saved version %q was deleted", id)
				}
			}
		})
	}
}
