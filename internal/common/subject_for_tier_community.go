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

import "fmt"

// subjectForTier maps a tier to its JetStream subject in the community/OSS
// build, which has a single default lane. An unknown tier is an error, never a
// silent fallback to the default lane. The enterprise build supplies its own
// subjectForTier in a build-tagged policy file (its full high/medium/default
// lane set), so the tier->subject mapping is edition-specific rather than
// hard-coded in the shared task.go. The exported SubjectForTier delegates to
// this function, keeping the shared signature free of any edition's lane names.
func subjectForTier(tier Tier) (string, error) {
	switch tier {
	case TierDefault:
		return TaskSubject, nil
	default:
		return "", fmt.Errorf("unknown task tier %q", tier)
	}
}
