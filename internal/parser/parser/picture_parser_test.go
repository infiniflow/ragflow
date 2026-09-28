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

package parser

import (
	"strings"
	"testing"
)

func TestNewPictureParser(t *testing.T) {
	p := NewPictureParser()
	if p == nil {
		t.Fatal("NewPictureParser returned nil")
	}
	if p.OutputFormat != "" {
		t.Errorf("OutputFormat = %q, want empty", p.OutputFormat)
	}
}

func TestPictureParser_ConfigureFromSetup(t *testing.T) {
	p := NewPictureParser()
	p.ConfigureFromSetup(map[string]any{
		"output_format": "text",
	})
	if p.OutputFormat != "text" {
		t.Errorf("OutputFormat = %q, want text", p.OutputFormat)
	}
}

func TestPictureParser_ParseWithResult_NilSetup(t *testing.T) {
	ctx := t.Context()
	p := NewPictureParser()
	p.ConfigureFromSetup(nil)
	res := p.ParseWithResult(ctx, "photo.png", []byte("\x89PNG"))
	if res.Err != nil {
		t.Errorf("unexpected error for nil setup: %v", res.Err)
	}
	if res.OutputFormat != "json" {
		t.Errorf("OutputFormat = %q, want json (image only allows json)", res.OutputFormat)
	}
	file, ok := res.File["doc_type_kwd"].(string)
	if !ok || file != "image" {
		t.Errorf("doc_type_kwd = %q, want image", file)
	}
}

func TestPictureParser_ParseWithResult_ValidExtensions(t *testing.T) {
	ctx := t.Context()
	exts := []string{"png", "jpg", "jpeg", "gif", "bmp", "tiff", "tif", "webp", "svg", "ico", "avif", "heic", "apng"}
	p := NewPictureParser()
	for _, ext := range exts {
		fn := "img." + ext
		res := p.ParseWithResult(ctx, fn, []byte{1, 2, 3})
		if res.Err != nil {
			t.Errorf("unexpected error for .%s: %v", ext, res.Err)
		}
		if res.OutputFormat != "json" {
			t.Errorf("OutputFormat = %q for .%s, want json (image only allows json)", res.OutputFormat, ext)
		}
	}
}

func TestPictureParser_ParseWithResult_InvalidExtension(t *testing.T) {
	ctx := t.Context()
	p := NewPictureParser()
	res := p.ParseWithResult(ctx, "sound.mp3", []byte{1})
	if res.Err == nil {
		t.Error("expected error for .mp3, got nil")
	}
	if !strings.Contains(res.Err.Error(), "mp3") {
		t.Errorf("error should mention unsupported extension, got: %v", res.Err)
	}
}

func TestPictureParser_ParseWithResult_VideoExtension(t *testing.T) {
	ctx := t.Context()
	p := NewPictureParser()
	res := p.ParseWithResult(ctx, "video.mp4", []byte{1})
	if res.Err == nil {
		t.Error("expected error for video .mp4, got nil")
	}
	if !strings.Contains(strings.ToLower(res.Err.Error()), "video") {
		t.Errorf("error should mention video, got: %v", res.Err)
	}
}

func TestPictureParser_ParseWithResult_OutputFormat(t *testing.T) {
	ctx := t.Context()
	p := NewPictureParser()
	p.ConfigureFromSetup(map[string]any{
		"output_format": "json",
	})
	res := p.ParseWithResult(ctx, "photo.jpg", []byte{1, 2, 3})
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if res.OutputFormat != "json" {
		t.Errorf("OutputFormat = %q, want json", res.OutputFormat)
	}
}

func TestImageExtensions(t *testing.T) {
	for ext := range imageExtensions {
		if ext == "" {
			t.Error("empty extension in imageExtensions set")
		}
		// All extensions should be lowercase (no uppercase keys).
		if strings.ToLower(ext) != ext {
			t.Errorf("non-lowercase extension key: %q", ext)
		}
	}
}
