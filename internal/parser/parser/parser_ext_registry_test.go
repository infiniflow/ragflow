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

package parser

import (
	"context"
	"testing"

	"ragflow/internal/utility"
)

type fakeExtParser struct{ textOnly bool }

func (f fakeExtParser) ParseWithResult(_ context.Context, _ string, _ []byte) ParseResult {
	return ParseResult{}
}

// resetParserFactories isolates the registry for a test, preserving any
// edition registrations installed by init() and restoring them afterward.
func resetParserFactories() func() {
	saved := parserFactories
	parserFactories = map[utility.FileType]func(textOnly bool) (ParseResultProducer, error){}
	return func() { parserFactories = saved }
}

// EE editions register a parser factory for a FileType the community switch
// does not handle. GetParser must return that parser.
func TestGetParserReturnsRegisteredFactory(t *testing.T) {
	defer resetParserFactories()()
	const cad = utility.FileType("cad")
	RegisterParserFactory(cad, func(textOnly bool) (ParseResultProducer, error) {
		return fakeExtParser{textOnly: textOnly}, nil
	})
	p, eff, err := GetParser(cad, nil)
	if err != nil {
		t.Fatalf("GetParser(%q) err = %v, want nil", cad, err)
	}
	if eff != cad {
		t.Fatalf("effective file type = %q, want %q", eff, cad)
	}
	if _, ok := p.(fakeExtParser); !ok {
		t.Fatalf("GetParser returned %T, want fakeExtParser", p)
	}
}

// An unregistered community-unknown type still errors as unsupported.
func TestGetParserUnregisteredTypeUnsupported(t *testing.T) {
	defer resetParserFactories()()
	_, _, err := GetParser(utility.FileType("dwg"), nil)
	if err == nil {
		t.Fatal("GetParser(unregistered dwg) err = nil, want unsupported error")
	}
}

// Registering an extra factory must not disturb the builtin switch.
func TestGetParserDoesNotAffectBuiltin(t *testing.T) {
	defer resetParserFactories()()
	p, eff, err := GetParser(utility.FileTypePDF, nil)
	if err != nil {
		t.Fatalf("GetParser(pdf) err = %v", err)
	}
	if eff != utility.FileTypePDF {
		t.Fatalf("effective = %q, want pdf", eff)
	}
	if _, ok := p.(*PDFParser); !ok {
		t.Fatalf("GetParser(pdf) returned %T, want *PDFParser (builtin unaffected)", p)
	}
}

// The textOnly flag must reach the registered factory.
func TestGetTextOnlyParserPropagatesTextOnly(t *testing.T) {
	defer resetParserFactories()()
	var seenTextOnly bool
	RegisterParserFactory(utility.FileType("cad"), func(textOnly bool) (ParseResultProducer, error) {
		seenTextOnly = textOnly
		return fakeExtParser{textOnly: textOnly}, nil
	})
	if _, _, err := GetTextOnlyParser(utility.FileType("cad"), nil); err != nil {
		t.Fatalf("GetTextOnlyParser err = %v", err)
	}
	if !seenTextOnly {
		t.Fatal("textOnly flag not propagated to registered factory")
	}
}
