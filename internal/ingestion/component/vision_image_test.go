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

func TestMaterializeInlineVisionImageNoEdgeLimitKeepsVLM(t *testing.T) {
	var encoded bytes.Buffer
	// Width exceeds the former 12000-edge guard. With the edge limit removed
	// (aligned to Python's picture.py, which imposes none) the inline image is
	// still materialized and its VLM payload preserved for the vision model.
	wide := image.NewRGBA(image.Rect(0, 0, 12001, 1))
	if err := png.Encode(&encoded, wide); err != nil {
		t.Fatalf("encode wide image: %v", err)
	}
	dataURI := "data:image/png;base64," + base64.StdEncoding.EncodeToString(encoded.Bytes())

	materialized, err := materializeInlineVisionImage(dataURI)
	if err != nil {
		t.Fatalf("materializeInlineVisionImage: %v", err)
	}
	if materialized == nil || materialized.VLMData != dataURI || !isUsableVisionImage(materialized.VLMData) {
		t.Fatalf("wide inline image must preserve its valid VLM payload (no edge limit): %#v", materialized)
	}
}

func TestIsUsableVisionImageRejectsPayloadOverVLMByteBudget(t *testing.T) {
	encodedLength := base64.StdEncoding.EncodedLen(maxVisionImageBytes + 3)
	payload := strings.Repeat("A", encodedLength)
	if isUsableVisionImage(payload) {
		t.Fatal("base64 payload above the VLM byte budget must be rejected")
	}
}
