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

package tokenizer

// BERT WordPiece counter: the bge-*-en / e5-* / gte-base / jina-v2 family.
//
// These models are BERT-derived, so they tokenize with a 30k WordPiece vocab
// (`vocab.txt`) plus BERT's own text processing. Everything here mirrors the
// served tokenizer - the HuggingFace conversion of the same vocab, verified by
// scripts/gen_tokenizer_oracle.py:
//
//	normalizer:    BertNormalizer(clean_text=true, handle_chinese_chars=true,
//	                              strip_accents=null (follows lowercase=true), lowercase=true)
//	pre_tokenizer: BertPreTokenizer (whitespace, punctuation and CJK chars split)
//	model:         WordPiece(continuing_subword_prefix="##", max_input_chars_per_word=100)
//
// Note the asymmetry with the XLM-R counter: this family lowercases and strips
// accents, XLM-R does neither. Counting "HELLO" as one token for both would be
// wrong for one of them.

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// wordPieceMaxCharsPerWord mirrors the model's max_input_chars_per_word: longer
// words become a single unk token instead of being split.
const wordPieceMaxCharsPerWord = 100

// wordPieceModel is a WordPiece vocabulary plus BERT's text processing flags.
type wordPieceModel struct {
	vocab           map[string]int32
	unkToken        string
	continuing      string
	lowercase       bool
	stripAccents    bool
	cleanText       bool
	handleChinese   bool
	maxCharsPerWord int

	// names is the id -> piece map, built on first use: only the oracle test asks
	// for token texts, and production would not want a second copy of the vocab.
	nameOnce sync.Once
	names    map[int32]string
}

// bgLargeEnVocabPin is the digest of the shipped vocab.txt.
var expectedWordPieceHashes = map[string]string{
	"BAAI/bge-large-en-v1.5/vocab.txt": "c3b4105373feaa5b0b2c332321c1e592d6f39658",
}

const wordPieceAssetPinKey = "BAAI/bge-large-en-v1.5/vocab.txt"

var bertWordPieceAssetNames = []string{
	"ragflow_deps/huggingface.co/BAAI/bge-large-en-v1.5/vocab.txt",
	"vocab.txt",
	"bert_wordpiece_vocab.txt",
}

// parseWordPieceVocab reads one token per line, index order.
func parseWordPieceVocab(raw []byte) (map[string]int32, error) {
	lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
	vocab := make(map[string]int32, len(lines))
	for i, line := range lines {
		if line == "" && i == len(lines)-1 {
			continue
		}
		token := strings.TrimSuffix(line, "\r")
		if _, dup := vocab[token]; dup {
			continue
		}
		vocab[token] = int32(i)
	}
	if len(vocab) == 0 {
		return nil, errors.New("wordpiece: empty vocabulary")
	}
	return vocab, nil
}

// normalize applies BertNormalizer's steps in the order HF applies them.
func (m *wordPieceModel) normalize(text string) string {
	s := text
	if m.cleanText {
		s = cleanBertText(s)
	}
	if m.handleChinese {
		s = padChineseChars(s)
	}
	if m.lowercase {
		s = strings.ToLower(s)
	}
	if m.stripAccents {
		s = stripAccents(s)
	}
	return s
}

// cleanBertText removes control characters and normalizes whitespace, mirroring
// BertNormalizer.clean_text.
//
// The distinction that matters: whitespace is *replaced* by a space (so it still
// separates words), while control characters are *deleted*. Format characters
// (category Cf: zero-width space/joiner, word joiner, BOM) are deleted too - the
// oracle shows "a\u200bb" tokenizing as a single "ab". Treating either class as a
// space splits words the model keeps together, and leaves the counter over an
// entire token per occurrence.
func cleanBertText(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r == 0 || r == 0xFFFD:
			continue
		case unicode.IsSpace(r): // includes \t \n \r and NBSP-like spaces
			b.WriteRune(' ')
		case unicode.IsControl(r) || unicode.In(r, unicode.Cf):
			continue
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// padChineseChars puts a space around every CJK character, mirroring
// BertNormalizer.handle_chinese_chars.
func padChineseChars(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 8)
	prevSpace := true
	for _, r := range s {
		if isChineseChar(r) {
			if !prevSpace {
				b.WriteRune(' ')
			}
			b.WriteRune(r)
			b.WriteRune(' ')
			prevSpace = true
			continue
		}
		if r == ' ' {
			prevSpace = true
		} else {
			prevSpace = false
		}
		b.WriteRune(r)
	}
	return b.String()
}

// isChineseChar reports whether r is in a CJK block, using the same ranges BERT
// uses (CJK Unified Ideographs and extensions, and the CJK symbols block).
func isChineseChar(r rune) bool {
	return (r >= 0x4E00 && r <= 0x9FFF) ||
		(r >= 0x3400 && r <= 0x4DBF) ||
		(r >= 0x20000 && r <= 0x2A6DF) ||
		(r >= 0x2A700 && r <= 0x2B73F) ||
		(r >= 0x2B740 && r <= 0x2B81F) ||
		(r >= 0x2B820 && r <= 0x2CEAF) ||
		(r >= 0xF900 && r <= 0xFAFF) ||
		(r >= 0x2F800 && r <= 0x2FA1F)
}

// stripAccents removes combining marks, mirroring BertNormalizer.strip_accents.
//
// NFD first, then drop the marks. (An earlier version short-circuited when the
// input was already NFD, which is exactly the case that still has marks to
// remove: "e\u0301" is NFD, and the counter kept the accent as an extra token.)
func stripAccents(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range norm.NFD.String(s) {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// preTokenize splits text the way BertPreTokenizer does: on whitespace, around
// punctuation, and around CJK characters.
func preTokenizeBert(s string) []string {
	var (
		out   []string
		cur   strings.Builder
		flush = func() {
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
		}
	)
	for _, r := range s {
		switch {
		case unicode.IsSpace(r):
			flush()
		case isPunctuation(r):
			flush()
			out = append(out, string(r))
		case isChineseChar(r):
			flush()
			out = append(out, string(r))
		default:
			cur.WriteRune(r)
		}
	}
	flush()
	return out
}

// isPunctuation mirrors BERT's _is_punctuation: the ASCII ranges between letters
// and digits, plus Unicode category **P** — and only P. Symbols (category S, which
// includes every emoji) are NOT punctuation for BERT, so "🚀🔥" stays inside a
// word instead of becoming one token per character. Verified by the oracle: the
// emoji sample is what caught an IsSymbol() added here by mistake.
func isPunctuation(r rune) bool {
	if (r >= 33 && r <= 47) || (r >= 58 && r <= 64) || (r >= 91 && r <= 96) || (r >= 123 && r <= 126) {
		return true
	}
	return unicode.IsPunct(r)
}

// countTokens is the WordPiece greedy longest-match count, including the single
// unk token a word collapses to when nothing matches.
func (m *wordPieceModel) countTokens(text string) int {
	n, _ := m.encode(text, false)
	return n
}

// tokenIDs returns the exact token ids, for the oracle test's id-level comparison
// (two segmentations can share a count; only the ids prove they are the same).
func (m *wordPieceModel) tokenIDs(text string) []int32 {
	_, ids := m.encode(text, true)
	return ids
}

// tokenNames returns the piece of every token ("##"-prefixed for continuations,
// the model's unk token for unknowns), which is what the served tokenizer reports.
func (m *wordPieceModel) tokenNames(text string) []string {
	n, ids := m.encode(text, true)
	if n == 0 {
		return nil
	}
	m.nameOnce.Do(func() {
		m.names = make(map[int32]string, len(m.vocab))
		for piece, id := range m.vocab {
			m.names[id] = piece
		}
	})
	names := make([]string, 0, len(ids))
	for _, id := range ids {
		names = append(names, m.names[id])
	}
	return names
}

// encode runs BERT's greedy longest-match word splitting, returning the token count
// and - when wantIDs is set - the token ids. Both live in one function so the count
// path and the id path cannot drift apart.
func (m *wordPieceModel) encode(text string, wantIDs bool) (int, []int32) {
	s := m.normalize(text)
	if strings.TrimSpace(s) == "" {
		return 0, nil
	}
	total := 0
	var ids []int32
	unkID := m.vocab[m.unkToken]
	appendID := func(id int32) {
		total++
		if wantIDs {
			ids = append(ids, id)
		}
	}
	// One word at a time. If ANY position of a word cannot be matched, the whole
	// word collapses to a single [UNK] and the pieces already matched for it are
	// dropped - that is what BERT's reference implementation does
	// (`if is_bad: split_tokens.append(self.unk_token)`), and what the HuggingFace
	// WordPiece model does (`is_bad` -> one unk). Emitting "matched prefix + one
	// unk for the rest" instead over-counts such words, which is what the
	// id-level oracle found on the fuzz corpus.
	var pieces []int32
	for _, word := range preTokenizeBert(s) {
		if word == "" {
			continue
		}
		if len([]rune(word)) > m.maxCharsPerWord {
			appendID(unkID) // the whole word becomes unk
			continue
		}
		pieces = pieces[:0]
		start := 0
		runes := []rune(word)
		bad := false
		for start < len(runes) {
			end := len(runes)
			matched := false
			for end > start {
				piece := string(runes[start:end])
				if start > 0 {
					piece = m.continuing + piece
				}
				if id, ok := m.vocab[piece]; ok {
					pieces = append(pieces, id)
					start = end
					matched = true
					break
				}
				end--
			}
			if !matched {
				bad = true
				break
			}
		}
		if bad {
			appendID(unkID)
			continue
		}
		for _, id := range pieces {
			appendID(id)
		}
	}
	return total, ids
}

// trimToLimit returns the longest rune prefix with at most limit tokens.
func (m *wordPieceModel) trimToLimit(text string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if m.countTokens(text) <= limit {
		return text
	}
	runes := []rune(text)
	lo, hi := 0, len(runes)
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if m.countTokens(string(runes[:mid])) <= limit {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return string(runes[:lo])
}

type wordPieceCounter struct {
	model *wordPieceModel
	path  string
}

func (c *wordPieceCounter) ID() string { return CounterBERTWordPiece }

// IDs reports the token ids, for the oracle test's id-level comparison.
func (c *wordPieceCounter) IDs(text string) []int32 { return c.model.tokenIDs(text) }

// Names reports the piece of every token, which the oracle test compares directly
// with the served tokenizer's tokens.
func (c *wordPieceCounter) Names(text string) []string { return c.model.tokenNames(text) }

// SourcePath is the vocab file this counter loaded.
func (c *wordPieceCounter) SourcePath() string { return c.path }

func (c *wordPieceCounter) Count(text string) int { return c.model.countTokens(text) }

func (c *wordPieceCounter) TrimToLimit(text string, limit int) string {
	return c.model.trimToLimit(text, limit)
}

func (c *wordPieceCounter) Available() bool { return c.model != nil }

var wordPieceOnce struct {
	sync.Once
	counter Counter
	err     error
}

// LoadBERTWordPieceCounter loads (once) the BERT WordPiece counter from the
// shipped vocab.txt. A missing asset is not fatal: callers fall back to a
// calibrated cl100k count.
func LoadBERTWordPieceCounter() (Counter, error) {
	wordPieceOnce.Do(func() {
		wordPieceOnce.counter, wordPieceOnce.err = loadBERTWordPieceCounter()
	})
	return wordPieceOnce.counter, wordPieceOnce.err
}

func loadBERTWordPieceCounter() (Counter, error) {
	path, raw, err := readTokenizerAsset(bertWordPieceAssetNames, wordPieceAssetPinKey, expectedWordPieceHashes)
	if err != nil {
		return nil, err
	}
	vocab, err := parseWordPieceVocab(raw)
	if err != nil {
		return nil, fmt.Errorf("wordpiece asset %s: %w", path, err)
	}
	model := &wordPieceModel{
		vocab:           vocab,
		unkToken:        "[UNK]",
		continuing:      "##",
		lowercase:       true,
		stripAccents:    true,
		cleanText:       true,
		handleChinese:   true,
		maxCharsPerWord: wordPieceMaxCharsPerWord,
	}
	if _, ok := vocab[model.unkToken]; !ok {
		return nil, fmt.Errorf("wordpiece asset %s has no %s token", filepath.Base(path), model.unkToken)
	}
	return &wordPieceCounter{model: model, path: path}, nil
}

func init() {
	RegisterCounterLoader(CounterBERTWordPiece, LoadBERTWordPieceCounter)
}
