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
// (width * height * 4 for the RGBA raster image.Decode allocates) is rejected.
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
func CheckImageRasterLimit(w, h int) error {
	if max := ImageRasterMaxBytes(); max > 0 {
		if bytes := int64(w) * int64(h) * 4; bytes > max {
			return fmt.Errorf("image raster %dx%d (%d bytes) exceeds %d-byte limit", w, h, bytes, max)
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
