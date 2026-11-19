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

// SentencePiece (Unigram) counter: the XLM-R family.
//
// BAAI/bge-m3, intfloat/multilingual-e5-*, moka-ai/m3e-*, Alibaba-NLP/gte-
// multilingual-base and jina-embeddings-v3 all tokenize with the same
// XLM-RoBERTa SentencePiece model (Unigram, 250k pieces, byte_fallback), which
// is the vocabulary shipped in deps - so one asset and one implementation
// cover the whole family. See internal/tokenizer/embedding_token_limits.md.
//
// What is faithful and what is approximate:
//
//   - pieces, scores, byte-fallback pieces and the whitespace flags are read
//     straight from the .model file, so segmentation of the normalized text is
//     the model's own;
//   - normalization applies NFKC, which is the bulk of the model's `nmt_nfkc`
//     normalizer. The asset additionally carries a precompiled Darts charmap
//     (~232 KB of tables) that SPM applies before NFKC; this implementation does
//     not, so text containing unusual Unicode can normalize slightly
//     differently. Plain prose, tables, digits and CJK - i.e. essentially all
//     ingested content - are unaffected, and the embedder also verifies its
//     counting against the provider's reported usage (Calibration.ObserveUsage)
//     and shrinks-and-retries if a request is nonetheless rejected.
//
// The counter therefore reports Available() as soon as the asset loads, and the
// ingest path treats it as the model's tokenizer while keeping that verification
// running.
//
// Vocabulary lookup and Viterbi run in the native library
// (internal/binding/cpp/spm_unigram.cpp) over the Darts-clone double-array this
// repository already ships. A Go trie with one map per node held ~87 MB for this
// vocabulary (505k nodes at ~190 B each); the double-array needs ~4 MB. The loop
// stays on the C++ side because every step is a trie transition, so a cgo round
// trip per byte would cost more than the counting.
//
// A build without cgo cannot provide the vocabulary: the counter reports itself
// unavailable, and the ingest path refuses to substitute a calibrated estimate
// for a model that declares it (see internal/ingestion/task/embedder.go).

import (
	"crypto/sha1"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode"

	"golang.org/x/text/unicode/norm"

	rag "ragflow/internal/binding"
	"ragflow/internal/common"
)

// SentencePiece piece types (model.proto SentencePiece.Type).
const (
	spmTypeNormal  = 1
	spmTypeUnknown = 2
	spmTypeControl = 3
	spmTypeUser    = 4
	spmTypeUnused  = 5
	spmTypeByte    = 6
)

// sentencePieceModel is a SentencePiece Unigram model, parsed from the .model
// protobuf. Only the fields the counter needs are decoded.
type sentencePieceModel struct {
	pieces       []spmPiece
	vocab        *rag.SPMVocab // Darts-clone double-array over the segmentable pieces
	unkScore     float32
	unkID        int32          // index of the <unk> piece, -1 when the model has none
	byteByValue  map[byte]int32 // "<0xXX>" piece indexes
	byteFallback bool

	addDummyPrefix         bool
	removeExtraWhitespaces bool
	escapeWhitespaces      bool

	normalizerName string
	hasCharmap     bool
}

type spmPiece struct {
	text  string
	score float32
	typ   int32
}

// parseSentencePieceModel decodes what the counter needs from a .model file.
func parseSentencePieceModel(raw []byte) (*sentencePieceModel, error) {
	m := &sentencePieceModel{
		unkID:       -1,
		byteByValue: make(map[byte]int32, 256),
		// model.proto declares these `optional bool ... [default = true]`; proto
		// semantics therefore treat an absent field as true.
		addDummyPrefix:         true,
		removeExtraWhitespaces: true,
		escapeWhitespaces:      true,
	}
	modelType := int64(0)

	r := &pbReader{b: raw}
	for {
		field, wire, err := r.readKey()
		if err != nil {
			if errors.Is(err, errPBEOF) {
				break
			}
			return nil, fmt.Errorf("sentencepiece: %w", err)
		}
		switch {
		case field == 1 && wire == pbWireBytes: // ModelProto.pieces
			pieceBytes, err := r.readBytes()
			if err != nil {
				return nil, fmt.Errorf("sentencepiece: piece: %w", err)
			}
			piece, err := parseSPMPiece(pieceBytes)
			if err != nil {
				return nil, err
			}
			m.pieces = append(m.pieces, piece)
		case field == 2 && wire == pbWireBytes: // ModelProto.trainer_spec
			spec, err := r.readBytes()
			if err != nil {
				return nil, fmt.Errorf("sentencepiece: trainer_spec: %w", err)
			}
			// TrainerSpec.model_type = 3 (1 = UNIGRAM).
			sr := &pbReader{b: spec}
			for {
				f, w, err := sr.readKey()
				if err != nil {
					break
				}
				if f == 3 && w == pbWireVarint {
					v, err := sr.readVarint()
					if err != nil {
						break
					}
					modelType = int64(v)
					continue
				}
				if err := sr.skip(w); err != nil {
					break
				}
			}
		case field == 3 && wire == pbWireBytes: // ModelProto.normalizer_spec
			spec, err := r.readBytes()
			if err != nil {
				return nil, fmt.Errorf("sentencepiece: normalizer_spec: %w", err)
			}
			if err := m.parseNormalizerSpec(spec); err != nil {
				return nil, err
			}
		default:
			if err := r.skip(wire); err != nil {
				return nil, fmt.Errorf("sentencepiece: %w", err)
			}
		}
	}

	if len(m.pieces) == 0 {
		return nil, errors.New("sentencepiece: model has no pieces")
	}
	if modelType != 1 {
		// 1 = UNIGRAM. A BPE model in the same protobuf needs a different
		// encoder; refusing is better than counting with the wrong algorithm.
		return nil, fmt.Errorf("sentencepiece: model_type %d is not Unigram", modelType)
	}

	// The unknown score is the floor SPM uses for pieces it cannot match.
	m.unkScore = math.MaxFloat32
	for i, p := range m.pieces {
		if p.typ == spmTypeUnknown {
			m.unkScore = p.score
			m.unkID = int32(i)
		}
	}
	if m.unkScore == math.MaxFloat32 {
		minScore := m.pieces[0].score
		for _, p := range m.pieces {
			if p.score < minScore {
				minScore = p.score
			}
		}
		m.unkScore = minScore - 10
	}

	// Only the pieces that can take part in segmentation go into the trie; the
	// control/unknown/unused entries are reachable through byte fallback and the
	// unknown-run rules instead.
	keys := make([][]byte, 0, len(m.pieces))
	ids := make([]int32, 0, len(m.pieces))
	scores := make([]float32, len(m.pieces))
	for i := range m.pieces {
		p := &m.pieces[i]
		scores[i] = p.score
		if p.typ == spmTypeControl || p.typ == spmTypeUnknown || p.typ == spmTypeUnused {
			continue
		}
		keys = append(keys, []byte(p.text))
		ids = append(ids, int32(i))
		if p.typ == spmTypeByte && len(p.text) == 6 && strings.HasPrefix(p.text, "<0x") {
			var v int
			if _, err := fmt.Sscanf(p.text, "<0x%02X>", &v); err == nil && v >= 0 && v < 256 {
				m.byteByValue[byte(v)] = int32(i)
			}
		}
	}
	m.byteFallback = len(m.byteByValue) == 256

	byteByValue := [256]int32{}
	for i := range byteByValue {
		byteByValue[i] = -1
	}
	for b, idx := range m.byteByValue {
		byteByValue[b] = idx
	}
	vocab, err := rag.NewSPMVocab(keys, ids, scores, m.unkScore, m.unkID, &byteByValue, m.byteFallback)
	if err != nil {
		return nil, fmt.Errorf("sentencepiece: building the piece trie: %w", err)
	}
	m.vocab = vocab
	return m, nil
}

func parseSPMPiece(raw []byte) (spmPiece, error) {
	piece := spmPiece{typ: spmTypeNormal}
	r := &pbReader{b: raw}
	for {
		field, wire, err := r.readKey()
		if err != nil {
			break
		}
		switch {
		case field == 1 && wire == pbWireBytes: // SentencePiece.piece
			b, err := r.readBytes()
			if err != nil {
				return piece, fmt.Errorf("sentencepiece: piece text: %w", err)
			}
			piece.text = string(b)
		case field == 2 && wire == pbWireFixed32: // SentencePiece.score (float)
			b, err := r.readFixed32()
			if err != nil {
				return piece, fmt.Errorf("sentencepiece: piece score: %w", err)
			}
			piece.score = math.Float32frombits(b)
		case field == 3 && wire == pbWireVarint: // SentencePiece.type
			v, err := r.readVarint()
			if err != nil {
				return piece, fmt.Errorf("sentencepiece: piece type: %w", err)
			}
			piece.typ = int32(v)
		default:
			if err := r.skip(wire); err != nil {
				return piece, fmt.Errorf("sentencepiece: piece: %w", err)
			}
		}
	}
	return piece, nil
}

func (m *sentencePieceModel) parseNormalizerSpec(raw []byte) error {
	r := &pbReader{b: raw}
	for {
		field, wire, err := r.readKey()
		if err != nil {
			break
		}
		switch {
		case field == 1 && wire == pbWireBytes: // name
			b, err := r.readBytes()
			if err != nil {
				return fmt.Errorf("sentencepiece: normalizer name: %w", err)
			}
			m.normalizerName = string(b)
		case field == 2 && wire == pbWireBytes: // precompiled_charsmap
			b, err := r.readBytes()
			if err != nil {
				return fmt.Errorf("sentencepiece: charmap: %w", err)
			}
			m.hasCharmap = len(b) > 0
		case (field == 3 || field == 4 || field == 5) && wire == pbWireVarint:
			v, err := r.readVarint()
			if err != nil {
				return fmt.Errorf("sentencepiece: normalizer flag: %w", err)
			}
			on := v != 0
			switch field {
			case 3:
				m.addDummyPrefix = on
			case 4:
				m.removeExtraWhitespaces = on
			default:
				m.escapeWhitespaces = on
			}
		default:
			if err = r.skip(wire); err != nil {
				return fmt.Errorf("sentencepiece: normalizer_spec: %w", err)
			}
		}
	}
	return nil
}

// normalize mirrors the normalization the models are actually served with.
//
// Verified against the HuggingFace conversion of this exact vocabulary
// (scripts/gen_tokenizer_oracle.py), because that is what providers run:
//
//	normalizer:    Sequence(Precompiled(charmap), Replace(Regex(" {2,}"), " "))
//	pre_tokenizer: Metaspace(replacement="▁", prepend_scheme=always, split=true)
//
// Three details that are easy to get wrong, all pinned by the oracle test:
//
//   - runs of two or more SPACES collapse to one, but whitespace is never
//     trimmed: "hello world " keeps its trailing ▁ and costs one token more than
//     a trimmed version.
//   - any other whitespace run (\n, \t, \r\n) becomes a SINGLE ▁.
//   - a leading ▁ is always prepended, except for empty input and when the text
//     already starts with whitespace (the leading run fills that slot).
//
// DELIBERATELY not driven by the .model's flags. That file declares
// `remove_extra_whitespaces = true` (and would strip the trailing whitespace),
// but the tokenizer providers actually serve is the HuggingFace conversion, which
// drops that flag and keeps the Metaspace behavior above. Following the .model
// made this counter systematically one token short - 15 of 23 oracle samples -
// and the provider, not the file, decides what fits in the window.
//
// Normalization applies NFKC, which is the bulk of the model's `nmt_nfkc`
// normalizer; the precompiled charmap is not applied (see the file header).
func (m *sentencePieceModel) normalize(text string) string {
	if text == "" {
		return ""
	}
	s := collapseSpaces(normalizerLite(norm.NFKC.String(text)))
	if s == "" {
		// Everything the charmap deletes (e.g. a text of nothing but control
		// characters) leaves nothing for the dummy prefix to attach to. Emitting a
		// bare "▁" here would bill one token for text the model sees as empty.
		return ""
	}
	const space = '\u2581' // ▁
	var b strings.Builder
	b.Grow(len(s) + 2)
	b.WriteRune(space)
	prevSpace := false
	for i, r := range s {
		if unicode.IsSpace(r) {
			// A leading whitespace run is absorbed by the dummy prefix.
			if prevSpace || i == 0 {
				prevSpace = true
				continue
			}
			prevSpace = true
			b.WriteRune(space)
			continue
		}
		prevSpace = false
		b.WriteRune(r)
	}
	return b.String()
}

// normalizerLite applies the part of the model's precompiled `nmt_nfkc` charmap
// that NFKC does not cover, so the counter does not spend tokens on characters the
// served tokenizer deletes or turns into whitespace.
//
// The rules below are the ones scripts/gen_tokenizer_oracle.py exercises, each
// verified individually against the model (probe -> what bge-m3 does):
//
//	U+0001..U+0008, U+000B, U+000C, U+000E..U+001F  deleted
//	U+007F..U+0084, U+0086..U+009F                  deleted (0x85 is NOT deleted)
//	U+200B..U+200D, U+FEFF                          become a space
//	U+2581 (the escaped space itself)               becomes a space
//	U+0000, U+0085, U+2060                          kept, each as its own token
//
// This is deliberately a subset: the real charmap is a 232 KB Darts trie with
// hundreds of rules. What is not implemented here is unusual by construction - the
// probe above is what keeps this honest, and the ingest path additionally checks
// its counting against the provider's reported usage (Calibration.ObserveUsage)
// and shrinks-and-retries if a request is rejected anyway.
func normalizerLite(s string) string {
	if !strings.ContainsFunc(s, func(r rune) bool { return isCharmapSpecial(r) }) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case isDeletedByCharmap(r):
			continue
		case isCharmapSpace(r):
			b.WriteRune(' ')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func isCharmapSpecial(r rune) bool {
	return isDeletedByCharmap(r) || isCharmapSpace(r)
}

// isCharmapSpace reports the characters nmt_nfkc maps to a plain space rather than
// deleting them. U+2581 is the escaped space itself: the served tokenizer turns a
// literal "▁" in the input into a space (verified: normalize_str("▁a▁▁b") = " a b"),
// and Metaspace turns it back into one "▁" per run.
func isCharmapSpace(r rune) bool {
	switch r {
	case '\u200B', '\u200C', '\u200D', '\uFEFF', '\u2581':
		return true
	default:
		return false
	}
}

func isDeletedByCharmap(r rune) bool {
	switch {
	case r >= 0x0001 && r <= 0x0008:
		return true
	case r == 0x000B || r == 0x000C:
		return true
	case r >= 0x000E && r <= 0x001F:
		return true
	case r >= 0x007F && r <= 0x0084:
		return true
	case r >= 0x0086 && r <= 0x009F:
		return true
	default:
		return false
	}
}

// collapseSpaces collapses runs of two or more spaces into one, mirroring the
// tokenizer's Replace(Regex(" {2,}"), " ") normalizer step. Other whitespace is
// left untouched.
func collapseSpaces(s string) string {
	if !strings.Contains(s, "  ") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	prevSpace := false
	for _, r := range s {
		if r == ' ' {
			if prevSpace {
				continue
			}
			prevSpace = true
			b.WriteRune(r)
			continue
		}
		prevSpace = false
		b.WriteRune(r)
	}
	return b.String()
}

// countTokens is the Unigram Viterbi count over the normalized text: the number
// of pieces in the highest-scoring segmentation.
func (m *sentencePieceModel) countTokens(text string) int {
	if m.vocab == nil {
		return 0
	}
	s := m.normalize(text)
	if s == "" {
		return 0
	}
	total := 0
	for _, chunk := range splitPreTokens(s) {
		total += m.vocab.Count(chunk)
	}
	return total
}

// preTokenSpace is the escaped-space marker the model's normalizer emits and its
// Metaspace pre-tokenizer splits on.
const preTokenSpace = '\u2581' // ▁

// splitPreTokens splits the normalized text into the chunks the served tokenizer
// actually feeds to its Unigram model.
//
// This matters, and not only for cosmetics: bge-m3's tokenizer.json declares
// `Metaspace(add_prefix_space=true)` with the default `split=true`, so the model
// runs one Viterbi *per whitespace-delimited chunk*, not one over the whole text.
// A global Viterbi is free to pick a different (globally better-scoring)
// segmentation - the incident document has such a spot around byte 46,673, where
// the whole-document optimum splits "343/1" as "▁34"+"3" while the served
// per-chunk optimum splits it as "▁3"+"43" - so counting globally can disagree with
// the provider on both the pieces and, in principle, their number. Our normalized
// string is already the concatenation of the pre-tokens (every "▁" starts one), so
// splitting on "▁" reproduces the served pre-tokenization exactly.
func splitPreTokens(s string) []string {
	if s == "" {
		return nil
	}
	chunks := make([]string, 0, strings.Count(s, string(preTokenSpace))+1)
	start := 0
	for i, r := range s {
		if r == preTokenSpace && i > start {
			chunks = append(chunks, s[start:i])
			start = i
		}
	}
	return append(chunks, s[start:])
}

// tokenIDs returns the exact piece ids the model's encoder emits, one id per token
// (byte fallback contributes one id per byte, exactly as the model does). It backs
// the oracle test's id-level comparison: two different segmentations can share a
// token count, so "same count" is weaker evidence than "same tokens", and a
// count-neutral segmentation bug would otherwise stay invisible.
//
// The ids are the .model file's piece indexes. That is not always the numbering the
// served tokenizer uses - the HuggingFace conversion of bge-m3 reorders the specials
// and inserts <pad>/<mask>, which shifts every regular piece by one - so the oracle
// test compares piece *texts* (tokenNames) for this family and keeps the ids for
// local reasoning.
func (m *sentencePieceModel) tokenIDs(text string) []int32 {
	s := m.normalize(text)
	if s == "" {
		return nil
	}
	var ids []int32
	for _, chunk := range splitPreTokens(s) {
		ids = append(ids, m.vocab.TokenIDs(chunk)...)
	}
	return ids
}

// tokenNames returns the piece text of every token, which is numbering-independent
// and therefore directly comparable with the served tokenizer's own tokens.
func (m *sentencePieceModel) tokenNames(text string) []string {
	ids := m.tokenIDs(text)
	if len(ids) == 0 {
		return nil
	}
	names := make([]string, 0, len(ids))
	for _, id := range ids {
		if id >= 0 && int(id) < len(m.pieces) {
			names = append(names, m.pieces[id].text)
			continue
		}
		names = append(names, "")
	}
	return names
}

// trimToLimit returns the longest rune prefix of text with at most limit tokens.
func (m *sentencePieceModel) trimToLimit(text string, limit int) string {
	return trimToRuneLimitByCount(text, limit, m.countTokens)
}

// sentencePieceCounter adapts a parsed model to the Counter interface.
type sentencePieceCounter struct {
	model *sentencePieceModel
	path  string
}

func (c *sentencePieceCounter) ID() string { return CounterXLMRSentence }

// IDs reports the token ids this counter would emit. It is used by the oracle test
// to compare segmentations, not just their lengths; production code only needs
// Count and TrimToLimit.
func (c *sentencePieceCounter) IDs(text string) []int32 { return c.model.tokenIDs(text) }

// Names reports the piece text of every token; the oracle test prefers this over
// ids because the served tokenizer renumbers this model's specials.
func (c *sentencePieceCounter) Names(text string) []string { return c.model.tokenNames(text) }

// SourcePath is the `.model` file this counter loaded.
func (c *sentencePieceCounter) SourcePath() string { return c.path }

func (c *sentencePieceCounter) Count(text string) int { return c.model.countTokens(text) }

func (c *sentencePieceCounter) TrimToLimit(text string, limit int) string {
	return c.model.trimToLimit(text, limit)
}

func (c *sentencePieceCounter) Available() bool { return c.model != nil }

// expectedSPMHashes pins the on-disk SentencePiece asset. A mismatch means a
// corrupt or tampered file: refuse to load it rather than counting with it.
var expectedSPMHashes = map[string]string{
	"BAAI/bge-m3/sentencepiece.bpe.model": "7e88c49faff6c6c136fdf4a3402d0cb534c6ab10",
}

// xlmrSPMAssetNames are the file names the loader accepts, in priority order.
var xlmrSPMAssetNames = []string{
	"deps/huggingface.co/BAAI/bge-m3/sentencepiece.bpe.model",
	"sentencepiece.bpe.model",
	"xlmr_spm.model",
}

var sentencePieceOnce struct {
	sync.Once
	counter Counter
	err     error
}

// LoadXLMRSentencePieceCounter loads (once) the XLM-R SentencePiece counter. A
// missing asset is not fatal: callers fall back to a calibrated cl100k count.
func LoadXLMRSentencePieceCounter() (Counter, error) {
	sentencePieceOnce.Do(func() {
		sentencePieceOnce.counter, sentencePieceOnce.err = loadXLMRSentencePieceCounter()
	})
	return sentencePieceOnce.counter, sentencePieceOnce.err
}

func loadXLMRSentencePieceCounter() (Counter, error) {
	path, raw, err := readTokenizerAsset(xlmrSPMAssetNames, spmAssetPinKey, expectedSPMHashes)
	if err != nil {
		return nil, err
	}
	model, err := parseSentencePieceModel(raw)
	if err != nil {
		return nil, fmt.Errorf("sentencepiece asset %s: %w", path, err)
	}
	return &sentencePieceCounter{model: model, path: path}, nil
}

// spmAssetPinKey names the pinned file in expectedSPMHashes; the pin applies
// wherever the loader happened to find the same file.
const spmAssetPinKey = "BAAI/bge-m3/sentencepiece.bpe.model"

// readTokenizerAsset finds the first readable candidate and verifies its digest
// when one is pinned. It mirrors bpe_loader.go: disk only, no network I/O, and a
// corrupt file fails loudly instead of being skipped.
func readTokenizerAsset(names []string, pinKey string, pins map[string]string) (string, []byte, error) {
	var tried []string
	// MODEL_ASSETS_DIR wins over wherever the process happens to be running: it is how
	// a deployment points at a mounted asset tree (see common.ModelAssetCandidates).
	for _, name := range names {
		tried = append(tried, common.ModelAssetCandidates(name)...)
	}
	for _, root := range searchRoots() {
		for _, name := range names {
			tried = append(tried, filepath.Join(root, name))
		}
	}
	for _, candidate := range tried {
		raw, err := os.ReadFile(candidate)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return "", nil, fmt.Errorf("reading tokenizer asset %s: %w", candidate, err)
		}
		if want, ok := pins[pinKey]; ok {
			if got := sha1Hex(raw); got != want {
				return "", nil, fmt.Errorf("tokenizer asset %s digest mismatch (got %s, want %s); refusing to load a corrupt or tampered file", candidate, got, want)
			}
		}
		return candidate, raw, nil
	}
	return "", nil, fmt.Errorf("no tokenizer asset found; run `uv run deps/download_deps.py` or set %s to a directory holding them; tried: %s",
		common.EnvModelAssetsDir, strings.Join(tried, ", "))
}

func sha1Hex(raw []byte) string {
	sum := sha1.Sum(raw)
	return fmt.Sprintf("%x", sum)
}

func init() {
	RegisterCounterLoader(CounterXLMRSentence, LoadXLMRSentencePieceCounter)
}

// ---------------------------------------------------------------------------
// Minimal protobuf reader (sentencepiece model.proto, wire format only)
// ---------------------------------------------------------------------------

const (
	pbWireVarint  = 0
	pbWireFixed64 = 1
	pbWireBytes   = 2
	pbWireFixed32 = 5
)

var errPBEOF = errors.New("protobuf: unexpected end of input")

type pbReader struct {
	b []byte
	i int
}

func (r *pbReader) readKey() (int, int, error) {
	v, err := r.readVarint()
	if err != nil {
		return 0, 0, err
	}
	return int(v >> 3), int(v & 7), nil
}

func (r *pbReader) readVarint() (uint64, error) {
	var (
		value uint64
		shift uint
	)
	for {
		if r.i >= len(r.b) {
			return 0, errPBEOF
		}
		c := r.b[r.i]
		r.i++
		value |= uint64(c&0x7F) << shift
		if c&0x80 == 0 {
			return value, nil
		}
		shift += 7
		if shift > 63 {
			return 0, errors.New("protobuf: varint overflow")
		}
	}
}

func (r *pbReader) readBytes() ([]byte, error) {
	length, err := r.readVarint()
	if err != nil {
		return nil, err
	}
	if uint64(r.i)+length > uint64(len(r.b)) {
		return nil, errPBEOF
	}
	out := r.b[r.i : r.i+int(length)]
	r.i += int(length)
	return out, nil
}

func (r *pbReader) readFixed32() (uint32, error) {
	if r.i+4 > len(r.b) {
		return 0, errPBEOF
	}
	v := binary.LittleEndian.Uint32(r.b[r.i : r.i+4])
	r.i += 4
	return v, nil
}

func (r *pbReader) skip(wire int) error {
	switch wire {
	case pbWireVarint:
		_, err := r.readVarint()
		return err
	case pbWireFixed64:
		if r.i+8 > len(r.b) {
			return errPBEOF
		}
		r.i += 8
		return nil
	case pbWireBytes:
		_, err := r.readBytes()
		return err
	case pbWireFixed32:
		_, err := r.readFixed32()
		return err
	default:
		return fmt.Errorf("protobuf: unsupported wire type %d", wire)
	}
}
