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
	"sync"

	deepdocpdf "ragflow/internal/deepdoc/parser/pdf"
)

const (
	// MaxImagePayloadBytes bounds compressed image data accepted by media paths.
	MaxImagePayloadBytes = 32 << 20
	// MaxImagePixels bounds decoded image dimensions before allocating a raster.
	MaxImagePixels = 40_000_000
	// MaxImageEdge bounds either decoded image dimension before raster allocation.
	MaxImageEdge = 12_000
)

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
