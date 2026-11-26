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

package media

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"image/color"
	"image/png"
	"math"

	"ragflow/internal/thirdparty/ivy"
	"ragflow/internal/thirdparty/ivy/emf"
)

const maxMetafileEdge = 2048

var metafileRenderSlots = make(chan struct{}, 2)

func init() {
	for _, mime := range []string{"image/wmf", "image/x-wmf", "image/emf", "image/x-emf", "image/x-wmz", "image/x-emz"} {
		RegisterImageTranscoder(mime, transcodeMetafile)
	}
}

// MetafileMIME identifies WMF and EMF by their headers.
func MetafileMIME(data []byte) string {
	switch {
	case len(data) >= 44 && binary.LittleEndian.Uint32(data) == 1 && string(data[40:44]) == " EMF":
		return "image/x-emf"
	case bytes.HasPrefix(data, []byte{0xd7, 0xcd, 0xc6, 0x9a}):
		return "image/x-wmf"
	case len(data) >= 18 && (data[0] == 1 || data[0] == 2) && data[1] == 0 && data[2] == 9 && data[3] == 0 &&
		data[4] == 0 && (data[5] == 1 || data[5] == 3):
		return "image/x-wmf"
	}
	return ""
}

// MetafileMIMEForExtension returns the MIME for a lowercase file extension.
func MetafileMIMEForExtension(ext string) string {
	switch ext {
	case ".wmf":
		return "image/x-wmf"
	case ".emf":
		return "image/x-emf"
	case ".wmz":
		return "image/x-wmz"
	case ".emz":
		return "image/x-emz"
	}
	return ""
}

func transcodeMetafile(ctx context.Context, data []byte) (string, []byte, error) {
	if len(data) == 0 || len(data) > ImageMaxBytes() {
		return "", nil, fmt.Errorf("metafile: input size %d is outside the %d-byte limit", len(data), ImageMaxBytes())
	}
	// The ingestion dispatcher may already have converted the image for OCR.
	if bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")) {
		return "image/png", data, nil
	}
	select {
	case metafileRenderSlots <- struct{}{}:
		defer func() { <-metafileRenderSlots }()
	case <-ctx.Done():
		return "", nil, ctx.Err()
	}
	cfg := ivy.Config{
		SystemFonts: true,
		Limits: ivy.Limits{
			MaxMemory: int64(ImageMaxBytes()),
			MaxPixels: maxMetafileEdge * maxMetafileEdge,
		},
	}
	doc, err := emf.Open(ctx, bytes.NewReader(data), emf.WithConfig(cfg))
	if err != nil {
		return "", nil, fmt.Errorf("open metafile: %w", err)
	}
	defer doc.Close()

	page, err := doc.Page(ctx, 0)
	if err != nil {
		return "", nil, fmt.Errorf("read metafile page: %w", err)
	}
	w, h := page.Size()
	if edge := max(w, h); !(edge > 0) || math.IsInf(float64(edge), 0) {
		return "", nil, fmt.Errorf("metafile: invalid dimensions %gx%g", w, h)
	}
	dpi := min(192, 72*maxMetafileEdge/max(w, h))
	raster, err := ivy.RenderImage(ctx, page, dpi, color.White)
	if err != nil {
		return "", nil, fmt.Errorf("render metafile: %w", err)
	}

	var encoded bytes.Buffer
	if err := png.Encode(&encoded, raster); err != nil {
		return "", nil, fmt.Errorf("encode metafile PNG: %w", err)
	}
	if encoded.Len() > ImageMaxBytes() {
		return "", nil, fmt.Errorf("metafile: PNG exceeds the %d-byte image limit", ImageMaxBytes())
	}
	return "image/png", encoded.Bytes(), nil
}
