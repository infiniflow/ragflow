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

package dataset

import "testing"

// datasetTestExtraModels adds no tables beyond the community schema.
func datasetTestExtraModels() []interface{} { return nil }

// datasetTestMembershipRole keeps the caller's membership role: the community
// edition has no tenant role table.
func datasetTestMembershipRole(_ *testing.T, _ string, role string) string { return role }

// grantDatasetTestResource grants nothing: the community edition has no
// per-resource permission table to seed.
func grantDatasetTestResource(_ *testing.T, _ string, _ string, _ string, _ string, _ ...string) {
}

// enableDatasetTestModelAccess grants nothing: the community edition has no
// model group access control.
func enableDatasetTestModelAccess(_ *testing.T, _ string, _ string, _ string) {}
