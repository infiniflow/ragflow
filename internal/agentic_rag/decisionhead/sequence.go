// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package decisionhead

import (
	"strings"
	"unicode/utf8"

	"ragflow/internal/tokenizer"
)

// Sequence is one model input: the token ids, the marker positions the decision
// head reads (one per option, at that option's [MASK]), and the question-type id
// the contract assigns.
type Sequence struct {
	IDs     []int32
	Markers []int32
	QType   int64
}

// Options for the noul decision: a fixed two-way choice where index 1 carries
// the "sufficient" probability. The reference implementation renders them the
// same way for every request.
var noulOptions = []string{"false", "true"}

// BuildSequence assembles the contract sequence for one decision:
//
//	[CLS] {qtype} question: {instructions} [SEP] [MASK] opt0 [MASK] opt1 ... [SEP] {state} [SEP]
//
// The head and the options share head_max_len; when the options do not fit they
// are shortened TOGETHER (never one alone, or the options stop being
// comparable) and the head keeps at least 8 tokens. The state takes the room
// left under max_len and a trailing [SEP] is appended before the whole thing is
// truncated; markers past max_len are dropped (DEPLOY_CONTRACT.md §四).
//
// Every span goes through both scrub layers, so neither a literal marker nor one
// that only appears after tokenisation can shift the marker positions, and a
// separator cannot be injected into the structure (§3.2).
func BuildSequence(enc tokenizer.MetaspaceEncoder, c *Contract, qtype, instructions string, optionTexts []string, state string) (Sequence, error) {
	qtypeID, err := c.qtypeID(qtype)
	if err != nil {
		return Sequence{}, err
	}
	special := c.SpecialIDs
	span := func(text string) []int32 {
		return scrubIDs(enc.IDs(scrubText(text, c.MaskToken)), special)
	}

	head := span(renderHead(c.HeadFormat, qtype, instructions))
	options := make([][]int32, 0, len(optionTexts))
	for _, text := range optionTexts {
		// The option text is prefixed with one space before encoding.
		textIDs := span(" " + text)
		if len(textIDs) > c.OptionTokenCap {
			textIDs = textIDs[:c.OptionTokenCap]
		}
		options = append(options, append([]int32{special["mask"]}, textIDs...))
	}

	// Shared budget: shrink the options together when the head would not fit.
	budget := c.HeadMaxLen - totalLen(options)
	if budget < 16 && len(options) > 0 {
		perOption := max(4, (c.HeadMaxLen-16)/len(options))
		for i, option := range options {
			if len(option) > perOption {
				options[i] = option[:perOption]
			}
		}
		budget = c.HeadMaxLen - totalLen(options)
	}
	if len(head) > max(8, budget) {
		head = head[:max(8, budget)]
	}

	seq := make([]int32, 0, len(head)+totalLen(options)+len(state)+8)
	seq = append(seq, special["cls"])
	seq = append(seq, head...)
	seq = append(seq, special["sep"])
	markers := make([]int32, 0, len(options))
	for _, option := range options {
		markers = append(markers, int32(len(seq)))
		seq = append(seq, option...)
	}
	seq = append(seq, special["sep"])
	room := max(0, c.MaxLen-len(seq)-1)
	stateIDs := span(state)
	if len(stateIDs) > room {
		stateIDs = stateIDs[:room]
	}
	seq = append(seq, stateIDs...)
	seq = append(seq, special["sep"])
	if len(seq) > c.MaxLen {
		seq = seq[:c.MaxLen]
	}
	kept := markers[:0]
	for _, m := range markers {
		if int(m) < len(seq) {
			kept = append(kept, m)
		}
	}
	return Sequence{IDs: seq, Markers: kept, QType: qtypeID}, nil
}

// renderHead fills the contract's head format. The connective " question: " is
// English in every language — it is a literal part of the template, not
// something the language routing touches.
func renderHead(format, qtype, instructions string) string {
	return strings.NewReplacer(
		"{qtype}", qtype,
		"{instructions}", instructions,
	).Replace(format)
}

// scrubText is the string half of the marker-forgery defence: untrusted text
// must not carry a literal marker, or the caller could plant one and move the
// positions the model reads its decision from. All spellings seen in the wild
// are removed.
func scrubText(text, maskToken string) string {
	if text == "" {
		return ""
	}
	for _, token := range []string{maskToken, "[MASK]", "<mask>"} {
		if token != "" {
			text = strings.ReplaceAll(text, token, "")
		}
	}
	return text
}

// scrubIDs is the id half — the authoritative one. It is the only way to prove
// a span carries no marker (a marker can survive tokenisation in another form),
// and it also closes separator injection: a crafted state could otherwise embed
// a separator and forge a structural boundary the string scrub never covered.
func scrubIDs(ids []int32, special map[string]int32) []int32 {
	drop := map[int32]bool{special["mask"]: true, special["cls"]: true, special["sep"]: true}
	out := ids[:0]
	for _, id := range ids {
		if !drop[id] {
			out = append(out, id)
		}
	}
	return out
}

// SplitSegments cuts text into segments of at most segmentTokens tokens,
// preferring line boundaries. A chunk longer than the budget is judged segment
// by segment; the decision for the chunk is the OR over its segments (any
// segment that helps makes the chunk worth showing). Text past maxSegments is
// returned as overflow and must be kept UNJUDGED by the caller — it is not
// evidence of uselessness, only of an unpaid-for bill.
//
// Token counts are additive across line boundaries: a newline is its own merge
// unit and the unit after it is re-prefixed, exactly as when the line is
// counted alone, so per-line counts can be summed without re-encoding.
func SplitSegments(enc tokenizer.MetaspaceEncoder, text string, segmentTokens, maxSegments int) (segments []string, overflow string) {
	if text == "" || segmentTokens <= 0 || maxSegments <= 0 {
		return nil, text
	}
	if enc.Count(text) <= segmentTokens {
		return []string{text}, ""
	}
	segments = splitWithinBudget(enc, text, segmentTokens)
	if len(segments) <= maxSegments {
		return segments, ""
	}
	kept := segments[:maxSegments]
	consumed := 0
	for _, s := range kept {
		consumed += len(s)
	}
	return kept, text[consumed:]
}

// splitWithinBudget packs whole lines into segments until the next line would
// exceed the budget; a single oversized line is cut at rune boundaries.
func splitWithinBudget(enc tokenizer.MetaspaceEncoder, text string, segmentTokens int) []string {
	var out []string
	var cur strings.Builder
	curTokens := 0
	for _, line := range splitLinesKeepNewline(text) {
		n := enc.Count(line)
		if n > segmentTokens {
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
				curTokens = 0
			}
			out = append(out, splitLineByRunes(enc, line, segmentTokens)...)
			continue
		}
		if curTokens > 0 && curTokens+n > segmentTokens {
			out = append(out, cur.String())
			cur.Reset()
			curTokens = 0
		}
		cur.WriteString(line)
		curTokens += n
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

// splitLinesKeepNewline cuts after every '\n', keeping it at the end of the
// line it terminates.
func splitLinesKeepNewline(text string) []string {
	var lines []string
	for len(text) > 0 {
		i := strings.IndexByte(text, '\n')
		if i < 0 {
			lines = append(lines, text)
			break
		}
		lines = append(lines, text[:i+1])
		text = text[i+1:]
	}
	return lines
}

// splitLineByRunes cuts one oversized line into rune-aligned pieces, each within
// the token budget. Binary search over rune prefixes: Count is monotone in the
// prefix length.
func splitLineByRunes(enc tokenizer.MetaspaceEncoder, line string, segmentTokens int) []string {
	var out []string
	for len(line) > 0 {
		lo, hi, best := 1, utf8.RuneCountInString(line), 0
		for lo <= hi {
			mid := (lo + hi) / 2
			if enc.Count(runePrefix(line, mid)) <= segmentTokens {
				best = mid
				lo = mid + 1
			} else {
				hi = mid - 1
			}
		}
		if best == 0 {
			best = 1 // one rune is at most 4 byte-fallback tokens
		}
		piece := runePrefix(line, best)
		out = append(out, piece)
		line = line[len(piece):]
	}
	return out
}

// runePrefix returns the first n runes of s.
func runePrefix(s string, n int) string {
	count := 0
	for i := range s {
		if count == n {
			return s[:i]
		}
		count++
	}
	return s
}

func totalLen(parts [][]int32) int {
	total := 0
	for _, p := range parts {
		total += len(p)
	}
	return total
}

// TruncateToTokens returns the longest prefix of text that fits in maxTokens
// tokens. Whole lines are kept when they fit and a single line taller than the
// entire budget is cut at a rune boundary — the same fallback splitWithinBudget
// uses, so both policies count tokens identically.
func TruncateToTokens(enc tokenizer.MetaspaceEncoder, text string, maxTokens int) string {
	if text == "" || maxTokens <= 0 {
		return ""
	}
	if enc.Count(text) <= maxTokens {
		return text
	}
	var b strings.Builder
	used := 0
	for _, line := range splitLinesKeepNewline(text) {
		n := enc.Count(line)
		if used+n > maxTokens {
			break
		}
		b.WriteString(line)
		used += n
	}
	if b.Len() == 0 {
		if pieces := splitLineByRunes(enc, text, maxTokens); len(pieces) > 0 {
			return pieces[0]
		}
	}
	return b.String()
}
