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

import (
	"fmt"
	"strings"
)

// parseIngestorTier normalizes and validates an ingestor-tier CLI/env string.
// The accepted spellings are "", "low" (case-insensitive): an empty/absent
// value defaults to low, and "low" maps to TierDefault. Any other value —
// including "default" and the multi-lane "high"/"medium" — is rejected, because
// the lane topology those names imply is not defined in this build; a silent
// default would mask a misconfiguration. The exported ParseIngestorTier
// delegates to this function.
func parseIngestorTier(s string) (Tier, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "low":
		return TierDefault, nil
	default:
		return "", fmt.Errorf("invalid ingestor tier %q: the open-source build supports only the default (low) lane", s)
	}
}
