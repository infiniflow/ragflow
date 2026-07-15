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

package service

import (
	"strings"
	"testing"
)

// TestValidateParserID_AcceptsRegistryRefs verifies that every
// canonical builtin pipeline id passes validation.
func TestValidateParserID_AcceptsRegistryRefs(t *testing.T) {
	for _, id := range []string{"general", "book", "audio", "qa", "table", "tag"} {
		if err := validateParserID(id); err != nil {
			t.Errorf("validateParserID(%q) = %v, want nil", id, err)
		}
	}
}

// TestValidateParserID_AcceptsNaiveAlias verifies the legacy
// parser_id "naive" still validates (alias for general) so existing
// dataset rows are not rejected.
func TestValidateParserID_AcceptsNaiveAlias(t *testing.T) {
	if err := validateParserID("naive"); err != nil {
		t.Errorf("validateParserID(naive) = %v, want nil (alias for general)", err)
	}
}

// TestValidateParserID_RejectsUnknown verifies unknown/empty
// values are rejected with an error that lists the valid options.
func TestValidateParserID_RejectsUnknown(t *testing.T) {
	for _, id := range []string{"", "unknown", "NAIVE"} {
		err := validateParserID(id)
		if err == nil {
			t.Errorf("validateParserID(%q) = nil, want error", id)
		}
	}

	// The error message should surface the canonical valid options so the
	// caller knows what to send. "general" must appear (canonical), while
	// "naive" is an alias and may or may not appear.
	err := validateParserID("unknown")
	if err == nil {
		t.Fatal("expected error for unknown parser id")
	}
	msg := err.Error()
	if !strings.Contains(msg, "general") {
		t.Errorf("error message %q should mention general", msg)
	}
}
