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
//
//  limitations under the License.
//

package service

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/engine"
)

// stubEngine embeds engine.DocEngine so it satisfies the interface at compile
// time (its methods delegate to the embedded nil value); only the methods the
// test observes are overridden. It exists purely to give the wiring test a
// comparable, non-nil engine value to assert propagation with.
type stubEngine struct{ engine.DocEngine }

func (stubEngine) GetType() string { return "stub" }

// TestNewKnowledgeCompileConsumerWiresStorage guards the production assembly:
// the dataset-level compile consumer MUST be built with WithStorage so its
// reader/writer/contribution store are engine-backed. A regression that drops
// WithStorage — or threads the wrong/nil engine or db through it — would
// silently disable every production merge; this catches it by asserting the
// exact deps the ingestor provides actually reach the consumer's reader/writer.
func TestNewKnowledgeCompileConsumerWiresStorage(t *testing.T) {
	ingestor := newUnitIngestor("test-kc-wiring", 1, nil)
	// Provide the deps the production path threads through WithStorage and
	// assert they propagate verbatim to the consumer (not just the flag).
	eng := stubEngine{}
	db, err := gorm.Open(sqlite.Open("file:test-kc-wiring?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	ingestor.deps.Engine = eng
	ingestor.deps.DB = db

	consumer := ingestor.newKnowledgeCompileConsumer()
	if consumer == nil {
		t.Fatal("newKnowledgeCompileConsumer returned nil")
	}
	if !consumer.IsStorageConfigured() {
		t.Fatal("production consumer must be storage-configured (WithStorage applied)")
	}
	if consumer.StorageEngine() != eng {
		t.Fatal("WithStorage did not propagate the ingestor's engine to the consumer reader/writer")
	}
	if consumer.StorageDB() != db {
		t.Fatal("WithStorage did not propagate the ingestor's db to the consumer")
	}
}
