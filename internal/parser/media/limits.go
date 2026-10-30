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
	"os"
	"strconv"

	"ragflow/internal/common"
)

const (
	defaultImageMaxBytes        = 32 << 20
	defaultMediaMaxImagesPerRun = 1024
	defaultMediaMaxBytesPerRun  = int64(1 << 30)
)

// Limits is the run media quota snapshot shared by parser budgets.
type Limits struct {
	ImageMaxBytes   int64
	MaxImagesPerRun int
	MaxBytesPerRun  int64
}

// LoadLimits reads the configured media limits once for a parser run.
func LoadLimits() Limits {
	return Limits{
		ImageMaxBytes:   int64(ImageMaxBytes()),
		MaxImagesPerRun: MediaMaxImagesPerRun(),
		MaxBytesPerRun:  MediaMaxBytesPerRun(),
	}
}

// ImageMaxBytes is the single per-image limit shared by every media path:
// data URI decoding, ZIP entry extraction, picture parsing, vision images and
// the run media cache. Set RAGFLOW_MEDIA_IMAGE_MAX_BYTES to a positive byte
// count to change it.
func ImageMaxBytes() int {
	if n, err := strconv.Atoi(os.Getenv(common.EnvMediaImageMaxBytes)); err == nil && n > 0 && n <= int(^uint(0)>>1)/4*3 {
		return n
	}
	return defaultImageMaxBytes
}

// MediaMaxImagesPerRun bounds the number of distinct images one run may keep
// in the media cache. Set RAGFLOW_MEDIA_MAX_IMAGES_PER_RUN to a positive count
// to change it.
func MediaMaxImagesPerRun() int {
	if n, err := strconv.Atoi(os.Getenv(common.EnvMediaMaxImagesPerRun)); err == nil && n > 0 {
		return n
	}
	return defaultMediaMaxImagesPerRun
}

// MediaMaxBytesPerRun bounds the total decoded image bytes one run may keep in
// the media cache. Set RAGFLOW_MEDIA_MAX_BYTES_PER_RUN to a positive byte count
// to change it.
func MediaMaxBytesPerRun() int64 {
	if n, err := strconv.ParseInt(os.Getenv(common.EnvMediaMaxBytesPerRun), 10, 64); err == nil && n > 0 {
		return n
	}
	return defaultMediaMaxBytesPerRun
}
