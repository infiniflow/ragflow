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
	"strings"
)

const (
	maxOCRImageBytes  = 32 << 20
	maxOCRImagePixels = 40_000_000
	maxOCRImageEdge   = 12_000
)

func materializeInlineVisionImage(raw string) (*visionImage, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	materialized := &visionImage{VLMData: raw}
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		return materialized, nil
	}

	data, err := decodeVisionPayload(raw)
	if err != nil {
		return materialized, nil
	}
	raster, format, err := decodeOCRImage(data)
	if err != nil {
		return materialized, nil
	}
	materialized.Raster = raster
	if !strings.HasPrefix(raw, "data:image/") {
		mimeType := imageMIMEForFormat(format)
		if mimeType != "" {
			materialized.VLMData = "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(data)
		}
	}
	return materialized, nil
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
	encoded = strings.Map(func(r rune) rune {
		if r == '\r' || r == '\n' || r == ' ' || r == '\t' {
			return -1
		}
		return r
	}, encoded)
	maxEncodedBytes := (maxOCRImageBytes+2)/3*4 + 256
	if len(encoded) == 0 || len(encoded) > maxEncodedBytes {
		return nil, fmt.Errorf("vision image: encoded payload exceeds limit")
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		data, err = base64.RawStdEncoding.DecodeString(encoded)
	}
	if err != nil {
		return nil, fmt.Errorf("vision image: decode base64: %w", err)
	}
	if len(data) > maxOCRImageBytes {
		return nil, fmt.Errorf("vision image: payload exceeds %d bytes", maxOCRImageBytes)
	}
	return data, nil
}

func decodeOCRImage(data []byte) (image.Image, string, error) {
	if len(data) == 0 || len(data) > maxOCRImageBytes {
		return nil, "", fmt.Errorf("local OCR: image payload size %d is outside limits", len(data))
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, "", fmt.Errorf("local OCR: decode image config: %w", err)
	}
	if config.Width <= 0 || config.Height <= 0 || config.Width > maxOCRImageEdge || config.Height > maxOCRImageEdge || int64(config.Width)*int64(config.Height) > maxOCRImagePixels {
		return nil, "", fmt.Errorf("local OCR: image dimensions %dx%d exceed limits", config.Width, config.Height)
	}
	img, decodedFormat, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, "", fmt.Errorf("local OCR: decode image: %w", err)
	}
	return img, decodedFormat, nil
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
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, img); err != nil {
		return "", fmt.Errorf("vision image: encode raster: %w", err)
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(encoded.Bytes()), nil
}
