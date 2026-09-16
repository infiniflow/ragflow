//
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
//

package connector

import (
	"strings"

	md "github.com/JohannesKaufmann/html-to-markdown"
	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/html"
)

// inlineTagsWithDefaultRule are the tags for which html-to-markdown ships a
// conversion rule of its own (strong/b/em/i/a). When such an element holds real
// content we fall through to that rule; the remaining styled inline tags have no
// rule of their own, so we keep their content like the library's default.
var inlineTagsWithDefaultRule = map[string]bool{
	"a": true, "b": true, "strong": true, "em": true, "i": true,
}

// newMarkdownConverter returns the HTML-to-Markdown converter shared by the
// connectors in this package: html-to-markdown with an asterisk emphasis
// delimiter and the whitespace-preserving plugin below.
func newMarkdownConverter() *md.Converter {
	converter := md.NewConverter("", true, &md.Options{EmDelimiter: "*"})
	converter.Use(preserveInlineWhitespace())
	return converter
}

// preserveInlineWhitespace keeps a whitespace-only styled inline element's text
// when a word directly touches it, so the words on either side don't run
// together. A word processor keeps a differently formatted space as a run of its
// own, so `First<strong> </strong>Last` is what a document with a bolded space
// converts to, and `FirstLast` is what gets chunked and embedded. Elements whose
// neighbours are other elements are left to html-to-markdown's neighbour-aware
// spacing, which already inserts the boundary there.
func preserveInlineWhitespace() md.Plugin {
	return func(converter *md.Converter) []md.Rule {
		return []md.Rule{{
			Filter: []string{"a", "b", "strong", "em", "i", "u", "s", "del", "strike", "sub", "sup", "span"},
			Replacement: func(content string, selec *goquery.Selection, opt *md.Options) *string {
				if strings.TrimSpace(content) != "" {
					if inlineTagsWithDefaultRule[goquery.NodeName(selec)] {
						return nil
					}
					return &content
				}
				if raw := selec.Text(); raw != "" && strings.TrimSpace(raw) == "" && hasAdjacentTextWord(selec) {
					return &raw
				}
				if inlineTagsWithDefaultRule[goquery.NodeName(selec)] {
					return nil
				}
				return &content
			},
		}}
	}
}

// hasAdjacentTextWord reports whether a non-whitespace text node directly
// neighbours the element, i.e. a word that would run into its neighbours if the
// element's own whitespace were dropped.
func hasAdjacentTextWord(selec *goquery.Selection) bool {
	if len(selec.Nodes) == 0 {
		return false
	}
	node := selec.Nodes[0]
	for _, sibling := range []*html.Node{node.PrevSibling, node.NextSibling} {
		if sibling != nil && sibling.Type == html.TextNode && strings.TrimSpace(sibling.Data) != "" {
			return true
		}
	}
	return false
}
