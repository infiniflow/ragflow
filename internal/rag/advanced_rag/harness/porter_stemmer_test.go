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

package harness

import "testing"

// TestPorterStemMatchesNLTK pins porterStem against nltk.stem.porter's
// NLTK_EXTENSIONS output (PorterStemmer().stem), collected with
//
//	from nltk.stem import PorterStemmer
//	print(PorterStemmer().stem(word))
//
// in production's environment. Every pair below is ground truth, not
// aspiration: the keyword-narrowing stem match (keywordForms/_sentence_stems)
// must produce Python-identical stems or the narrow keeps different sentences
// than Python does.
func TestPorterStemMatchesNLTK(t *testing.T) {
	cases := []struct{ word, want string }{
		// Step-1a/1b regulars.
		{"nominations", "nomin"}, {"nominated", "nomin"}, {"companies", "compani"},
		{"running", "run"}, {"caresses", "caress"}, {"ponies", "poni"},
		{"caress", "caress"}, {"cats", "cat"}, {"meetings", "meet"}, {"stems", "stem"},
		{"cries", "cri"}, {"flies", "fli"},
		// Irregular pool.
		{"sky", "sky"}, {"skies", "sky"}, {"dying", "die"}, {"lying", "lie"},
		{"tying", "tie"}, {"news", "news"}, {"innings", "inning"},
		{"outings", "outing"}, {"cannings", "canning"}, {"howe", "howe"},
		{"proceed", "proceed"}, {"exceed", "exceed"}, {"succeed", "succeed"},
		// 4-letter ies/ied reductions.
		{"ties", "tie"}, {"dies", "die"}, {"died", "die"}, {"spied", "spi"},
		// eed / *d / *o step-1b branches.
		{"agreed", "agre"}, {"feed", "feed"}, {"plastered", "plaster"},
		{"bled", "bled"}, {"motoring", "motor"}, {"sing", "sing"},
		{"conflated", "conflat"}, {"troubled", "troubl"}, {"sized", "size"},
		{"hopping", "hop"}, {"falling", "fall"}, {"filing", "file"},
		// Step-1c y-condition (consonant before the y; len(stem) > 1).
		{"happy", "happi"}, {"enjoy", "enjoy"}, {"spy", "spi"}, {"try", "tri"},
		// Step-2/3/4 table from the paper, NLTK mode.
		{"relation", "relat"}, {"conditional", "condit"}, {"rational", "ration"},
		{"valenci", "valenc"}, {"hesitanci", "hesit"}, {"digitizer", "digit"},
		{"conformabli", "conform"}, {"radicalli", "radic"},
		{"differentli", "differ"}, {"vileli", "vile"},
		{"analogousli", "analog"}, {"vietnamization", "vietnam"},
		{"predication", "predic"}, {"operator", "oper"},
		{"feudalism", "feudal"}, {"decisiveness", "decis"},
		{"hopefulness", "hope"}, {"callousness", "callous"},
		{"formaliti", "formal"}, {"sensitiviti", "sensit"},
		{"sensibiliti", "sensibl"}, {"triplicate", "triplic"},
		{"formative", "form"}, {"formalize", "formal"},
		{"electriciti", "electr"}, {"electrical", "electr"},
		{"hopeful", "hope"}, {"goodness", "good"},
		{"revival", "reviv"}, {"allowance", "allow"}, {"inference", "infer"},
		{"airliner", "airlin"}, {"gyroscopic", "gyroscop"},
		{"adjustable", "adjust"}, {"defensible", "defens"},
		{"irritant", "irrit"}, {"replacement", "replac"},
		{"adjustment", "adjust"}, {"dependent", "depend"},
		{"adoption", "adopt"}, {"homologou", "homolog"},
		{"communism", "commun"}, {"activate", "activ"},
		{"angulariti", "angular"}, {"homologous", "homolog"},
		{"effective", "effect"}, {"bowdlerize", "bowdler"},
		// Step-5a/5b.
		{"probate", "probat"}, {"rate", "rate"}, {"cease", "ceas"},
		{"controll", "control"}, {"roll", "roll"},
	}
	for _, c := range cases {
		if got := porterStem(c.word); got != c.want {
			t.Errorf("porterStem(%q) = %q, want %q", c.word, got, c.want)
		}
	}
}

// TestStemUsesPorter pins the seam: the package's stem() (the one
// keywordForms/_sentence_stems call) is the Porter stemmer, matching Python
// text_processing._stem's nltk branch.
func TestStemUsesPorter(t *testing.T) {
	if stem("nominations") != "nomin" {
		t.Errorf("stem(nominations) = %q, want the Porter stem \"nomin\"", stem("nominations"))
	}
	if stem("flies") != "fli" {
		t.Errorf("stem(flies) = %q, want \"fli\"", stem("flies"))
	}
}
