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

package component

import (
	"context"
	_ "embed"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"gorm.io/gorm"

	deepdoctype "ragflow/internal/deepdoc/parser/type"
	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
	"ragflow/internal/utility"
)

//go:embed testdata/picture_text.svg
var pictureSVGFixture []byte

const pictureSVGFixtureText = "HELLO WORLD\nQuarterly revenue\nR&D budget\nNode label\nSecond line"

func TestExtractSVGText(t *testing.T) {
	for _, tc := range []struct {
		name string
		svg  string
		want string
	}{
		{"fixture", string(pictureSVGFixture), pictureSVGFixtureText},
		{"inline tspans stay on one line", `<svg xmlns="http://www.w3.org/2000/svg"><text>Hel<tspan fill="red">lo</tspan> there</text></svg>`, "Hello there"},
		{"tspans that only set x stay on one line", `<svg><text y="10"><tspan x="0">H</tspan><tspan x="8">i</tspan></text></svg>`, "Hi"},
		{"whitespace is collapsed", "<svg><text>\n  a \t b\n</text></svg>", "a b"},
		{"tooltip inside text is ignored", `<svg><text><title>tooltip</title>kept</text></svg>`, "kept"},
		{"prefixed namespace", `<s:svg xmlns:s="http://www.w3.org/2000/svg"><s:text>kept</s:text></s:svg>`, "kept"},
		{"doctype entities", `<!DOCTYPE svg PUBLIC "-//W3C//DTD SVG 1.1//EN" "http://www.w3.org/Graphics/SVG/1.1/DTD/svg11.dtd" [<!ENTITY ns_svg "http://www.w3.org/2000/svg"><!ENTITY label 'kept'>]><svg xmlns="&ns_svg;"><text>&label;</text></svg>`, "kept"},
		{"utf-8 byte order mark", "\xef\xbb\xbf<?xml version=\"1.0\" encoding=\"UTF-8\"?><svg><text>kept</text></svg>", "kept"},
		{"cdata", `<svg><text><![CDATA[1 < 2]]></text></svg>`, "1 < 2"},
		{"text outside text elements is ignored", `<svg><title>t</title><desc>d</desc><g>stray</g><text>kept</text></svg>`, "kept"},
		{"style and script are ignored", `<svg><foreignObject><div xmlns="http://www.w3.org/1999/xhtml"><style>p{}</style><script>x()</script>kept</div></foreignObject></svg>`, "kept"},
		{"line break element", `<svg><foreignObject><div xmlns="http://www.w3.org/1999/xhtml">a<br/>b</div></foreignObject></svg>`, "a\nb"},
		{"non-utf8 encoding", "<?xml version=\"1.0\" encoding=\"ISO-8859-1\"?><svg><text>caf\xe9</text></svg>", "café"},
		{"no text", `<svg xmlns="http://www.w3.org/2000/svg"><rect width="1" height="1"/></svg>`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := extractSVGText(t.Context(), []byte(tc.svg))
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("text = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestExtractSVGTextRejectsInvalidInput(t *testing.T) {
	for _, tc := range []struct {
		name string
		svg  string
		want string
	}{
		{"not xml", "\x89PNG\r\n\x1a\n", "decode SVG"},
		{"empty document", "<?xml version=\"1.0\"?>", "no root element"},
		{"unclosed element", `<svg><text>a</text>`, "decode SVG"},
		{"mismatched tags", `<svg><text>a</g></svg>`, "decode SVG"},
		{"undefined entity", `<svg><text>&undefined;</text></svg>`, "decode SVG"},
		{"other root", `<html><body>a</body></html>`, `root element is "html"`},
		{"excessive nesting", "<svg>" + strings.Repeat("<g>", maxSVGDepth), "nest deeper"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := extractSVGText(t.Context(), []byte(tc.svg))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("text = %q, error = %v, want error containing %q", got, err, tc.want)
			}
		})
	}
}

func TestExtractSVGTextHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := extractSVGText(ctx, pictureSVGFixture); err == nil {
		t.Fatal("expected a cancellation error")
	}
}

// TestMaybeDispatchImageSVG runs the picture template on an SVG. The text
// comes from the document itself, so it needs neither the local OCR analyzer
// nor a vision model, and the raw SVG is never sent to a vision provider.
func TestMaybeDispatchImageSVG(t *testing.T) {
	templateData, err := os.ReadFile("../pipeline/template/ingestion_pipeline_picture.json")
	if err != nil {
		t.Fatal(err)
	}
	var template struct {
		DSL struct {
			Components map[string]struct {
				Obj struct{ Params map[string]any }
			}
		}
	}
	if err := json.Unmarshal(templateData, &template); err != nil {
		t.Fatal(err)
	}
	params := template.DSL.Components["Parser:ViewsCaptureLight"].Obj.Params
	original := deepdoctype.NativeDocAnalyzerFactory
	deepdoctype.NativeDocAnalyzerFactory = func() (deepdoctype.DocAnalyzer, bool) {
		t.Error("SVG must not reach the raster OCR analyzer")
		return nil, false
	}
	t.Cleanup(func() { deepdoctype.NativeDocAnalyzerFactory = original })
	for _, tc := range []struct {
		name    string
		method  string
		enabled bool
	}{
		{"vision disabled", "ocr", false},
		{"vision enabled", "ocr", true},
		{"vlm method", "custom-vlm", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			driver := &imagePromptCaptureDriver{}
			originalTenant, originalConfig := resolveTenantModelByType, resolveModelConfig
			resolveTenantModelByType = func(context.Context, *gorm.DB, string, entity.ModelType) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
				return driver, "vision", &modelModule.APIConfig{}, 0, nil
			}
			resolveModelConfig = func(context.Context, *gorm.DB, string, entity.ModelType, string) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
				return driver, "vision", &modelModule.APIConfig{}, 0, nil
			}
			t.Cleanup(func() { resolveTenantModelByType, resolveModelConfig = originalTenant, originalConfig })
			params["image"].(map[string]any)["parse_method"] = tc.method
			component, err := NewParserComponent(params)
			if err != nil {
				t.Fatal(err)
			}
			pc := component.(*ParserComponent)
			pc.enableVisionEnhancement = tc.enabled
			result, err := pc.Invoke(t.Context(), nil, map[string]any{"name": "diagram.svg", "file_type": "image", "binary": pictureSVGFixture, "tenant_id": "t1"})
			if err != nil {
				t.Fatal(err)
			}
			items, _ := result["json"].([]map[string]any)
			if len(items) != 1 || items[0]["text"] != pictureSVGFixtureText {
				t.Fatalf("items = %+v, want text %q", items, pictureSVGFixtureText)
			}
			payload, _ := items[0]["image"].(string)
			if !strings.HasPrefix(payload, "data:image/svg+xml;base64,") || items[0]["doc_type_kwd"] != "image" {
				t.Fatalf("missing SVG image attachment: %+v", items)
			}
			if len(driver.captured) != 0 {
				t.Fatalf("raw SVG was sent to the vision model: %#v", driver.captured)
			}
		})
	}
}

func TestMaybeDispatchImageSVGWarnings(t *testing.T) {
	driver := &imagePromptCaptureDriver{}
	original := resolveTenantModelByType
	resolveTenantModelByType = func(context.Context, *gorm.DB, string, entity.ModelType) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
		return driver, "vision", &modelModule.APIConfig{}, 0, nil
	}
	t.Cleanup(func() { resolveTenantModelByType = original })
	textless := []byte(`<svg xmlns="http://www.w3.org/2000/svg"><rect width="1" height="1"/></svg>`)
	result, handled, err := maybeDispatchImage(t.Context(), nil, utility.FileTypeVISUAL, "shape.SVG", textless, map[string]any{"tenant_id": "t1"}, defaultSetups(), true)
	if err != nil {
		t.Fatal(err)
	}
	if !handled || len(result.JSON) != 1 || result.JSON[0]["text"] != "" || result.JSON[0]["image"] == "" {
		t.Fatalf("result = %+v, handled = %v", result, handled)
	}
	if len(driver.captured) != 0 {
		t.Fatalf("raw SVG was sent to the vision model: %#v", driver.captured)
	}
	warnings := strings.Join(result.Warnings, "\n")
	for _, want := range []string{"image SVG contains no text", "image VLM enhancement skipped: SVG is not rasterized"} {
		if !strings.Contains(warnings, want) {
			t.Fatalf("warnings = %q, want %q", warnings, want)
		}
	}
}

func TestMaybeDispatchImageRejectsInvalidSVG(t *testing.T) {
	for _, method := range []string{"ocr", "custom-vlm"} {
		setups := defaultSetups()
		setups["image"]["parse_method"] = method
		_, handled, err := maybeDispatchImage(t.Context(), nil, utility.FileTypeVISUAL, "broken.svg", []byte(`<svg><text>a</svg>`), nil, setups, false)
		if !handled || err == nil || !strings.Contains(err.Error(), "parser: decode SVG") {
			t.Fatalf("method %q: handled = %v, error = %v, want SVG decode error", method, handled, err)
		}
		_, _, err = maybeDispatchImage(t.Context(), nil, utility.FileTypeVISUAL, "empty.svg", nil, nil, setups, false)
		if err == nil || !strings.Contains(err.Error(), "size limits") {
			t.Fatalf("method %q: error = %v, want size limit error", method, err)
		}
	}
}
