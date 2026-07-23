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

// PictureParser validates and stores configuration for image files.
// The actual OCR and VLM description is performed by the
// component-layer maybeDispatchImage, which mirrors Python's
// rag/app/picture.py:chunk() image branch.
//
// Python reference:
//   - Image is opened with PIL, converted to RGB
//   - PaddleOCR tried first (if layout_recognize == "@PaddleOCR")
//   - Falls back to local deepdoc.vision.OCR (ONNX text detection)
//   - If OCR text is short (≤32 chars / ≤32 words for English),
//     calls IMAGE2TEXT VLM describe() for a natural-language description
//   - Returns tokenized text with media context attached

package parser

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

// imageExtensions mirrors Python's visual extensions used in
// utility.FilenameType and the picture.py:chunk() image branch.
// Video extensions (.mp4, .mov, ...) are excluded — those are
// handled by maybeDispatchVideo.
var imageExtensions = map[string]bool{
	"jpg": true, "jpeg": true, "png": true, "gif": true,
	"bmp": true, "tiff": true, "tif": true, "webp": true,
	"svg": true, "ico": true, "avif": true, "heic": true,
	"apng": true, "icon": true, "pcx": true, "tga": true,
	"exif": true, "fpx": true, "psd": true, "cdr": true,
	"pcd": true, "dxf": true, "ufo": true, "eps": true,
	"ai": true, "raw": true, "wmf": true,
}

// PictureParser handles image files for OCR and VLM description.
// Mirrors the configuration from setups["picture"]:
//   - vlm.llm_id  →  VLMModelID (IMAGE2TEXT model for describe)
//   - output_format
//   - image_context_size
//   - layout_recognize  →  ("@PaddleOCR:model_name" or empty)
type PictureParser struct {
	VLMModelID       string // vlm.llm_id — the IMAGE2TEXT model
	OutputFormat     string
	ImageContextSize int    // default 0
	LayoutRecognize  string // layout_recognize (e.g. "@PaddleOCR")
	VideoPrompt      string // for video-in-image detection (Python fallback)
}

// NewPictureParser constructs a PictureParser.
func NewPictureParser() *PictureParser {
	return &PictureParser{}
}

// ConfigureFromSetup reads picture-specific configuration from the
// parser setup map. Extracts vlm.llm_id, output_format,
// image_context_size, layout_recognize, and video_prompt.
func (p *PictureParser) ConfigureFromSetup(setup map[string]any) {
	if p == nil || setup == nil {
		return
	}
	if vlm, ok := setup["vlm"].(map[string]any); ok {
		if llmID, ok := vlm["llm_id"].(string); ok && llmID != "" {
			p.VLMModelID = llmID
		}
	}
	if v, ok := setup["output_format"].(string); ok && v != "" {
		p.OutputFormat = v
	}
	if v, ok := setup["image_context_size"].(float64); ok {
		p.ImageContextSize = int(v)
	}
	if v, ok := setup["layout_recognize"].(string); ok && v != "" {
		p.LayoutRecognize = v
	}
	if v, ok := setup["video_prompt"].(string); ok && v != "" {
		p.VideoPrompt = v
	}
}

// ParseWithResult implements ParseResultProducer. It validates the
// file extension against the image extension whitelist. The actual
// OCR and VLM description happens via maybeDispatchImage at the
// component layer (mirrors Python's picture.py:chunk()).
func (p *PictureParser) ParseWithResult(ctx context.Context, filename string, data []byte) ParseResult {
	ext := strings.ToLower(filepath.Ext(filename))
	if len(ext) > 1 && ext[0] == '.' {
		ext = ext[1:]
	}

	// Video extensions: defer to maybeDispatchVideo. The picture.py
	// Python path handles both, but Go routes video separately.
	if ext != "" && isVideoExtension(ext) {
		return ParseResult{
			Err: fmt.Errorf("picture: video file %q should be routed through video parser, not picture", filename),
		}
	}

	if ext == "" || !imageExtensions[ext] {
		return ParseResult{
			Err: fmt.Errorf("picture: unsupported extension %q (filename: %s); accepted: .jpg/.jpeg/.png/.gif/.bmp/.tiff/.tif/.webp/.svg/.ico/.avif/.heic/...", ext, filename),
		}
	}

	// OutputFormat, VLMModelID, ImageContextSize, and LayoutRecognize
	// are consumed by maybeDispatchImage at the component layer.
	outFmt := p.OutputFormat
	if outFmt == "" {
		outFmt = "text"
	}

	return ParseResult{
		OutputFormat: outFmt,
		File: map[string]any{
			"name":         filename,
			"size":         len(data),
			"doc_type_kwd": "image",
		},
	}
}

// isVideoExtension returns true when the extension is a video format
// that Python's picture.py routes through the video branch.
func isVideoExtension(ext string) bool {
	switch ext {
	case "mp4", "mov", "avi", "flv", "mpeg", "mpg",
		"webm", "wmv", "3gp", "3gpp", "mkv":
		return true
	}
	return false
}
