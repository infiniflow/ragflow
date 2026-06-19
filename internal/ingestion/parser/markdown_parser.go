//
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
//

package parser

import (
	"fmt"
	"os"

	"github.com/gomarkdown/markdown/ast"
	"github.com/gomarkdown/markdown/parser"
)

const (
	GoMarkdown = "go_markdown"
)

type MarkdownParser struct {
	libType string
}

func NewMarkdownParser(libType string) (*MarkdownParser, error) {
	switch libType {
	case GoMarkdown:
		return &MarkdownParser{
			libType: GoMarkdown,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported Markdown library type: %s", libType)
	}
}

func (p *MarkdownParser) Parse(filename string, data []byte) error {
	fmt.Printf("Parsing Markdown file: %s\n", filename)
	switch p.libType {
	case GoMarkdown:
		return p.GoMarkdownParse(data)
	default:
		return fmt.Errorf("unsupported Markdown library type: %s", p.libType)
	}
}

func (p *MarkdownParser) GoMarkdownParse(data []byte) error {
	// create Markdown parser with extensions
	extensions := parser.CommonExtensions | parser.AutoHeadingIDs | parser.NoEmptyLineBeforeBlock
	parser.NewWithExtensions(extensions)
	markdownParser := parser.NewWithExtensions(extensions)
	doc := markdownParser.Parse(data)

	fmt.Print("--- AST tree:\n")
	ast.Print(os.Stdout, doc)
	fmt.Print("\n")

	return nil
}

func (p *MarkdownParser) String() string {
	return "MarkdownParser"
}
