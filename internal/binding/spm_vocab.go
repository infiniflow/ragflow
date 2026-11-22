//go:build cgo

// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package rag_analyzer

/*
#include <stdlib.h>
#include "cpp/rag_analyzer_c_api.h"
*/
import "C"

import (
	"fmt"
	"unsafe"
)

// SPMVocab is a SentencePiece Unigram vocabulary (the XLM-R family) whose lookup
// structure is the Darts-clone double-array already compiled into the native
// library.
//
// It exists because a per-node map trie costs roughly 190 bytes per node: the
// XLM-R vocabulary expands 250k pieces into 505k nodes and held ~96 MB of Go
// heap, while the same vocabulary needs ~4 MB of 4-byte double-array units. The
// Viterbi loop lives on the C++ side because every step of it is a trie
// transition, and a cgo round trip per byte would cost more than the counting.
//
// A vocabulary is immutable after NewSPMVocab, so Count and TokenIDs are safe for
// concurrent use. Close must not race with them.
type SPMVocab struct {
	handle C.SPMVocabHandle
}

// NewSPMVocab builds the native vocabulary from the pieces that participate in
// segmentation. keys[i] is the UTF-8 piece text, ids[i] is its index into
// scores, and byteByValue maps a raw byte to its "<0xXX>" piece index (-1 when
// the vocabulary has none); byteByValue is only read when byteFallback is set.
func NewSPMVocab(keys [][]byte, ids []int32, scores []float32, unkScore float32, unkID int32, byteByValue *[256]int32, byteFallback bool) (*SPMVocab, error) {
	if len(keys) == 0 || len(keys) != len(ids) || len(scores) == 0 {
		return nil, fmt.Errorf("spm: invalid vocabulary (keys=%d ids=%d scores=%d)", len(keys), len(ids), len(scores))
	}
	total := 0
	for _, key := range keys {
		total += len(key)
	}
	if total == 0 {
		return nil, fmt.Errorf("spm: vocabulary has no piece bytes")
	}

	// One contiguous block for every key: 250k separate C.CBytes allocations cost
	// far more than the copy itself.
	blob := C.malloc(C.size_t(total))
	if blob == nil {
		return nil, fmt.Errorf("spm: out of memory for %d piece bytes", total)
	}
	defer C.free(blob)
	arena := unsafe.Slice((*byte)(blob), total)

	cKeys := make([]*C.char, len(keys))
	cLens := make([]C.uint32_t, len(keys))
	cIDs := make([]C.int32_t, len(ids))
	offset := 0
	for i, key := range keys {
		if len(key) == 0 {
			return nil, fmt.Errorf("spm: piece %d is empty", i)
		}
		copy(arena[offset:], key)
		cKeys[i] = (*C.char)(unsafe.Pointer(&arena[offset]))
		cLens[i] = C.uint32_t(len(key))
		cIDs[i] = C.int32_t(ids[i])
		offset += len(key)
	}

	cScores := make([]C.float, len(scores))
	for i, score := range scores {
		cScores[i] = C.float(score)
	}

	handle := C.SPM_Vocab_Create(
		(**C.char)(unsafe.Pointer(&cKeys[0])),
		&cLens[0],
		&cIDs[0],
		C.uint32_t(len(keys)),
		&cScores[0],
		C.uint32_t(len(scores)),
		C.float(unkScore),
		C.int32_t(unkID),
		(*C.int32_t)(unsafe.Pointer(byteByValue)),
		C.bool(byteFallback),
	)
	if handle == nil {
		return nil, fmt.Errorf("spm: native vocabulary build failed")
	}
	return &SPMVocab{handle: handle}, nil
}

// Count returns the number of pieces in the highest-scoring segmentation.
func (v *SPMVocab) Count(text string) int {
	if v == nil || v.handle == nil || len(text) == 0 {
		return 0
	}
	var count C.int32_t
	rc := C.SPM_Vocab_Viterbi(
		v.handle,
		(*C.char)(unsafe.Pointer(unsafe.StringData(text))),
		C.uint32_t(len(text)),
		nil,
		0,
		&count,
	)
	if rc != 0 {
		// Never return 0: an under-count makes a provider reject the request.
		panic(fmt.Sprintf("spm: native viterbi failed (rc=%d, len=%d)", int(rc), len(text)))
	}
	return int(count)
}

// TokenIDs returns the piece ids of the chosen segmentation, one entry per token.
func (v *SPMVocab) TokenIDs(text string) []int32 {
	if v == nil || v.handle == nil || len(text) == 0 {
		return nil
	}
	// One token never consumes fewer than one byte, so len(text) is an upper
	// bound on the token count.
	ids := make([]int32, len(text))
	var count C.int32_t
	rc := C.SPM_Vocab_Viterbi(
		v.handle,
		(*C.char)(unsafe.Pointer(unsafe.StringData(text))),
		C.uint32_t(len(text)),
		(*C.int32_t)(unsafe.Pointer(&ids[0])),
		C.uint32_t(len(ids)),
		&count,
	)
	if rc != 0 {
		panic(fmt.Sprintf("spm: native viterbi failed (rc=%d, len=%d)", int(rc), len(text)))
	}
	return ids[:count]
}

// ArrayBytes is the resident size of the double-array, for diagnostics and tests.
func (v *SPMVocab) ArrayBytes() uint64 {
	if v == nil || v.handle == nil {
		return 0
	}
	return uint64(C.SPM_Vocab_ArrayBytes(v.handle))
}

// ScoresBytes is the resident size of the per-piece score table.
func (v *SPMVocab) ScoresBytes() uint64 {
	if v == nil || v.handle == nil {
		return 0
	}
	return uint64(C.SPM_Vocab_ScoresBytes(v.handle))
}

// Close releases the native vocabulary.
func (v *SPMVocab) Close() {
	if v == nil || v.handle == nil {
		return
	}
	C.SPM_Vocab_Destroy(v.handle)
	v.handle = nil
}
