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
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"io"
	"strings"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
	_ "image/gif"
	_ "image/jpeg"

	parserlib "ragflow/internal/parser/parser"
)

const (
	maxVisionImageBytes  = parserlib.MaxImagePayloadBytes
	maxVisionImagePixels = parserlib.MaxImagePixels
	maxVisionImageEdge   = parserlib.MaxImageEdge
	maxVLMEncodedBytes   = (maxVisionImageBytes+2)/3*4 + 256
)

func materializeInlineVisionImage(raw string) (*visionImage, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	materialized := &visionImage{VLMData: raw}
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		materialized.VLMDataValidated = isUsableVisionImage(raw)
		return materialized, nil
	}

	isDataURI := strings.HasPrefix(strings.ToLower(raw), "data:image/")
	payload := raw
	if isDataURI {
		comma := strings.IndexByte(raw, ',')
		if comma <= 0 || !strings.HasSuffix(strings.ToLower(raw[:comma]), ";base64") {
			return materialized, nil
		}
		payload = raw[comma+1:]
	} else if strings.HasPrefix(strings.ToLower(raw), "data:") {
		return materialized, nil
	}
	format, err := inspectVisionImagePayload(payload)
	if err != nil {
		materialized.VLMDataValidated = false
		return materialized, nil
	}
	materialized.VLMDataValidated = true
	if strings.ContainsAny(payload, "\r\n \t") {
		if data, decodeErr := decodeVisionPayload(payload); decodeErr == nil {
			payload = base64.StdEncoding.EncodeToString(data)
		}
	}
	if isDataURI {
		if payload != raw[strings.IndexByte(raw, ',')+1:] {
			materialized.VLMData = raw[:strings.IndexByte(raw, ',')+1] + payload
		}
		return materialized, nil
	}
	if mimeType := imageMIMEForFormat(format); mimeType != "" {
		materialized.VLMData = "data:" + mimeType + ";base64," + payload
	}
	return materialized, nil
}

func inspectVisionImagePayload(payload string) (string, error) {
	if payload == "" || len(payload) > maxVLMEncodedBytes {
		return "", fmt.Errorf("vision image: encoded payload exceeds limit")
	}
	for _, encoding := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding} {
		decoder := base64.NewDecoder(encoding, &visionPayloadReader{source: payload})
		config, format, err := image.DecodeConfig(io.LimitReader(decoder, maxVisionImageBytes+1))
		if err != nil {
			continue
		}
		if config.Width <= 0 || config.Height <= 0 || config.Width > maxVisionImageEdge || config.Height > maxVisionImageEdge || int64(config.Width)*int64(config.Height) > maxVisionImagePixels {
			return "", fmt.Errorf("vision image: image dimensions %dx%d exceed limits", config.Width, config.Height)
		}
		return format, nil
	}
	return "", fmt.Errorf("vision image: invalid image payload")
}

func decodeVisionPayload(raw string) ([]byte, error) {
	encoded := strings.TrimSpace(raw)
	if strings.HasPrefix(strings.ToLower(encoded), "data:") {
		comma := strings.IndexByte(encoded, ',')
		if comma < 0 || !strings.Contains(strings.ToLower(encoded[:comma]), ";base64") {
			return nil, fmt.Errorf("vision image: unsupported data URI")
		}
		encoded = encoded[comma+1:]
	}
	maxEncodedBytes := (maxVisionImageBytes+2)/3*4 + 256
	if len(encoded) == 0 || len(encoded) > maxEncodedBytes {
		return nil, fmt.Errorf("vision image: encoded payload exceeds limit")
	}
	for _, encoding := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding} {
		decoder := base64.NewDecoder(encoding, &visionPayloadReader{source: encoded})
		data, err := io.ReadAll(io.LimitReader(decoder, maxVisionImageBytes+1))
		if err != nil {
			continue
		}
		if len(data) > maxVisionImageBytes {
			return nil, fmt.Errorf("vision image: payload exceeds %d bytes", maxVisionImageBytes)
		}
		if len(data) == 0 {
			return nil, fmt.Errorf("vision image: empty base64 payload")
		}
		return data, nil
	}
	return nil, fmt.Errorf("vision image: decode base64 failed")
}

type visionPayloadReader struct {
	source string
	offset int
}

func (r *visionPayloadReader) Read(dst []byte) (int, error) {
	written := 0
	for written < len(dst) && r.offset < len(r.source) {
		b := r.source[r.offset]
		r.offset++
		if b == '\r' || b == '\n' || b == ' ' || b == '\t' {
			continue
		}
		dst[written] = b
		written++
	}
	if written > 0 {
		return written, nil
	}
	if r.offset == len(r.source) {
		return 0, io.EOF
	}
	return 0, nil
}

func imageWithinVisionLimits(img image.Image) bool {
	if img == nil {
		return false
	}
	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	return width > 0 && height > 0 && width <= maxVisionImageEdge && height <= maxVisionImageEdge && int64(width)*int64(height) <= maxVisionImagePixels
}

func imageMIMEForFormat(format string) string {
	switch strings.ToLower(format) {
	case "jpeg":
		return "image/jpeg"
	case "png":
		return "image/png"
	case "gif":
		return "image/gif"
	case "bmp":
		return "image/bmp"
	case "tiff":
		return "image/tiff"
	case "webp":
		return "image/webp"
	default:
		return ""
	}
}

func encodeVisionRaster(img image.Image) (string, error) {
	if img == nil {
		return "", nil
	}
	if !imageWithinVisionLimits(img) {
		return "", fmt.Errorf("vision image: raster dimensions exceed limits")
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, img); err != nil {
		return "", fmt.Errorf("vision image: encode raster: %w", err)
	}
	if encoded.Len() > maxVisionImageBytes {
		return "", fmt.Errorf("vision image: encoded raster exceeds %d bytes", maxVisionImageBytes)
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(encoded.Bytes()), nil
}
