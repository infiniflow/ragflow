//go:build cgo

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

package chunker

import (
	"errors"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"ragflow/internal/common"
	"ragflow/internal/ingestion/component"
)

func TestGeneralChunkerPDFContinuesWhenStorageResolutionFails(t *testing.T) {
	previousResolver := component.ResolveDocumentStorageOverride
	component.ResolveDocumentStorageOverride = func(string) (*component.DocumentStorageRef, error) {
		return nil, errors.New("storage unavailable")
	}
	t.Cleanup(func() { component.ResolveDocumentStorageOverride = previousResolver })

	core, logs := observer.New(zap.WarnLevel)
	originalLogger := common.Logger
	common.Logger = zap.New(core)
	t.Cleanup(func() { common.Logger = originalLogger })

	chunker, err := NewGeneralChunker(nil)
	if err != nil {
		t.Fatalf("NewGeneralChunker: %v", err)
	}
	out, err := chunker.Invoke(t.Context(), nil, map[string]any{
		"name":          "document.pdf",
		"file_type":     "pdf",
		"doc_id":        "doc-with-missing-storage",
		"output_format": "json",
		"json":          []map[string]any{{"text": "body", "doc_type_kwd": "text"}},
	})
	if err != nil {
		t.Fatalf("GeneralChunker.Invoke: %v", err)
	}
	if logs.FilterMessage("GeneralChunker: could not open PDF for on-demand cropping").Len() != 1 {
		t.Fatalf("logs = %v, want PDF open warning", logs.All())
	}
	chunks := outputChunks(t, out)
	if len(chunks) != 1 || chunks[0]["text"] != "body" {
		t.Fatalf("chunks = %#v, want body chunk after crop fallback", chunks)
	}
}
