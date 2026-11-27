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
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/entity"
)

// A server started without --migrate only converges the tables its own writes
// touch, so anything missing from this list is not "the migration has not run
// yet": it is Error 1146 on the request path. dataset_model_ref is written by
// every dataset create and update, which is how a plain --api start used to
// fail.
func TestAutoMigrateRuntimeModelsCreatesRuntimeWrittenTables(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}

	if err = autoMigrateRuntimeModels(context.Background(), db); err != nil {
		t.Fatalf("autoMigrateRuntimeModels: %v", err)
	}

	for _, model := range []interface{}{
		&entity.DatasetModelRef{},
		&entity.CompilationTemplate{},
		&entity.CompilationTemplateGroup{},
		&entity.CompilationTemplateVersion{},
		&entity.KnowledgeCompileDataset{},
	} {
		if !db.Migrator().HasTable(model) {
			t.Errorf("%T has no table after autoMigrateRuntimeModels", model)
		}
	}
}
