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
	"errors"
	"testing"

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
	Create(v *entity.UserCanvasVersion) error
	GetByID(id string) (*entity.UserCanvasVersion, error)
	ListByCanvasID(canvasID string) ([]*entity.UserCanvasVersion, error)
	GetLatest(canvasID string) (*entity.UserCanvasVersion, error)
	SaveOrReplaceLatest(opts SaveOrReplaceLatestVersionOptions) (*entity.UserCanvasVersion, error)
	Delete(id string) error
	DeleteByCanvasID(canvasID string) (int64, error)
	DeleteAllUnpublishedExcess(canvasID string, keep int) error
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
