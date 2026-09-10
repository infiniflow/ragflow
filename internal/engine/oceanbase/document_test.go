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

package oceanbase

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestDocumentWritesValidateIdentifiers(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	engine := newEngineWithDB("oceanbase", "legacy_doc", db)

	if err := engine.IndexDocument(t.Context(), "skill_bad`name", "skill-1", map[string]interface{}{}); err == nil {
		t.Fatal("IndexDocument() error = nil, want invalid identifier error")
	}
	if _, err := engine.BulkIndex(t.Context(), "skill_bad`name", nil); err == nil {
		t.Fatal("BulkIndex() error = nil, want invalid identifier error")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestBulkIndexRejectsEmptyIdentifiers(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	engine := newEngineWithDB("oceanbase", "legacy_doc", db)

	if _, err := engine.BulkIndex(t.Context(), "skill_tenant", []interface{}{map[string]interface{}{"name": "missing ID"}}); err == nil {
		t.Fatal("BulkIndex() error = nil, want empty identifier error")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
