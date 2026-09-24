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
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"net/url"
	"path"
	"strings"

	"gorm.io/gorm"
)

var errUnsupportedHTMLImageSource = errors.New("HTML image source is not a relative in-bucket path")

func isExternalHTMLImageURL(source string) bool {
	u, err := url.Parse(strings.TrimSpace(source))
	return err == nil && (strings.EqualFold(u.Scheme, "https") || strings.EqualFold(u.Scheme, "http")) && u.Host != ""
}

// resolveHTMLImageSource materializes a relative HTML image from the same
// storage bucket before the shared OCR/VLM media path processes it.
func resolveHTMLImageSource(ctx context.Context, bucket, htmlPath string, item map[string]any) error {
	if item == nil || bucket == "" || item["doc_type_kwd"] != "image" {
		return errUnsupportedHTMLImageSource
	}
	source, _ := item["image_src"].(string)
	assetPath, ok := resolveRelativeHTMLImagePath(htmlPath, source)
	if !ok {
		return errUnsupportedHTMLImageSource
	}
	data, err := FetchBinaryLimited(ctx, bucket, assetPath, maxOCRImageBytes)
	if err != nil {
		return fmt.Errorf("read HTML image asset: %w", err)
	}
	if len(data) == 0 {
		return errors.New("HTML image asset is empty")
	}
	payload, ok := htmlImageDataURI(data)
	if !ok {
		return errors.New("HTML image asset is not a supported raster")
	}
	item["image"] = payload
	delete(item, "image_src")
	return nil
}

func htmlSourceStorageLocation(ctx context.Context, db *gorm.DB, inputs map[string]any) (string, string, bool) {
	bucket, _ := getString(inputs, "bucket")
	objectPath, _ := getString(inputs, "path")
	if bucket != "" && objectPath != "" {
		return bucket, objectPath, true
	}
	docID, _ := getString(inputs, "doc_id")
	if docID == "" || (db == nil && ResolveDocumentStorageOverride == nil) {
		return "", "", false
	}
	ref, err := ResolveDocumentStorage(ctx, db, docID)
	if err != nil || ref == nil || ref.Bucket == "" || ref.Path == "" {
		return "", "", false
	}
	return ref.Bucket, ref.Path, true
}

func resolveRelativeHTMLImagePath(htmlObjectPath, imageSource string) (string, bool) {
	if strings.TrimSpace(htmlObjectPath) == "" {
		return "", false
	}
	if strings.Contains(imageSource, `\`) {
		return "", false
	}
	sourceURL, err := url.Parse(strings.ReplaceAll(strings.TrimSpace(imageSource), " ", "%20"))
	if err != nil || sourceURL.IsAbs() || sourceURL.Host != "" || sourceURL.Opaque != "" ||
		sourceURL.Path == "" || strings.HasPrefix(sourceURL.Path, "/") {
		return "", false
	}
	cleanHTMLPath := path.Clean(strings.TrimSpace(htmlObjectPath))
	if path.IsAbs(cleanHTMLPath) || cleanHTMLPath == ".." || strings.HasPrefix(cleanHTMLPath, "../") {
		return "", false
	}
	baseDir := path.Dir(cleanHTMLPath)
	assetPath := path.Clean(path.Join(baseDir, sourceURL.Path))
	if path.IsAbs(assetPath) || assetPath == "." || assetPath == ".." || strings.HasPrefix(assetPath, "../") {
		return "", false
	}
	// Relative references may leave the HTML directory, but must stay inside
	// the current storage bucket.
	if assetPath == ".." || strings.HasPrefix(assetPath, "../") {
		return "", false
	}
	return assetPath, true
}

func htmlImageDataURI(data []byte) (string, bool) {
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width <= 0 || config.Height <= 0 ||
		config.Width > maxOCRImageEdge || config.Height > maxOCRImageEdge ||
		int64(config.Width)*int64(config.Height) > maxOCRImagePixels {
		return "", false
	}
	mimeType := imageMIMEForFormat(format)
	if mimeType == "" {
		return "", false
	}
	return "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(data), true
}
