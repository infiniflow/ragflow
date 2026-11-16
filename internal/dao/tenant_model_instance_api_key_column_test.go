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
	"strings"
	"sync"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"

	"ragflow/internal/entity"
)

// TestTenantModelInstanceAPIKeyColumnIsUnsizedText pins the MySQL migration
// type of tenant_model_instance.api_key. The column used to be declared
// varchar(512), so saving a provider key longer than 512 characters failed at
// the database layer with MySQL Error 1406 (#22422), while tenant_llm.api_key
// is already TEXT. AutoMigrate derives the column type from the entity tag, so
// asserting the dialector's full data type guards both the cap removal and the
// widening of existing installations.
func TestTenantModelInstanceAPIKeyColumnIsUnsizedText(t *testing.T) {
	shared, _, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { shared.Close() })

	db, err := gorm.Open(mysql.New(mysql.Config{
		DSN:                       "test:test@tcp(localhost:3306)/test?parseTime=True",
		Conn:                      shared,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}

	s, err := schema.Parse(&entity.TenantModelInstance{}, &sync.Map{}, db.NamingStrategy)
	if err != nil {
		t.Fatalf("failed to parse TenantModelInstance schema: %v", err)
	}
	field, ok := s.FieldsByDBName["api_key"]
	if !ok {
		t.Fatal("api_key column missing from TenantModelInstance schema")
	}

	if field.Size != 0 {
		t.Fatalf("api_key must not carry a length cap, schema size = %d", field.Size)
	}
	fullType := db.Migrator().FullDataTypeOf(field).SQL
	if !strings.HasPrefix(fullType, "text") {
		t.Fatalf("mysql full data type for api_key = %q, want the unsized text form", fullType)
	}
	if strings.Contains(fullType, "varchar") || strings.Contains(fullType, "512") {
		t.Fatalf("mysql full data type for api_key = %q, must not stay a sized varchar", fullType)
	}
}
