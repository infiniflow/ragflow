//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package service

import (
	"ragflow/internal/dao"
	"ragflow/internal/engine"
	kvrocks "ragflow/internal/engine/kvrocks"
	"ragflow/internal/ingestion"
)

// newUnitIngestor builds a unit-test Ingestor. The dependency bundle reads the
// process-wide handles the surrounding test swaps in (dao.DB via
// testutil.ReplaceDBForTest, the message queue via engine.SetMessageQueueEngine),
// so real values flow into the ingestor while nil values exercise the
// nil-dependency guards. Engine stays nil: executor tests stub the index writer
// or assert the downstream no-engine failure.
func newUnitIngestor(name string, maxConcurrency int32, supportedTypes []string) *Ingestor {
	ingestor := NewIngestor(name, maxConcurrency, supportedTypes, &ingestion.Deps{
		DB: dao.DB,
		MQ: engine.GetMessageQueueEngine(),
		KV: kvrocks.Get(),
	})
	// Unit fixtures do not initialize the process-wide Kvrocks client. Use the
	// in-memory claim store so claim acquisition exercises the same fail-closed
	// contract as production without depending on an external service.
	if ingestor.taskClaims == nil {
		ingestor.taskClaims = newFakeTaskClaimStore()
	}
	return ingestor
}
