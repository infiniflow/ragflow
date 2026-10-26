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

// Package decisionhead runs the typed-decision model (ONNX) in-process: given a
// question, a set of option texts and a state (evidence text), it scores each
// option and the decision is the argmax. RAGFlow asks it one question only —
// "noul", whether a retrieved chunk helps answer the question — before the
// chunk is shown to the chat model.
//
// Everything the model expects is a training-side contract shipped in
// laya_config.json next to the checkpoint (special token ids, instruction
// templates, language routing, head format, temperature). It is read, never
// hardcoded: mmBERT has no [MASK] token, so a literal-name lookup resolves to
// the unk id and moves every marker without anything raising
// (DEPLOY_CONTRACT.md §一, §七).
package decisionhead

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Contract is the deploy-side view of laya_config.json. Fields the training side
// declares are reproduced verbatim here; nothing in this package invents a
// token id, a template or a temperature.
type Contract struct {
	MaxLen               int                            `json:"max_len"`
	HeadMaxLen           int                            `json:"head_max_len"`
	OptionTokenCap       int                            `json:"option_token_cap"`
	QTypes               map[string]int                 `json:"qtypes"`
	SpecialIDs           map[string]int32               `json:"special_ids"`
	MaskToken            string                         `json:"mask_token"`
	InstructionTemplates map[string]map[string][]string `json:"instruction_templates"`
	LangRule             LangRule                       `json:"instruction_lang_rule"`
	HeadFormat           string                         `json:"head_format"`
	Temperature          float64                        `json:"temperature"`
	TemperatureByOptions map[string]float64             `json:"temperature_by_options"`

	cjkRanges []cjkRange
}

// LangRule is instruction_lang_rule, reproduced so the routing is byte-for-byte
// the training side's: the language follows the QUESTION's CJK ratio, never the
// caller's habit or the state's language (a state seen with both a Chinese and
// an English question lands in two distributions and the calibration drifts).
type LangRule struct {
	CJKThreshold float64 `json:"cjk_threshold"`
	Fallback     string  `json:"fallback"`
}

// cjkRange is one "lo-hi" hex range from the contract's cjk_ranges.
type cjkRange struct{ lo, hi rune }

// LoadContract reads the training-side contract from path.
func LoadContract(path string) (*Contract, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("decisionhead: reading contract %s: %w", path, err)
	}
	var c Contract
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, fmt.Errorf("decisionhead: parsing contract %s: %w", path, err)
	}
	if c.MaxLen <= 0 || c.HeadMaxLen <= 0 || c.OptionTokenCap <= 0 {
		return nil, fmt.Errorf("decisionhead: contract %s has no usable length budget", path)
	}
	for _, role := range []string{"cls", "sep", "mask", "pad"} {
		if _, ok := c.SpecialIDs[role]; !ok {
			return nil, fmt.Errorf("decisionhead: contract %s declares no special_ids[%q]", path, role)
		}
	}
	if c.MaskToken == "" {
		return nil, fmt.Errorf("decisionhead: contract %s declares no mask_token", path)
	}
	if c.HeadFormat == "" {
		return nil, fmt.Errorf("decisionhead: contract %s declares no head_format", path)
	}
	ranges, err := parseCJKRanges(raw)
	if err != nil {
		return nil, fmt.Errorf("decisionhead: contract %s: %w", path, err)
	}
	c.cjkRanges = ranges
	return &c, nil
}

// rawCJKRanges is the on-disk shape of the cjk_ranges field, kept separate from
// Contract so the ranges can be parsed into rune pairs once at load.
type rawCJKRanges struct {
	InstructionLangRule struct {
		CJKRanges []string `json:"cjk_ranges"`
	} `json:"instruction_lang_rule"`
}

func parseCJKRanges(raw []byte) ([]cjkRange, error) {
	var doc rawCJKRanges
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("cjk_ranges: %w", err)
	}
	var out []cjkRange
	for _, spec := range doc.InstructionLangRule.CJKRanges {
		lo, hi, ok := strings.Cut(spec, "-")
		if !ok {
			return nil, fmt.Errorf("cjk range %q is not lo-hi", spec)
		}
		loR, err := parseCodePoint(lo)
		if err != nil {
			return nil, fmt.Errorf("cjk range %q: %w", spec, err)
		}
		hiR, err := parseCodePoint(hi)
		if err != nil {
			return nil, fmt.Errorf("cjk range %q: %w", spec, err)
		}
		out = append(out, cjkRange{lo: loR, hi: hiR})
	}
	return out, nil
}

// parseCodePoint reads one end of a cjk range. The shipped contract writes the
// bounds exactly as its own regex does — "\u3400" — so the JSON value carries a
// literal backslash-u prefix, not the code point itself. A hand-written contract
// may instead use bare hex, which the same call accepts.
func parseCodePoint(spec string) (rune, error) {
	trimmed := strings.TrimSpace(spec)
	for _, prefix := range []string{`\u`, `\U`, "u+", "U+", "0x", "0X"} {
		if len(trimmed) > len(prefix) && strings.EqualFold(trimmed[:len(prefix)], prefix) {
			trimmed = trimmed[len(prefix):]
			break
		}
	}
	value, err := strconv.ParseInt(trimmed, 16, 32)
	if err != nil {
		return 0, err
	}
	return rune(value), nil
}

// RouteLang reproduces the contract's rule: text = the first non-empty field
// (here always the question), cjk_ratio(text) > threshold -> "zh", else "en";
// an empty text falls back. The ratio counts runes, not bytes, and the text is
// measured alone — joining fields with spaces shifts the denominator and flips
// borderline samples (DEPLOY_CONTRACT.md §3.4).
func (c *Contract) RouteLang(question string) string {
	if strings.TrimSpace(question) == "" {
		return c.LangRule.Fallback
	}
	if c.CJKRatio(question) > c.LangRule.CJKThreshold {
		return "zh"
	}
	return "en"
}

// CJKRatio is len(cjk runes) / len(runes) of text.
func (c *Contract) CJKRatio(text string) float64 {
	runes := []rune(text)
	if len(runes) == 0 {
		return 0
	}
	hits := 0
	for _, r := range runes {
		for _, rng := range c.cjkRanges {
			if r >= rng.lo && r <= rng.hi {
				hits++
				break
			}
		}
	}
	return float64(hits) / float64(len(runes))
}

// Template renders the instruction for one decision: it picks one of the
// per-language templates and substitutes the question into its {q} slot.
// Training rotates four templates per language on purpose (so the model cannot
// memorise one phrasing); this rotates deterministically by decision index,
// which keeps a benchmark run reproducible while staying inside the trained
// distribution.
//
// The rendered string is the instruction, not the head: the contract's
// head_format is what wraps it (see renderHead).
func (c *Contract) Template(qtype, lang, question string, index int) (string, error) {
	byLang := c.InstructionTemplates[qtype]
	if byLang == nil {
		return "", fmt.Errorf("decisionhead: contract has no instruction_templates[%q]", qtype)
	}
	return renderTemplate(byLang, qtype, lang, question, index)
}

// relevanceTemplates is the alternative instruction set: it asks whether the
// evidence is RELEVANT to the question instead of whether it suffices to answer
// it.
//
// It is deliberately NOT in laya_config.json. The checkpoint was trained on the
// contract's sufficiency phrasings, so a relevance reading is out of
// distribution for the same head and its probabilities are not calibrated to
// the threshold. It exists because sufficiency is arguably the wrong question
// for a retrieval filter — a multi-hop passage can be a necessary link without
// answering anything on its own, which is exactly the failure measured on the
// gold-answer-bearing chunks — and asking differently is the cheapest way to
// find out whether the head can carry that reading at all.
var relevanceTemplates = map[string][]string{
	"en": {
		"Is the provided evidence relevant to the question: {q}?",
		"Does the evidence have anything to do with the question: {q}?",
		"Is the evidence on-topic for the question: {q}?",
		"Is the evidence related to the question: {q}?",
	},
	"zh": {
		"给定的证据是否与这个问题相关：{q}？",
		"上述证据是否和这个问题有关：{q}？",
		"证据是否切题：{q}？",
		"证据与问题是否相关：{q}？",
	},
}

// RelevanceTemplate renders the relevance phrasing for one decision, rotating
// the four phrasings by decision index exactly as the contract's set does.
func (c *Contract) RelevanceTemplate(lang, question string, index int) (string, error) {
	return renderTemplate(relevanceTemplates, "relevance", lang, question, index)
}

// renderTemplate picks one of the per-language templates and substitutes the
// question into its {q} slot.
func renderTemplate(byLang map[string][]string, label, lang, question string, index int) (string, error) {
	templates := byLang[lang]
	if len(templates) == 0 {
		return "", fmt.Errorf("decisionhead: no instruction templates for %q[%q]", label, lang)
	}
	if index < 0 {
		index = -index
	}
	return strings.ReplaceAll(templates[index%len(templates)], "{q}", question), nil
}

// TemperatureFor is the calibrated temperature for one decision: the
// per-cardinality entry ("{qtype}:{bucket}") when the contract declares it,
// else the global Temperature. The caller divides the logits by it before
// softmax, which is what makes the exposed probability comparable to a
// threshold.
func (c *Contract) TemperatureFor(qtype string, options int) float64 {
	key := fmt.Sprintf("%s:%s", qtype, sizeBucket(options))
	if t, ok := c.TemperatureByOptions[key]; ok && t > 0 {
		return t
	}
	return c.Temperature
}

// sizeBucket maps an option count to the contract's bucket names.
func sizeBucket(count int) string {
	switch {
	case count <= 2:
		return "2"
	case count <= 5:
		return "3-5"
	case count <= 10:
		return "6-10"
	default:
		return "11+"
	}
}

// qtypeID is the contract's integer id for a question type.
func (c *Contract) qtypeID(qtype string) (int64, error) {
	id, ok := c.QTypes[qtype]
	if !ok {
		return 0, fmt.Errorf("decisionhead: contract has no qtypes[%q]", qtype)
	}
	return int64(id), nil
}
