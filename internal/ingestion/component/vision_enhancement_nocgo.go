//go:build !cgo

//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package component

import (
	"context"

	"gorm.io/gorm"
)

// visionPDFCropperNoop is the !cgo cropper: without a native renderer there is
// no on-demand crop, so it returns the inlined image when present (docx/
// markdown or any pre-inlined source) and nothing otherwise. The parser does
// not inline PDF media under cgo, and PDF parsing itself requires cgo, so this
// path is only ever exercised for the non-PDF inline forms.
type visionPDFCropperNoop struct{}

func newVisionImageCropper(_ context.Context, _ *gorm.DB, _ map[string]any) (visionImageCropper, error) {
	return visionPDFCropperNoop{}, nil
}

func (visionPDFCropperNoop) Crop(_ context.Context, item map[string]any) (*visionImage, error) {
	if img, _ := item["image"].(string); img != "" {
		return materializeInlineVisionImage(img)
	}
	return nil, nil
}

func (visionPDFCropperNoop) Close() error { return nil }
