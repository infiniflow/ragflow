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

package nats

import "ragflow/internal/common"

// tierOrder for the open-source / community build is a single lane: the bare
// community subject "tasks.RAGFLOW". The enterprise build (see
// tier_policy_ee.go) defines its own lane set. Keeping the lane set in a
// build-tagged file means the OSS split receives exactly one queue and no
// enterprise tier names, while the shared nats.go methods stay identical across
// both editions.
var tierOrder = []common.Tier{common.TierDefault}

// tierStreams for the community build is exactly the legacy single queue:
// RAGFLOW_TASKS / tasks.RAGFLOW / RAGFLOW_CONSUMER. Additional lanes are
// enterprise-only and never compiled into this build.
var tierStreams = map[common.Tier]tierStreamSpec{
	common.TierDefault: {"RAGFLOW_TASKS", []string{common.TaskSubject}, "RAGFLOW_CONSUMER"},
}

// defaultTier for the community build is the single default lane, which is also
// the legacy community queue. The enterprise build (tier_policy_ee.go) designates
// its own default lane.
func defaultTier() common.Tier { return common.TierDefault }
