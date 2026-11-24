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

#include "spm_unigram.h"

#include <algorithm>
#include <limits>
#include <utility>

namespace ragflow {

namespace {

// Unsigned byte order, which is the order darts' builder requires. Sorting with
// std::string's operator< would compare chars (signed on x86) and misorder every
// non-ASCII piece.
bool KeyLess(const std::string &lhs, const std::string &rhs) {
    return std::lexicographical_compare(
        reinterpret_cast<const unsigned char *>(lhs.data()),
        reinterpret_cast<const unsigned char *>(lhs.data()) + lhs.size(),
        reinterpret_cast<const unsigned char *>(rhs.data()),
        reinterpret_cast<const unsigned char *>(rhs.data()) + rhs.size());
}

// Utf8RuneLen is utf8.DecodeRuneInString's size, including its two rules that
// plain "does the continuation bytes exist" checks get wrong: overlong forms and
// surrogates are invalid, and an invalid sequence consumes exactly one byte.
std::size_t Utf8RuneLen(const char *s, std::size_t n, std::size_t i) {
    const auto at = [&](std::size_t k) { return static_cast<unsigned char>(s[k]); };
    const unsigned char c0 = at(i);
    if (c0 < 0x80) {
        return 1;
    }
    const auto is_cont = [&](std::size_t k) { return k < n && (at(k) & 0xC0) == 0x80; };
    const auto in_range = [&](std::size_t k, unsigned char lo, unsigned char hi) {
        return k < n && at(k) >= lo && at(k) <= hi;
    };
    if ((c0 & 0xE0) == 0xC0) {
        return in_range(i + 1, 0x80, 0xBF) ? 2 : 1;
    }
    if ((c0 & 0xF0) == 0xE0) {
        unsigned char lo = 0x80;
        unsigned char hi = 0xBF;
        if (c0 == 0xE0) {
            lo = 0xA0; // reject overlong
        } else if (c0 == 0xED) {
            hi = 0x9F; // reject surrogates
        }
        return in_range(i + 1, lo, hi) && in_range(i + 2, 0x80, 0xBF) ? 3 : 1;
    }
    if ((c0 & 0xF8) == 0xF0) {
        unsigned char lo = 0x80;
        unsigned char hi = 0xBF;
        if (c0 == 0xF0) {
            lo = 0x90; // reject overlong
        } else if (c0 == 0xF4) {
            hi = 0x8F; // reject > U+10FFFF
        } else if (c0 > 0xF4) {
            return 1;
        }
        return in_range(i + 1, lo, hi) && is_cont(i + 2) && is_cont(i + 3) ? 4 : 1;
    }
    return 1;
}

} // namespace

int SPMVocab::Build(std::vector<std::string> keys, std::vector<int32_t> ids,
                    const std::vector<float> &scores, float unk_score, int32_t unk_id,
                    const int32_t *byte_by_value, bool byte_fallback) {
    if (keys.empty() || keys.size() != ids.size() || scores.empty()) {
        return -1;
    }
    for (int32_t id : ids) {
        if (id < 0 || static_cast<std::size_t>(id) >= scores.size()) {
            return -1;
        }
    }

    std::vector<std::size_t> order(keys.size());
    for (std::size_t i = 0; i < order.size(); ++i) {
        order[i] = i;
    }
    std::sort(order.begin(), order.end(),
              [&](std::size_t lhs, std::size_t rhs) { return KeyLess(keys[lhs], keys[rhs]); });

    // darts requires strictly increasing keys: drop duplicates rather than letting
    // the builder produce an unreachable branch.
    std::vector<const char *> key_ptrs;
    std::vector<std::size_t> key_lens;
    std::vector<int> values;
    key_ptrs.reserve(keys.size());
    key_lens.reserve(keys.size());
    values.reserve(keys.size());
    max_key_len_ = 0;
    for (std::size_t k = 0; k < order.size(); ++k) {
        if (k > 0 && keys[order[k]] == keys[order[k - 1]]) {
            continue;
        }
        const std::string &key = keys[order[k]];
        key_ptrs.push_back(key.data());
        key_lens.push_back(key.size());
        values.push_back(static_cast<int>(ids[order[k]]));
        max_key_len_ = std::max(max_key_len_, key.size());
    }
    if (key_ptrs.empty()) {
        return -1;
    }

    if (darts_.build(key_ptrs.size(), key_ptrs.data(), key_lens.data(), values.data(), nullptr) != 0) {
        return -1;
    }

    scores_ = scores;
    unk_score_ = unk_score;
    unk_id_ = unk_id;
    byte_fallback_ = byte_fallback;
    byte_by_value_.assign(256, -1);
    if (byte_by_value != nullptr) {
        for (std::size_t i = 0; i < byte_by_value_.size(); ++i) {
            byte_by_value_[i] = byte_by_value[i];
        }
    }
    return 0;
}

bool SPMVocab::HasPieceAt(const char *s, std::size_t n, std::size_t i) const {
    DartsType::result_pair_type result;
    return darts_.commonPrefixSearch(s + i, &result, 1, n - i) > 0;
}

void SPMVocab::Viterbi(const char *s, std::size_t n, bool want_ids, int32_t *out_ids,
                       std::size_t out_capacity, int32_t *out_count) const {
    constexpr double kNegInf = -std::numeric_limits<double>::max();

    std::vector<double> best(n + 1, kNegInf);
    std::vector<int32_t> count(n + 1, 0);
    std::vector<int32_t> prev;
    std::vector<int32_t> piece_at;
    if (want_ids) {
        prev.assign(n + 1, 0);
        piece_at.assign(n + 1, 0);
    }
    best[0] = 0;

    // One slot per possible match length, so commonPrefixSearch never drops a
    // candidate that could win the Viterbi path.
    std::vector<DartsType::result_pair_type> matches(max_key_len_ + 1);

    for (std::size_t i = 0; i < n; ++i) {
        if (best[i] == kNegInf) {
            continue;
        }
        std::size_t num = darts_.commonPrefixSearch(s + i, matches.data(), matches.size(), n - i);
        const bool matched = num > 0;
        const std::size_t take = std::min(num, matches.size());
        for (std::size_t k = 0; k < take; ++k) {
            const std::size_t end = i + matches[k].length;
            const int32_t id = matches[k].value;
            const double cand = best[i] + static_cast<double>(scores_[id]);
            if (cand > best[end]) {
                best[end] = cand;
                count[end] = count[i] + 1;
                if (want_ids) {
                    prev[end] = static_cast<int32_t>(i);
                    piece_at[end] = id;
                }
            }
        }
        if (matched) {
            continue;
        }

        // No piece starts here: the reference tokenizer consumes the whole RUN of
        // characters that have no piece and bills it as ONE unknown token.
        std::size_t run_end = i;
        while (run_end < n) {
            const std::size_t rune_len = Utf8RuneLen(s, n, run_end);
            if (rune_len == 0 || HasPieceAt(s, n, run_end)) {
                break;
            }
            run_end += rune_len;
        }
        if (run_end == i) {
            // Unreachable: the loop above always advances by at least one rune.
            *out_count = count[i];
            return;
        }
        if (byte_fallback_) {
            double score = 0.0;
            for (std::size_t k = i; k < run_end; ++k) {
                const int32_t idx = byte_by_value_[static_cast<unsigned char>(s[k])];
                score += idx >= 0 ? static_cast<double>(scores_[idx]) : static_cast<double>(unk_score_);
            }
            const double cand = best[i] + score;
            if (cand > best[run_end]) {
                best[run_end] = cand;
                count[run_end] = count[i] + static_cast<int32_t>(run_end - i);
                if (want_ids) {
                    prev[run_end] = static_cast<int32_t>(i);
                    piece_at[run_end] = -2; // one <0xXX> piece per byte
                }
            }
            continue;
        }
        const double cand = best[i] + static_cast<double>(unk_score_);
        if (cand > best[run_end]) {
            best[run_end] = cand;
            count[run_end] = count[i] + 1;
            if (want_ids) {
                prev[run_end] = static_cast<int32_t>(i);
                piece_at[run_end] = -1; // the <unk> piece
            }
        }
    }

    if (best[n] == kNegInf) {
        // Never claim 0 tokens for non-empty text: an under-count is the dangerous
        // direction.
        *out_count = static_cast<int32_t>(n);
        return;
    }
    if (!want_ids) {
        *out_count = count[n];
        return;
    }

    std::vector<int32_t> ids;
    ids.reserve(static_cast<std::size_t>(count[n]));
    for (std::size_t pos = n; pos > 0;) {
        const int32_t tag = piece_at[pos];
        if (tag == -1) {
            ids.push_back(unk_id_);
        } else if (tag == -2) {
            for (std::size_t k = static_cast<std::size_t>(prev[pos]); k < pos; ++k) {
                const int32_t idx = byte_by_value_[static_cast<unsigned char>(s[k])];
                ids.push_back(idx >= 0 ? idx : unk_id_);
            }
        } else {
            ids.push_back(tag);
        }
        pos = static_cast<std::size_t>(prev[pos]);
    }
    std::reverse(ids.begin(), ids.end());
    if (ids.size() > out_capacity) {
        ids.resize(out_capacity);
    }
    std::copy(ids.begin(), ids.end(), out_ids);
    *out_count = static_cast<int32_t>(ids.size());
}

int SPMVocab::Count(const char *text, std::size_t len) const {
    if (text == nullptr || len == 0) {
        return 0;
    }
    int32_t count = 0;
    Viterbi(text, len, false, nullptr, 0, &count);
    return static_cast<int>(count);
}

int SPMVocab::TokenIDs(const char *text, std::size_t len, int32_t *out,
                       std::size_t out_capacity, int32_t *out_count) const {
    if (out_count == nullptr) {
        return -1;
    }
    if (text == nullptr || len == 0) {
        *out_count = 0;
        return 0;
    }
    if (out == nullptr || out_capacity < len) {
        return -1;
    }
    Viterbi(text, len, true, out, out_capacity, out_count);
    return 0;
}

} // namespace ragflow
