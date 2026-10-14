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

package parser

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"

	deepdocpdf "ragflow/internal/deepdoc/parser/pdf"
)

const (
	// MaxImagePayloadBytes bounds compressed image data accepted by media paths.
	MaxImagePayloadBytes = 32 << 20
)

// EnvImageRasterMaxBytes names the environment variable that sets the
// decoded-raster byte ceiling for image parsing (media dispatch and inline
// vision images). It mirrors Python's picture.py, which imposes no image size
// guard at all: length/width are unbounded and the raster-byte ceiling is opt
// in. A value of 0 (the default when unset, empty, or invalid/negative) means
// unlimited. When set to N > 0 (bytes), a decoded raster larger than N bytes
// (modelled as width * height * 4 for an 8-bit RGBA raster) is rejected.
//
// Operator guidance: with the default (unlimited) ceiling, a highly
// compressible image can still exhaust the ingestion worker's heap during
// decode. For untrusted input sources, set RAGFLOW_IMAGE_RASTER_MAX_BYTES to a
// budget the deployment can absorb (bounded by DeepDocConcurrency). The 4
// bytes/pixel model is an 8-bit RGBA estimate; 16-bit formats (e.g. NRGBA64)
// allocate roughly twice that, so size the ceiling with headroom for such
// input.
const EnvImageRasterMaxBytes = "RAGFLOW_IMAGE_RASTER_MAX_BYTES"

var (
	imageRasterMaxBytesOnce sync.Once
	imageRasterMaxBytes     int64
)

// ImageRasterMaxBytes returns the decoded-raster byte ceiling. It is resolved
// once from EnvImageRasterMaxBytes; negative or unparseable values fall back to
// 0 (unlimited) so a misconfiguration never silently blocks every image.
func ImageRasterMaxBytes() int64 {
	imageRasterMaxBytesOnce.Do(func() {
		imageRasterMaxBytes = resolveImageRasterMaxBytes()
	})
	return imageRasterMaxBytes
}

func resolveImageRasterMaxBytes() int64 {
	v := strings.TrimSpace(os.Getenv(EnvImageRasterMaxBytes))
	if v == "" {
		return 0
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// CheckImageRasterLimit returns an error if a w-by-h image's decoded RGBA
// raster would exceed the opt-in byte ceiling (ImageRasterMaxBytes). Edge
// (length/width) limits are intentionally absent to match Python's picture.py.
// A zero ceiling means unlimited and always passes.
//
// The raster is modelled as 4 bytes/pixel (8-bit RGBA); see EnvImageRasterMaxBytes
// for the 16-bit headroom note. The product is computed in uint64 so extreme
// dimensions cannot silently wrap to 0 under int64 signed overflow and bypass
// a positive ceiling.
func CheckImageRasterLimit(w, h int) error {
	if max := ImageRasterMaxBytes(); max > 0 {
		const bytesPerPixel = 4
		if uint64(w)*uint64(h)*bytesPerPixel > uint64(max) {
			return fmt.Errorf("image raster %dx%d exceeds %d-byte limit", w, h, max)
		}
	}
	return nil
}

// AcquireImageMedia limits concurrent image materialization across Parser
// payload encoding and ingestion VLM crop paths. The native analyzer keeps
// its own inference limit.
func AcquireImageMedia(ctx context.Context) (func(), error) {
	return sharedImageMediaAdmission().acquire(ctx)
}

type imageMediaAdmission struct {
	slots chan struct{}
}

func newImageMediaAdmission(limit int) *imageMediaAdmission {
	if limit < 1 {
		limit = 1
	}
	return &imageMediaAdmission{slots: make(chan struct{}, limit)}
}

func (a *imageMediaAdmission) acquire(ctx context.Context) (func(), error) {
	select {
	case a.slots <- struct{}{}:
		var once sync.Once
		return func() { once.Do(func() { <-a.slots }) }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

var (
	imageMediaAdmissionOnce sync.Once
	imageMediaAdmissionPool *imageMediaAdmission
)

func sharedImageMediaAdmission() *imageMediaAdmission {
	imageMediaAdmissionOnce.Do(func() {
		imageMediaAdmissionPool = newImageMediaAdmission(deepdocpdf.DeepDocConcurrency())
	})
	return imageMediaAdmissionPool
}
