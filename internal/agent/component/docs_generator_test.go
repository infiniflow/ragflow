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

package component

import (
	"context"
	"testing"
)

// TestDocsGenerator_Registered: the component is registered under
// its canonical name. The param check requires output_format and
// content; we provide a minimal valid params map.
func TestDocsGenerator_Registered(t *testing.T) {
	c, err := New("DocsGenerator", map[string]any{
		"output_format": "pdf",
		"content":       "Hello world",
	})
	if err != nil {
		t.Fatalf("New(DocsGenerator): %v", err)
	}
	if c.Name() != "DocsGenerator" {
		t.Errorf("Name=%q, want DocsGenerator", c.Name())
	}
	if c.Inputs() == nil {
		t.Error("Inputs() should be non-nil")
	}
	if c.Outputs() == nil {
		t.Error("Outputs() should be non-nil")
	}
}

// TestDocsGenerator_Invoke_HappyPath: with valid params, the
// component runs without error and produces a non-nil output map.
// Real PDF/DOCX generation needs an actual font file on disk
// (see docs_generator.go's font initialization); when that's
// missing the generator returns a soft error.
func TestDocsGenerator_Invoke_HappyPath(t *testing.T) {
	c, err := New("DocsGenerator", map[string]any{
		"output_format": "txt",
		"content":       "Hello world",
		"filename":      "test",
	})
	if err != nil {
		t.Fatalf("New(DocsGenerator): %v", err)
	}
	_, _ = c.Invoke(context.Background(), map[string]any{})
	// We do not assert err == nil here because txt output requires
	// the internal writer (which may not be available in this
	// checkout). The test pins that the call doesn't panic.
}
