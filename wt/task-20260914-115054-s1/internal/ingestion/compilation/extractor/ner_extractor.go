//
//  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
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

// Package extractor provides NER and relation extraction for the ingestion
// pipeline. It mirrors the Python rag/graphrag/ner package so that both
// code paths produce identical output.
//
// The C++ ThincNER engine (internal/binding/cpp/) loads spaCy model.ckpt+model.bin
// directly for NER inference. Relation extraction is pure Go regex.
//
// Usage:
//
//	ext := extractor.NewExtractor("en")
//	result, err := ext.Extract("Apple Inc. was founded by Steve Jobs.", true)
//	for _, e := range result.Entities { ... }
//	for _, r := range result.Relations { ... }
//go:build cgo_thincner

package extractor
