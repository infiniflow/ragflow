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

package entity

import "strings"

// ModelType represents the type of model as a bitmask, matching Python's
// ModelTypeBinary enum (common/constants.py).
// Bit flags (LSB->MSB): 1=chat, 2=embedding, 4=asr, 8=vision, 16=rerank, 32=tts, 64=ocr,
// 128=decision. The first seven mirror Python's ModelTypeBinary; `decision` has
// no Python counterpart — it exists only in this Go implementation, and a
// migration the other way would have to drop or re-map it.
// A model can belong to multiple types simultaneously.
type ModelType int

const (
	// ModelTypeChat chat model (1 << 0)
	ModelTypeChat ModelType = 1 << iota
	// ModelTypeEmbedding embedding model (1 << 1)
	ModelTypeEmbedding
	// ModelTypeSpeech2Text speech to text model (1 << 2)
	ModelTypeSpeech2Text
	// ModelTypeImage2Text image to text model (1 << 3)
	ModelTypeImage2Text
	// ModelTypeRerank rerank model (1 << 4)
	ModelTypeRerank
	// ModelTypeTTS text to speech model (1 << 5)
	ModelTypeTTS
	// ModelTypeOCR optical character recognition model (1 << 6)
	ModelTypeOCR
	// ModelTypeDecision decision model (1 << 7): the model that judges whether a
	// retrieved passage is worth putting in front of the chat model. It is kept
	// as its own category rather than a chat model with a flag because the two
	// are selected independently — a tenant may want a cheap judge in front of an
	// expensive answering model — and because the judge is spoken to in a
	// different protocol (TypeSafe System One: one request carrying several
	// questions), so a plain chat model must not silently be usable as one.
	ModelTypeDecision
)

// Has returns true if the bitmask contains the given model type.
func (mt ModelType) Has(target ModelType) bool {
	return int(mt)&int(target) != 0
}

// String returns the string representation of the model type.
// For a single-bit value returns the corresponding name.
// For multi-bit values returns comma-separated names (e.g. "chat,embedding").
func (mt ModelType) String() string {
	if mt == 0 {
		return ""
	}
	parts := make([]string, 0, 4)
	if mt.Has(ModelTypeChat) {
		parts = append(parts, "chat")
	}
	if mt.Has(ModelTypeEmbedding) {
		parts = append(parts, "embedding")
	}
	if mt.Has(ModelTypeSpeech2Text) {
		parts = append(parts, "speech2text")
	}
	if mt.Has(ModelTypeImage2Text) {
		parts = append(parts, "image2text")
	}
	if mt.Has(ModelTypeRerank) {
		parts = append(parts, "rerank")
	}
	if mt.Has(ModelTypeTTS) {
		parts = append(parts, "tts")
	}
	if mt.Has(ModelTypeOCR) {
		parts = append(parts, "ocr")
	}
	if mt.Has(ModelTypeDecision) {
		parts = append(parts, "decision")
	}
	return strings.Join(parts, ",")
}

// HumanReadable returns the string names of all model types in this bitmask.
func (mt ModelType) HumanReadable() []string {
	if mt == 0 {
		return nil
	}
	parts := make([]string, 0, 4)
	if mt.Has(ModelTypeChat) {
		parts = append(parts, "chat")
	}
	if mt.Has(ModelTypeEmbedding) {
		parts = append(parts, "embedding")
	}
	if mt.Has(ModelTypeSpeech2Text) {
		parts = append(parts, "asr")
	}
	if mt.Has(ModelTypeImage2Text) {
		parts = append(parts, "vision")
	}
	if mt.Has(ModelTypeRerank) {
		parts = append(parts, "rerank")
	}
	if mt.Has(ModelTypeTTS) {
		parts = append(parts, "tts")
	}
	if mt.Has(ModelTypeOCR) {
		parts = append(parts, "ocr")
	}
	if mt.Has(ModelTypeDecision) {
		parts = append(parts, "decision")
	}
	return parts
}

// ModelTypeFromString converts a string to a ModelType.
func ModelTypeFromString(s string) ModelType {
	switch s {
	case "chat":
		return ModelTypeChat
	case "embedding":
		return ModelTypeEmbedding
	case "asr", "speech2text":
		return ModelTypeSpeech2Text
	case "vision", "image2text":
		return ModelTypeImage2Text
	case "rerank":
		return ModelTypeRerank
	case "tts":
		return ModelTypeTTS
	case "ocr":
		return ModelTypeOCR
	case "decision":
		return ModelTypeDecision
	default:
		return 0
	}
}

// ModelTypeFromStrings converts multiple strings to a combined bitmask ModelType.
// This matches Python's calculate_model_type() which ORs together multiple type values.
func ModelTypeFromStrings(types []string) ModelType {
	var result ModelType
	for _, t := range types {
		result |= ModelTypeFromString(t)
	}
	return result
}

// ModelVerifyStatus mirrors Python's ModelVerifyStatusEnum
// (common/constants.py).
const (
	ModelVerifySuccess = "success"
	ModelVerifyFail    = "fail"
	ModelVerifyUnknown = "unknown"
)
