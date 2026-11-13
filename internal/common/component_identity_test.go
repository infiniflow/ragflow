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

package common

import (
	"reflect"
	"testing"
)

func TestIsExtractorComponentID(t *testing.T) {
	tests := []struct {
		id   string
		want bool
	}{
		{id: "Extractor", want: false},
		{id: "extractor", want: false},
		{id: "Extractor:AutoExtractDefault", want: true},
		{id: "extractor:node", want: false},
		{id: "EXTRACTOR:Node", want: false},
		{id: "Extractor:", want: false},
		{id: "extractor_0", want: false},
		{id: "Extractor_0", want: false},
		{id: "extractors", want: false},
		{id: "my_extractor", want: false},
		{id: "Parser:Extractor", want: false},
		{id: "", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			if got := IsExtractorComponentID(tt.id); got != tt.want {
				t.Fatalf("IsExtractorComponentID(%q) = %v, want %v", tt.id, got, tt.want)
			}
		})
	}
}

func TestIsParserComponentID(t *testing.T) {
	tests := []struct {
		id   string
		want bool
	}{
		{id: "Parser:HipSignsRhyme", want: true},
		{id: "Parser:", want: false},
		{id: "parser:node", want: false},
		{id: "PARSER:Node", want: false},
		{id: "Parser", want: false},
		{id: "parser_0", want: false},
		{id: "Extractor:Parser", want: false},
		{id: "", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			if got := IsParserComponentID(tt.id); got != tt.want {
				t.Fatalf("IsParserComponentID(%q) = %v, want %v", tt.id, got, tt.want)
			}
		})
	}
}

func TestIsCompilerComponentID(t *testing.T) {
	tests := []struct {
		id   string
		want bool
	}{
		{id: "Compiler:CurvySunsLove", want: true},
		{id: "Compiler:", want: false},
		{id: "compiler:node", want: false},
		{id: "COMPILER:Node", want: false},
		{id: "compiler_0", want: false},
		{id: "Compiler", want: false},
		{id: "compilers", want: false},
		{id: "Parser:Compiler", want: false},
		{id: "", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			if got := IsCompilerComponentID(tt.id); got != tt.want {
				t.Fatalf("IsCompilerComponentID(%q) = %v, want %v", tt.id, got, tt.want)
			}
		})
	}
}

func TestIsComponentKey(t *testing.T) {
	tests := []struct {
		id   string
		want bool
	}{
		{id: "Extractor:AutoExtractDefault", want: true},
		{id: "Parser:HipSignsRhyme", want: true},
		{id: "Parser:", want: false},
		{id: "GeneralChunker:SixApplesFall", want: true},
		{id: "File:node", want: true},
		{id: "Extractor:", want: false},
		{id: ":foo", want: false},
		{id: "foo:bar", want: false},
		{id: "extractor:0", want: false},
		{id: "extractor_0", want: false},
		{id: "compiler_0", want: false},
		{id: "Extractor", want: false},
		{id: "chunk_token_num", want: false},
		{id: "parser_0", want: false},
		{id: "", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			if got := IsComponentKey(tt.id); got != tt.want {
				t.Fatalf("IsComponentKey(%q) = %v, want %v", tt.id, got, tt.want)
			}
		})
	}
}

func TestIsComponentName(t *testing.T) {
	names := []string{
		ComponentNameFile,
		ComponentNameParser,
		ComponentNameExtractor,
		ComponentNameCompiler,
		ComponentNameTokenizer,
		ComponentNameGeneralChunker,
		ComponentNameTokenChunker,
		ComponentNameTitleChunker,
		ComponentNameManualChunker,
		ComponentNameOneChunker,
		ComponentNamePageChunker,
		ComponentNameQAChunker,
		ComponentNameTableChunker,
		ComponentNameGroupTitleChunker,
		ComponentNameHierarchyTitleChunker,
	}
	for _, name := range names {
		if !IsComponentName(name) {
			t.Errorf("IsComponentName(%q) = false, want true", name)
		}
	}
	for _, name := range []string{"", "Chunker", "extractor", "Extractor "} {
		if IsComponentName(name) {
			t.Errorf("IsComponentName(%q) = true, want false", name)
		}
	}
}

func TestSortedExtractorKeys(t *testing.T) {
	parserConfig := map[string]any{
		"Extractor:B": nil,
		"Extractor:A": nil,
		"Extractor:":  nil,
		"Extractor":   nil,
		"extractor_0": nil,
		"Parser:X":    nil,
	}
	want := []string{"Extractor:A", "Extractor:B"}
	if got := SortedExtractorKeys(parserConfig); !reflect.DeepEqual(got, want) {
		t.Fatalf("SortedExtractorKeys = %#v, want %#v", got, want)
	}
}

func TestIsExtractorComponent(t *testing.T) {
	tests := []struct {
		id   string
		name string
		want bool
	}{
		{id: "node-1", name: "Extractor", want: true},
		{id: "Extractor:node-1", name: "", want: true},
		{id: "extractor_0", name: "", want: false},
		{id: "node-1", name: "Parser", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name+"/"+tt.id, func(t *testing.T) {
			if got := IsExtractorComponent(tt.id, tt.name); got != tt.want {
				t.Fatalf("IsExtractorComponent(%q, %q) = %v, want %v", tt.id, tt.name, got, tt.want)
			}
		})
	}
}

func TestIsParserComponent(t *testing.T) {
	tests := []struct {
		id   string
		name string
		want bool
	}{
		{id: "node-1", name: "Parser", want: true},
		{id: "Parser:node-1", name: "", want: true},
		{id: "node-1", name: "Extractor", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name+"/"+tt.id, func(t *testing.T) {
			if got := IsParserComponent(tt.id, tt.name); got != tt.want {
				t.Fatalf("IsParserComponent(%q, %q) = %v, want %v", tt.id, tt.name, got, tt.want)
			}
		})
	}
}
