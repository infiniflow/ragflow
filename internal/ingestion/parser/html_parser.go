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
	"strings"

	"golang.org/x/net/html"
)

const (
	Official string = "official"
)

type HTMLParser struct {
	libType string
}

func NewHTMLParser(libType string) (*HTMLParser, error) {
	switch libType {
	case Official:
		return &HTMLParser{
			libType: Official,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported HTML library type: %s", libType)
	}
}

func (p *HTMLParser) Parse(filename string, data []byte) error {
	fmt.Printf("Parsing HTML file: %s\n", filename)
	switch p.libType {
	case Official:
		return p.OfficialHTMLParse(data)
	default:
		return fmt.Errorf("unsupported HTML library type: %s", p.libType)
	}
}

func (p *HTMLParser) OfficialHTMLParse(data []byte) error {
	doc, _ := html.Parse(strings.NewReader(string(data)))
	p.WalkIterative(doc)
	return nil
}

func (p *HTMLParser) WalkIterative(root *html.Node) {
	if root == nil {
		return
	}

	// Stack: stores node and its depth
	type item struct {
		node  *html.Node
		depth int
	}
	stack := []item{{root, 0}}

	for len(stack) > 0 {
		// Pop the top of the stack
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		indent := strings.Repeat("  ", current.depth)

		// Handle different node types
		switch current.node.Type {
		case html.ElementNode:
			// Print opening tag
			fmt.Printf("%s<%s", indent, current.node.Data)
			// Optionally print attributes
			for _, attr := range current.node.Attr {
				fmt.Printf(" %s=%q", attr.Key, attr.Val)
			}
			fmt.Println(">")

		case html.TextNode:
			// Print text content (trim extra whitespace)
			text := strings.TrimSpace(current.node.Data)
			if text != "" {
				fmt.Printf("%stext: %q\n", indent, text)
			}

		case html.CommentNode:
			fmt.Printf("%scomment: %s\n", indent, current.node.Data)

		case html.DoctypeNode:
			fmt.Printf("%sDOCTYPE: %s\n", indent, current.node.Data)
		}

		// Push children onto stack in reverse order to maintain original sequence
		var children []*html.Node
		for child := current.node.FirstChild; child != nil; child = child.NextSibling {
			children = append([]*html.Node{child}, children...) // Reverse order
		}
		for _, child := range children {
			stack = append(stack, item{child, current.depth + 1})
		}
	}
}

func (p *HTMLParser) String() string {
	return "HTMLParser"
}
