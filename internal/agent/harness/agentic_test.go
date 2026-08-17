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

package harness

import "testing"

// TestDiscoveredEntity_EntityTag asserts the entity/keyword tag takes priority.
func TestDiscoveredEntity_EntityTag(t *testing.T) {
	chunks := []map[string]interface{}{
		{"entities_kwd": []string{"Paris", "France"}, "docnm_kwd": "Document A"},
	}
	if got := discoveredEntity(chunks); got != "Paris" {
		t.Errorf("discoveredEntity = %q, want Paris", got)
	}
}

// TestDiscoveredEntity_ImportantKwdString asserts a string important_kwd returns
// its first word.
func TestDiscoveredEntity_ImportantKwdString(t *testing.T) {
	chunks := []map[string]interface{}{
		{"important_kwd": "Quantum Computing"},
	}
	if got := discoveredEntity(chunks); got != "Quantum" {
		t.Errorf("discoveredEntity = %q, want Quantum", got)
	}
}

// TestDiscoveredEntity_DocNameFallback asserts the doc name fallback when no
// entity/keyword tag is present.
func TestDiscoveredEntity_DocNameFallback(t *testing.T) {
	chunks := []map[string]interface{}{
		{"content_with_weight": "some text", "docnm_kwd": "Annual Report 2023"},
	}
	if got := discoveredEntity(chunks); got != "Annual Report 2023" {
		t.Errorf("discoveredEntity = %q, want 'Annual Report 2023'", got)
	}
}

// TestDiscoveredEntity_Empty asserts empty evidence yields empty.
func TestDiscoveredEntity_Empty(t *testing.T) {
	if got := discoveredEntity(nil); got != "" {
		t.Errorf("discoveredEntity(nil) = %q, want empty", got)
	}
	if got := discoveredEntity([]map[string]interface{}{{"content_with_weight": "x"}}); got != "" {
		t.Errorf("discoveredEntity(no name) = %q, want empty", got)
	}
}

// TestNoteEntity asserts empty names never clear a prior discovery.
func TestNoteEntity(t *testing.T) {
	p := &Pipeline{}
	if p.HasDiscoveredEntity() {
		t.Fatal("fresh pipeline must have no discovered entity")
	}
	p.noteEntity("Paris")
	if !p.HasDiscoveredEntity() {
		t.Fatal("noteEntity(Paris) must set discovered entity")
	}
	p.noteEntity("") // fruitless round must not clear
	if !p.HasDiscoveredEntity() {
		t.Error("empty noteEntity must not clear prior discovery")
	}
	p.noteEntity("France")
	if p.lastEntity != "France" {
		t.Errorf("lastEntity = %q, want France", p.lastEntity)
	}
}
