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

// Dispatch tests pin the routing contract:
//
//   - FileTypeOTHER + missing setups → text-page mode.
//   - FileTypeMarkdown → JSON payload family on the matching output
//     key, with the pages slice preserved.
//   - FileTypePDF + setups["pdf"].output_format set to a value not
//     in allowed_output_format["pdf"] → component errors with the
//     format-mismatch message (matches the Python check() behavior).

package component

import (
	"context"
	"strings"
	"testing"

	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/utility"
)

// TestDispatch_OutputFormatValidation_Allowed is the happy-path
// pin: a Markdown file with output_format=json passes the
// allowed_output_format check and runs the structured dispatch.
func TestDispatch_OutputFormatValidation_Allowed(t *testing.T) {
	param := schema.ParserParam{}.Defaults()
	// Defaults already include markdown → {text, json}.
	c := &ParserComponent{Param: param}

	out, err := c.Invoke(context.Background(), map[string]any{
		"binary":    []byte("# Title\n\nbody\n"),
		"doc_id":    "doc.md",
		"file_type": "md",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got, want := out["output_format"], "json"; got != want {
		t.Errorf("output_format = %v, want %v", got, want)
	}
	jsonItems, ok := out["json"].([]map[string]any)
	if !ok {
		t.Fatalf("json payload missing or wrong type: %T", out["json"])
	}
	if len(jsonItems) == 0 {
		t.Errorf("json payload empty; want at least 1 item")
	}
	// Pages must still exist for chunker-side consumers.
	pages, ok := out["pages"].([]schema.Page)
	if !ok || len(pages) == 0 {
		t.Errorf("pages slice missing or empty: %T", out["pages"])
	}
	// File metadata is carried through dispatch.
	if fm, ok := out["file"].(map[string]any); !ok || fm["name"] != "doc.md" {
		t.Errorf("file metadata missing or wrong: %+v", out["file"])
	}
}

// TestDispatch_OutputFormatValidation_Rejection pins the
// whitelist enforcement: a request for output_format=html on the
// markdown family is rejected because markdown's allowed list is
// {text, json}. The component must surface this as a hard error
// before any fallback so a misconfigured template cannot silently
// degrade.
func TestDispatch_OutputFormatValidation_Rejection(t *testing.T) {
	param := schema.ParserParam{}.Defaults()
	// Override the markdown setup to ask for an unsupported format.
	// The key is "markdown" (the python-side family identifier),
	// NOT "md" — utility.FileTypeMarkdown happens to be the string
	// "md" but the setup key is the family name. resolveOutputFormat
	// looks up setups[string(fileType)], so the fileType passed in
	// here must match the setup key.
	param.Setups["markdown"] = schema.ParserSetup{"output_format": "html"}
	// inputs["file_type"] must also be "markdown" so fileTypeFromInputs
	// returns a FileType whose string form matches the setup key.
	c := &ParserComponent{Param: param}

	_, err := c.Invoke(context.Background(), map[string]any{
		"binary":    []byte("# Title\n"),
		"file_type": "md",
	})
	if err == nil {
		t.Fatal("Invoke: want error for unsupported output_format, got nil")
	}
	if !strings.Contains(err.Error(), "output_format") {
		t.Errorf("error %q must mention output_format", err.Error())
	}
	if !strings.Contains(err.Error(), "markdown") && !strings.Contains(err.Error(), "md") {
		t.Errorf("error %q must mention the family", err.Error())
	}
}

// TestDispatch_TextPageMode_NoFileType pins the no-dispatch
// path. When the upstream inputs supply neither file_type nor
// file.name, the component degrades to text-page mode and
// emits output_format=text. This is the documented behavior for
// canvas-bound invocations that wire the binary directly without
// a family hint.
func TestDispatch_TextPageMode_NoFileType(t *testing.T) {
	param := schema.ParserParam{}.Defaults()
	c := &ParserComponent{Param: param}

	out, err := c.Invoke(context.Background(), map[string]any{
		"binary": []byte("plain content\n"),
		"doc_id": "unknown",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got, want := out["output_format"], "text"; got != want {
		t.Errorf("output_format = %v, want %v (text-page mode)", got, want)
	}
	pages, ok := out["pages"].([]schema.Page)
	if !ok || len(pages) == 0 {
		t.Fatalf("pages slice missing or empty: %T", out["pages"])
	}
}

// TestDispatch_SupportedFamilyFailure_HardErrors pins the agreed
// migration rule: once a supported family is identified, parser
// resolution/execution failures must surface as errors instead of
// silently degrading to text-page mode.
func TestDispatch_SupportedFamilyFailure_HardErrors(t *testing.T) {
	param := schema.ParserParam{}.Defaults()
	c := &ParserComponent{Param: param}

	_, err := c.Invoke(context.Background(), map[string]any{
		"binary":    []byte("PDF payload as bytes (not a real PDF — stub test)\n"),
		"file_type": "pdf",
	})
	if err == nil {
		t.Fatal("Invoke: want error for supported family parse failure, got nil")
	}
	if !strings.Contains(err.Error(), "pdf") {
		t.Errorf("error %q must mention pdf", err.Error())
	}
}

// TestFileTypeFromInputs_ResolutionOrder pins the precedence
// rules documented on parser_dispatch.go:fileTypeFromInputs:
//
//  1. inputs["file_type"]  (explicit family hint)
//  2. inputs["file"].name  (filename in the file descriptor)
//  3. inputs["name"]       (last-resort filename)
//  4. FileTypeOTHER        (text-page mode)
func TestFileTypeFromInputs_ResolutionOrder(t *testing.T) {
	cases := []struct {
		name string
		in   map[string]any
		want string
	}{
		{"explicit pdf", map[string]any{"file_type": "pdf"}, "pdf"},
		{"explicit markdown (family form)", map[string]any{"file_type": "markdown"}, "md"},
		{"file.name docx", map[string]any{"file": map[string]any{"name": "report.docx"}}, "docx"},
		{"name fallback md", map[string]any{"name": "notes.md"}, "md"},
		{"unrelated inputs", map[string]any{"binary": []byte("x"), "doc_id": "abc"}, "other"},
		{"unknown family hint", map[string]any{"file_type": "image/xyz"}, "other"},
		{"nil inputs", nil, "other"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := fileTypeFromInputs(tc.in)
			if string(got) != tc.want {
				t.Errorf("fileTypeFromInputs(%v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestResolveOutputFormat_DefaultsAndWhitelist pins the two-layer
// behavior of resolveOutputFormat: it returns the setup's
// output_format when present (or "text" when absent), and
// rejects values not in the allowed_output_format list.
func TestResolveOutputFormat_DefaultsAndWhitelist(t *testing.T) {
	allowed := map[string][]string{
		"pdf":      {"json", "markdown"},
		"markdown": {"text", "json"},
	}
	cases := []struct {
		name    string
		setups  map[string]schema.ParserSetup
		family  string
		want    string
		wantErr bool
	}{
		{
			name:   "no setup → empty (text-page mode)",
			setups: nil,
			family: "pdf",
			want:   "",
		},
		{
			name:   "setup with output_format=json → json",
			setups: map[string]schema.ParserSetup{"pdf": {"output_format": "json"}},
			family: "pdf",
			want:   "json",
		},
		{
			name:   "setup with output_format=markdown → markdown",
			setups: map[string]schema.ParserSetup{"pdf": {"output_format": "markdown"}},
			family: "pdf",
			want:   "markdown",
		},
		{
			name:   "setup without output_format → default text",
			setups: map[string]schema.ParserSetup{"markdown": {}},
			family: "markdown",
			want:   "text",
		},
		{
			name:    "pdf asking for html (not allowed) → reject",
			setups:  map[string]schema.ParserSetup{"pdf": {"output_format": "html"}},
			family:  "pdf",
			wantErr: true,
		},
		{
			name:   "family with no whitelist → accept setup value",
			setups: map[string]schema.ParserSetup{"video": {"output_format": "json"}},
			family: "video",
			want:   "json",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveOutputFormat(tc.family, tc.setups, allowed)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("want error, got nil (value=%q)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestResolveLibType_UsesOwningFamilySetup(t *testing.T) {
	setups := schema.ParserParam{}.Defaults().Setups
	setups["slides"]["lib_type"] = "office_oxide"
	setups["slides"]["parse_method"] = "deepdoc"
	setups["spreadsheet"]["lib_type"] = "office_oxide"
	setups["spreadsheet"]["parse_method"] = "deepdoc"

	cases := []struct {
		name            string
		fileType        utility.FileType
		wantLibType     string
		wantParseMethod string
	}{
		{
			name:            "pptx resolves from slides family",
			fileType:        utility.FileTypePPTX,
			wantLibType:     "office_oxide",
			wantParseMethod: "deepdoc",
		},
		{
			name:            "xlsx resolves from spreadsheet family",
			fileType:        utility.FileTypeXLSX,
			wantLibType:     "office_oxide",
			wantParseMethod: "deepdoc",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotLibType, gotParseMethod := resolveLibType(tc.fileType, setups)
			if gotLibType != tc.wantLibType || gotParseMethod != tc.wantParseMethod {
				t.Fatalf("resolveLibType(%q) = (%q, %q), want (%q, %q)",
					tc.fileType, gotLibType, gotParseMethod, tc.wantLibType, tc.wantParseMethod)
			}
		})
	}
}
