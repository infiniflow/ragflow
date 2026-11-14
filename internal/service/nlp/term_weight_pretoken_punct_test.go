// Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
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

package nlp

import (
	"reflect"
	"testing"
)

// The punctuation class must terminate and compile. An escaped closing
// bracket left the class unterminated, rePretokenPunct stayed nil, and the
// punctuation rule silently never fired.
func TestPretokenPunctPatternCompiles(t *testing.T) {
	if pretokenPunctErr != nil {
		t.Fatalf("pretokenPunctPattern does not compile: %v", pretokenPunctErr)
	}
	if rePretokenPunct == nil {
		t.Fatal("rePretokenPunct is nil")
	}
}

// Tokens whose first character is punctuation are replaced with "#" and
// dropped, mirroring the Python re.match on the same class.
func TestPretokenDropsTokensStartingWithPunctuation(t *testing.T) {
	d := NewTermWeightDealer("")
	for _, in := range []string{"？请问", "#hashtag", "。。。"} {
		if got := d.Pretoken(in, true, true, ""); len(got) != 0 {
			t.Errorf("Pretoken(%q) = %q; want no tokens", in, got)
		}
	}
}

// Anchored like Python's re.match: punctuation inside a token does not drop
// it. This is what separates this fix from merely unescaping the closing
// bracket, which would also drop "3.5", "don't" or "Hello,".
func TestPretokenKeepsTokensContainingPunctuation(t *testing.T) {
	d := NewTermWeightDealer("")
	cases := []struct {
		in   string
		want []string
	}{
		{"3.5", []string{"3.5"}},
		{"don't", []string{"don't"}},
		{"e-mail", []string{"e-mail"}},
		{"Hello, world.", []string{"Hello,", "world."}},
		{"你好。", []string{"你好。"}},
		{"a，b", []string{"a，b"}},
	}
	for _, c := range cases {
		if got := d.Pretoken(c.in, true, true, ""); !reflect.DeepEqual(got, c.want) {
			t.Errorf("Pretoken(%q) = %q; want %q", c.in, got, c.want)
		}
	}
}
