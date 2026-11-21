//go:build !cgo

// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package models

import (
	"context"
	"fmt"
	"image"
)

func xiaomiOCRPDF(context.Context, []byte, *OCRConfig, func(image.Image) (string, error)) (*OCRFileResponse, error) {
	return nil, fmt.Errorf("XiaomiOCR PDF rendering requires cgo")
}
