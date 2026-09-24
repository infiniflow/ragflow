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
	"image"
	"image/png"
	"strings"
	"testing"
)

func TestMaterializeInlineVisionImageKeepsVLMForOCRDimensionLimit(t *testing.T) {
	var encoded bytes.Buffer
	large := image.NewRGBA(image.Rect(0, 0, maxOCRImageEdge+1, 1))
	if err := png.Encode(&encoded, large); err != nil {
		t.Fatalf("encode oversized image: %v", err)
	}
	dataURI := "data:image/png;base64," + base64.StdEncoding.EncodeToString(encoded.Bytes())

	materialized, err := materializeInlineVisionImage(dataURI)
	if err != nil {
		t.Fatalf("materializeInlineVisionImage: %v", err)
	}
	if materialized == nil || materialized.Raster != nil {
		t.Fatalf("raster = %#v, want OCR to skip the oversized image", materialized)
	}
	if materialized.VLMData != dataURI || !isUsableVisionImage(materialized.VLMData) {
		t.Fatal("oversized OCR input should preserve its valid VLM payload")
	}
}

func TestIsUsableVisionImageRejectsPayloadOverVLMByteBudget(t *testing.T) {
	encodedLength := base64.StdEncoding.EncodedLen(maxVLMImageBytes + 3)
	payload := strings.Repeat("A", encodedLength)
	if isUsableVisionImage(payload) {
		t.Fatal("base64 payload above the VLM byte budget must be rejected")
	}
}
