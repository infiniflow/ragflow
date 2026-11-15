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

package tokenizer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// TestPretokenizeMetaspaceUnits pins the pre-tokenizer rules that are not
// obvious from the Metaspace config alone. Every expectation here was verified
// against the reference tokenizer (HF tokenizers) on the mmBERT checkpoint;
// the load-bearing ones are the newline run standing alone WITH the following
// unit re-prefixed, and '\t'/'\r' NOT being boundaries.
func TestPretokenizeMetaspaceUnits(t *testing.T) {
	cases := []struct {
		text string
		want []string
	}{
		{"a b", []string{"▁a", "▁b"}},
		{"a\nb", []string{"▁a", "\n", "▁b"}},
		{"a  b", []string{"▁a", "▁", "▁b"}},
		{" a", []string{"▁a"}},
		{"a ", []string{"▁a", "▁"}},
		{"   leading", []string{"▁", "▁", "▁leading"}},
		{"a\tb", []string{"▁a\tb"}},
		{"a\rb", []string{"▁a\rb"}},
		{"a\n\nb", []string{"▁a", "\n\n", "▁b"}},
		{"a\n", []string{"▁a", "\n"}},
		{"\na", []string{"\n", "▁a"}},
		{"日\n本", []string{"▁日", "\n", "▁本"}},
		{"hello, world!", []string{"▁hello,", "▁world!"}},
		{"", nil},
	}
	for _, c := range cases {
		got := pretokenizeMetaspace(replaceSpaces(c.text))
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("pretokenizeMetaspace(%q) = %q, want %q", c.text, got, c.want)
		}
	}
}

// replaceSpaces applies the Replace normalizer, which is what the pre-tokenizer
// receives in the real pipeline.
func replaceSpaces(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == ' ' {
			out = append(out, "\u2581"...)
			continue
		}
		out = append(out, s[i])
	}
	return string(out)
}

// metaspaceFixture is a minimal tokenizer.json exercising the whole path:
// normalizer -> pre-tokenizer -> BPE merge -> byte fallback. Ids are chosen so
// each unit is one token when merged, and one byte-fallback token for OOV.
const metaspaceFixture = `{
  "model": {
    "type": "BPE",
    "unk_token": "<unk>",
    "byte_fallback": true,
    "vocab": {
      "<unk>": 1, "▁": 100, "a": 101, "b": 102, "\n": 103,
      "▁a": 104, "▁b": 105, "\n\n": 107, "\t": 108, "\r": 109,
      "x": 110, "▁x": 111, "日": 112, "本": 113, "▁日": 114, "▁本": 115,
      "<0x41>": 116
    },
    "merges": ["▁ a", "▁ b", "▁ x", "▁ 日", "▁ 本", "\n \n"]
  }
}`

func loadMetaspaceFixture(t *testing.T) MetaspaceEncoder {
	t.Helper()
	path := filepath.Join(t.TempDir(), "tokenizer.json")
	if err := os.WriteFile(path, []byte(metaspaceFixture), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	enc, err := LoadMetaspaceBPE(path)
	if err != nil {
		t.Fatalf("LoadMetaspaceBPE: %v", err)
	}
	return enc
}

func TestMetaspaceEncodeIDs(t *testing.T) {
	enc := loadMetaspaceFixture(t)
	cases := []struct {
		text string
		want []int32
	}{
		{"a b", []int32{104, 105}},
		{"a\nb", []int32{104, 103, 105}},
		{"a  b", []int32{104, 100, 105}},
		{" a", []int32{104}},
		{"a ", []int32{104, 100}},
		{"a\tb", []int32{104, 108, 102}},
		{"a\rb", []int32{104, 109, 102}},
		{"a\n\nb", []int32{104, 107, 105}},
		{"\na", []int32{103, 104}},
		{"日\n本", []int32{114, 103, 115}},
		{"x", []int32{111}},
		// OOV rune falls back to one <0xXX> token per UTF-8 byte.
		{"A", []int32{100, 116}},
		{"", nil},
	}
	for _, c := range cases {
		got := enc.IDs(c.text)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("EncodeIDs(%q) = %v, want %v", c.text, got, c.want)
		}
		if n := enc.Count(c.text); n != len(c.want) {
			t.Errorf("Count(%q) = %d, want %d", c.text, n, len(c.want))
		}
	}
}

// TestLoadMetaspaceBPERejectsNonBPE keeps the loader honest about the model
// type it claims to serve.
func TestLoadMetaspaceBPERejectsNonBPE(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tokenizer.json")
	doc := map[string]any{"model": map[string]any{"type": "WordPiece", "vocab": map[string]int32{"[PAD]": 0}}}
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := LoadMetaspaceBPE(path); err == nil {
		t.Fatal("LoadMetaspaceBPE accepted a WordPiece model")
	}
}
