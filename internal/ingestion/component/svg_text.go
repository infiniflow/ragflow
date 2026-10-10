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

// SVG is a vector format: the registered raster decoders cannot read it, and
// its text is already present in the document. Image dispatch therefore reads
// SVG text from the XML instead of rasterizing it for OCR.

package component

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	htmlcharset "golang.org/x/net/html/charset"

	"ragflow/internal/parser/parser"
)

// maxSVGDepth bounds element nesting: the XML decoder keeps one stack entry
// per open element, so depth is the allocation an SVG payload could inflate.
const maxSVGDepth = 1024

// maxSVGEntities bounds the entity declarations read from a DOCTYPE.
const maxSVGEntities = 64

// svgEntityPattern matches the internal entity declarations of a DOCTYPE,
// which some editors use for namespace URIs.
var svgEntityPattern = regexp.MustCompile(`<!ENTITY\s+([A-Za-z_][\w.-]*)\s+(?:"([^"]*)"|'([^']*)')\s*>`)

func isSVGFilename(filename string) bool {
	return strings.EqualFold(filepath.Ext(filename), ".svg")
}

// extractSVGText returns the rendered text of an SVG document: the character
// data of <text> elements and of the HTML embedded in <foreignObject>, one
// line per text element, repositioned <tspan>, or HTML block.
func extractSVGText(ctx context.Context, data []byte) (string, error) {
	decoder := xml.NewDecoder(bytes.NewReader(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))))
	// transcoded records that the decoder no longer reads data byte for byte,
	// so entity references cannot be counted in it.
	var transcoded bool
	decoder.CharsetReader = func(label string, input io.Reader) (io.Reader, error) {
		transcoded = true
		return htmlcharset.NewReaderLabel(label, input)
	}
	var expansion int64
	entities := make(map[string]string, len(xml.HTMLEntity))
	for name, value := range xml.HTMLEntity {
		entities[name] = value
	}
	decoder.Entity = entities

	var lines []string
	var line strings.Builder
	var pendingSpace bool
	flush := func() {
		if line.Len() > 0 {
			lines = append(lines, line.String())
		}
		line.Reset()
		pendingSpace = false
	}
	// depth counts open elements; textDepth and skipDepth record the depth at
	// which a text container or a non-rendered subtree was opened.
	var depth, textDepth, skipDepth int
	var sawRoot bool
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", fmt.Errorf("parser: decode SVG: %w", err)
		}
		switch el := token.(type) {
		case xml.Directive:
			if transcoded {
				continue
			}
			matches := svgEntityPattern.FindAllSubmatch(el, maxSVGEntities+1)
			if len(matches) > maxSVGEntities {
				return "", fmt.Errorf("parser: decode SVG: DOCTYPE declares more than %d entities", maxSVGEntities)
			}
			// Entity values are substituted as literal text and never parsed
			// again, so a declaration cannot expand recursively. Repeated
			// references still multiply a value, so the total they expand to
			// is budgeted before the decoder substitutes any of them.
			for _, match := range matches {
				value := string(match[2]) + string(match[3])
				references := bytes.Count(data, []byte("&"+string(match[1])+";"))
				expansion += int64(references) * int64(len(value))
				if expansion > parser.MaxImagePayloadBytes {
					return "", errors.New("parser: decode SVG: entity expansion exceeds size limits")
				}
				entities[string(match[1])] = value
			}
		case xml.StartElement:
			depth++
			if depth > maxSVGDepth {
				return "", fmt.Errorf("parser: decode SVG: elements nest deeper than %d levels", maxSVGDepth)
			}
			name := el.Name.Local
			if depth == 1 {
				if name != "svg" {
					return "", fmt.Errorf("parser: decode SVG: root element is %q, not svg", name)
				}
				sawRoot = true
			}
			switch {
			case skipDepth > 0:
			case svgNonRendered[name]:
				skipDepth = depth
			case textDepth == 0:
				if name == "text" || name == "foreignObject" {
					textDepth = depth
				}
			case startsSVGTextLine(name, el.Attr):
				flush()
			}
		case xml.EndElement:
			if textDepth > 0 && skipDepth == 0 && (depth == textDepth || svgTextBlocks[el.Name.Local]) {
				flush()
			}
			if depth == textDepth {
				textDepth = 0
			}
			if depth == skipDepth {
				skipDepth = 0
			}
			depth--
		case xml.CharData:
			if textDepth == 0 || skipDepth > 0 {
				continue
			}
			// Collapse whitespace runs while copying, as SVG rendering does.
			for len(el) > 0 {
				r, size := utf8.DecodeRune(el)
				el = el[size:]
				if unicode.IsSpace(r) {
					pendingSpace = true
					continue
				}
				if pendingSpace && line.Len() > 0 {
					line.WriteByte(' ')
				}
				pendingSpace = false
				line.WriteRune(r)
			}
		}
	}
	if !sawRoot {
		return "", errors.New("parser: decode SVG: no root element")
	}
	return strings.Join(lines, "\n"), nil
}

// svgNonRendered lists the elements whose character data is never drawn.
var svgNonRendered = map[string]bool{
	"style": true, "script": true, "title": true, "desc": true, "metadata": true,
}

// svgTextBlocks lists the HTML elements inside <foreignObject> that break the
// line they appear in.
var svgTextBlocks = map[string]bool{
	"br": true, "div": true, "p": true, "li": true, "tr": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
}

// startsSVGTextLine reports whether an element nested in a text container
// begins a new line: an HTML block, or a <tspan> that moves the baseline.
// A <tspan> that only sets x stays inline, since exporters position single
// glyphs or words that way.
func startsSVGTextLine(name string, attrs []xml.Attr) bool {
	if svgTextBlocks[name] {
		return true
	}
	if name != "tspan" {
		return false
	}
	for _, attr := range attrs {
		if attr.Name.Local == "y" || attr.Name.Local == "dy" {
			return true
		}
	}
	return false
}
