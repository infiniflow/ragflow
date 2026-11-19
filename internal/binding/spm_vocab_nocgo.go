//go:build !cgo

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

import "fmt"

// SPMVocab is the no-CGO stub: the Darts-backed vocabulary lives in the native
// library, so a build without cgo cannot provide it. The SentencePiece counter
// reports itself unavailable and the ingest path refuses to substitute a
// calibrated estimate, exactly as it does for a missing asset.
type SPMVocab struct{}

func NewSPMVocab(keys [][]byte, ids []int32, scores []float32, unkScore float32, unkID int32, byteByValue *[256]int32, byteFallback bool) (*SPMVocab, error) {
	return nil, fmt.Errorf("rag_analyzer: cgo required for the SentencePiece vocabulary (pieces=%d)", len(keys))
}

func (v *SPMVocab) Count(string) int { return 0 }

func (v *SPMVocab) TokenIDs(string) []int32 { return nil }

func (v *SPMVocab) ArrayBytes() uint64 { return 0 }

func (v *SPMVocab) ScoresBytes() uint64 { return 0 }

func (v *SPMVocab) Close() {}
