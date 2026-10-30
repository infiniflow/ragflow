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

import "testing"

func TestImageMaxBytesDefaultsAndConfiguration(t *testing.T) {
	t.Setenv("RAGFLOW_MEDIA_IMAGE_MAX_BYTES", "")
	if got := ImageMaxBytes(); got != 32<<20 {
		t.Fatalf("default = %d, want %d", got, 32<<20)
	}
	t.Setenv("RAGFLOW_MEDIA_IMAGE_MAX_BYTES", "67108864")
	if got := ImageMaxBytes(); got != 64<<20 {
		t.Fatalf("configured = %d, want %d", got, 64<<20)
	}
	for _, invalid := range []string{"0", "-1", "not-a-number"} {
		t.Setenv("RAGFLOW_MEDIA_IMAGE_MAX_BYTES", invalid)
		if got := ImageMaxBytes(); got != 32<<20 {
			t.Fatalf("invalid %q = %d, want the default", invalid, got)
		}
	}
}

func TestMediaRunLimitsDefaultsAndConfiguration(t *testing.T) {
	t.Setenv("RAGFLOW_MEDIA_MAX_IMAGES_PER_RUN", "")
	t.Setenv("RAGFLOW_MEDIA_MAX_BYTES_PER_RUN", "")
	if got := MediaMaxImagesPerRun(); got != 1024 {
		t.Fatalf("default images = %d, want 1024", got)
	}
	if got := MediaMaxBytesPerRun(); got != 1<<30 {
		t.Fatalf("default bytes = %d, want %d", got, 1<<30)
	}

	t.Setenv("RAGFLOW_MEDIA_MAX_IMAGES_PER_RUN", "7")
	t.Setenv("RAGFLOW_MEDIA_MAX_BYTES_PER_RUN", "2048")
	if got := MediaMaxImagesPerRun(); got != 7 {
		t.Fatalf("configured images = %d, want 7", got)
	}
	if got := MediaMaxBytesPerRun(); got != 2048 {
		t.Fatalf("configured bytes = %d, want 2048", got)
	}

	for _, invalid := range []string{"0", "-1", "not-a-number"} {
		t.Setenv("RAGFLOW_MEDIA_MAX_IMAGES_PER_RUN", invalid)
		t.Setenv("RAGFLOW_MEDIA_MAX_BYTES_PER_RUN", invalid)
		if got := MediaMaxImagesPerRun(); got != 1024 {
			t.Fatalf("invalid images %q = %d, want 1024", invalid, got)
		}
		if got := MediaMaxBytesPerRun(); got != 1<<30 {
			t.Fatalf("invalid bytes %q = %d, want %d", invalid, got, int64(1<<30))
		}
	}
}
