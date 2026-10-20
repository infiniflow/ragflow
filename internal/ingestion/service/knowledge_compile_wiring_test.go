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
)

// TestNewKnowledgeCompileConsumerWiresStorage guards the production assembly:
// the dataset-level compile consumer MUST be built with WithStorage so its
// reader/writer/contribution store are engine-backed. A regression that drops
// WithStorage would silently disable every production merge; this catches it.
func TestNewKnowledgeCompileConsumerWiresStorage(t *testing.T) {
	ingestor := newUnitIngestor("test-kc-wiring", 1, nil)
	// Provide the deps the production path threads through WithStorage.
	ingestor.deps.Engine = nil // value irrelevant: the flag is what we assert
	ingestor.deps.DB = nil

	consumer := ingestor.newKnowledgeCompileConsumer()
	if consumer == nil {
		t.Fatal("newKnowledgeCompileConsumer returned nil")
	}
	if !consumer.IsStorageConfigured() {
		t.Fatal("production consumer must be storage-configured (WithStorage applied)")
	}
}
