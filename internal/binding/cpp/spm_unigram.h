// Copyright(C) 2026 InfiniFlow, Inc. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

#pragma once

#include <cstddef>
#include <cstdint>
#include <string>
#include <vector>

#include "darts/darts.h"

namespace ragflow {

// SPMVocab is a SentencePiece Unigram vocabulary whose lookup structure is the
// Darts-clone double-array this repository already ships
// (internal/binding/cpp/darts/darts.h, the same one POSTable/DartsTrie use).
//
// A per-node std::map/Go map trie costs ~190 B per node, i.e. ~96 MB for the
// XLM-R vocabulary (250k pieces expand to 505k nodes). A double-array stores
// one 4-byte unit per slot, so the same vocabulary takes single-digit MB.
//
// The Viterbi loop lives here rather than in the caller because every step of
// it needs an O(1) trie transition; crossing the cgo boundary per byte would
// cost more than the counting itself.
class SPMVocab {
public:
    using DartsType = Darts::DoubleArrayImpl<void, void, int, void>;

    SPMVocab() = default;

    // Build indexes `keys`, associating each with ids[i] (an index into
    // `scores`). keys are copied into the double-array and may be dropped by the
    // caller afterwards.
    //
    // byte_by_value maps a raw byte to the piece index of its "<0xXX>" token, -1
    // when the vocabulary has none. It is only consulted when byte_fallback is
    // true.
    //
    // Returns 0 on success, negative on failure.
    int Build(std::vector<std::string> keys, std::vector<int32_t> ids,
              const std::vector<float> &scores, float unk_score, int32_t unk_id,
              const int32_t *byte_by_value, bool byte_fallback);

    // Count is the Unigram Viterbi token count of text.
    int Count(const char *text, std::size_t len) const;

    // TokenIDs is Count plus the chosen piece ids. `out` must hold at least `len`
    // entries (one token never consumes fewer than one byte). Returns 0 on
    // success.
    int TokenIDs(const char *text, std::size_t len, int32_t *out,
                 std::size_t out_capacity, int32_t *out_count) const;

    // ArrayBytes is the double-array's resident size; the score table is reported
    // separately by ScoresBytes.
    uint64_t ArrayBytes() const {
        return static_cast<uint64_t>(darts_.size()) * static_cast<uint64_t>(darts_.unit_size());
    }

    uint64_t ScoresBytes() const {
        return static_cast<uint64_t>(scores_.capacity()) * sizeof(float);
    }

private:
    // viterbi mirrors the reference implementation exactly: same float64
    // accumulator, same strict-greater tie handling, same unknown-run rules.
    void Viterbi(const char *s, std::size_t n, bool want_ids, int32_t *out_ids,
                 std::size_t out_capacity, int32_t *out_count) const;

    // HasPieceAt reports whether any piece starts at byte offset i, which is what
    // decides whether an unknown-token run ends there.
    bool HasPieceAt(const char *s, std::size_t n, std::size_t i) const;

    DartsType darts_;
    std::vector<float> scores_;
    std::vector<int32_t> byte_by_value_; // 256 entries, -1 when absent
    float unk_score_ = 0.0f;
    int32_t unk_id_ = -1;
    bool byte_fallback_ = false;
    std::size_t max_key_len_ = 0;
};

} // namespace ragflow
