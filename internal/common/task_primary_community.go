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

//go:build !ee

package common

// PrimaryTier is the ingestion lane the shared admin / memory-wake-up paths
// treat as primary. The community build uses the single low lane, which reuses
// the bare community queue name (TaskSubject) so an OSS split needs no rename
// and OSS/EE can share one NATS server. The enterprise build (task_primary_ee.go)
// uses the medium lane.
func PrimaryTier() Tier {
	return TierLow
}

// PrimaryTierSubject is the JetStream subject of PrimaryTier. The community
// build has a single lane whose subject equals the bare community queue name
// (TaskSubject), so the admin pull endpoint and the memory wake-up fallback
// publish to TaskSubject without any edition branching at the call site.
func PrimaryTierSubject() string {
	return TaskSubject
}
