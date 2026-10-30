//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package parser

import (
	"context"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
	"ragflow/internal/utility"
)

func TestXLSXParserFailsWhenMediaCacheIsMissing(t *testing.T) {
	data := newTestXLSX(t, func(f *excelize.File) {
		mustSetCell(t, f, "Sheet1", "A1", "Name")
		if err := f.AddPictureFromBytes("Sheet1", "C3", &excelize.Picture{
			Extension: ".png",
			File:      officeTestPNG(t, 15),
		}); err != nil {
			t.Fatalf("AddPictureFromBytes: %v", err)
		}
	})
	p, err := NewXLSXParser("")
	if err != nil {
		t.Fatalf("NewXLSXParser: %v", err)
	}
	res := p.ParseWithResult(context.Background(), "missing.xlsx", data)
	if res.Err == nil || !strings.Contains(res.Err.Error(), "media cache is not configured") {
		t.Fatalf("ParseWithResult error = %v, want missing-cache failure", res.Err)
	}
}

func TestXLSXTextParserSkipsImagesWithoutCache(t *testing.T) {
	data := newTestXLSX(t, func(f *excelize.File) {
		mustSetCell(t, f, "Sheet1", "A1", "Name")
		mustSetCell(t, f, "Sheet1", "A2", "Alice")
		if err := f.AddPictureFromBytes("Sheet1", "C3", &excelize.Picture{
			Extension: ".png",
			File:      officeTestPNG(t, 16),
			Format:    &excelize.GraphicOptions{AltText: "figure"},
		}); err != nil {
			t.Fatalf("AddPictureFromBytes: %v", err)
		}
	})
	p, err := GetTextOnlyParser(utility.FileTypeXLSX)
	if err != nil {
		t.Fatal(err)
	}
	res := p.ParseWithResult(context.Background(), "text.xlsx", data)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if len(res.JSON) != 1 || res.JSON[0].ToMap()["doc_type_kwd"] != "table" {
		t.Fatalf("items = %+v, want the sheet table without the skipped image", ItemsToMaps(res.JSON))
	}
	if !strings.Contains(spreadsheetText(res), "Alice") {
		t.Fatalf("parsed rows = %q, want sheet content", spreadsheetText(res))
	}
	if len(res.Warnings) != 0 {
		t.Fatalf("warnings = %v, want none for a text-only parse", res.Warnings)
	}
}

// TestXLSXParserNormalizationRetryEmitsImage covers a workbook whose second
// sheet needs name normalization: the first attempt parses the first sheet
// (including its image) and then fails; the retry must reuse the admitted image
// reference without loading or writing the same source a second time.
func TestXLSXParserNormalizationRetryEmitsImage(t *testing.T) {
	data := newTestXLSX(t, func(f *excelize.File) {
		mustSetCell(t, f, "Sheet1", "A1", "Name")
		mustSetCell(t, f, "Sheet1", "A2", "Alice")
		if err := f.AddPictureFromBytes("Sheet1", "C3", &excelize.Picture{
			Extension: ".png",
			File:      officeTestPNG(t, 17),
			Format:    &excelize.GraphicOptions{AltText: "figure"},
		}); err != nil {
			t.Fatalf("AddPictureFromBytes: %v", err)
		}
		if _, err := f.NewSheet("Data"); err != nil {
			t.Fatalf("NewSheet: %v", err)
		}
	})
	data = rewriteXLSXEntry(t, data, "xl/workbook.xml", func(content []byte) []byte {
		updated := strings.Replace(string(content), `name="Data"`, `name="Visible:Data"`, 1)
		if updated == string(content) {
			t.Fatal("workbook.xml did not contain the Data sheet")
		}
		return []byte(updated)
	})

	ctx, cache := officeMediaTestContext()
	p, err := NewXLSXParser("")
	if err != nil {
		t.Fatalf("NewXLSXParser: %v", err)
	}
	res := p.ParseWithResult(ctx, "normalize-image.xlsx", data)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	var imageItem map[string]any
	for _, item := range ItemsToMaps(res.JSON) {
		if item["doc_type_kwd"] == "image" {
			imageItem = item
		}
	}
	if imageItem == nil {
		t.Fatalf("items = %+v, want the sheet image", ItemsToMaps(res.JSON))
	}
	ref := requireOfficeMediaRef(t, imageItem)
	if ref.Format != officeMediaFormatXLSX || ref.MIME != "image/png" {
		t.Fatalf("media_ref = %+v, want xlsx image/png", ref)
	}
	if imageItem["sheet"] != "Sheet1" || imageItem["cell"] != "C3" {
		t.Fatalf("image identity = %+v, want Sheet1!C3", imageItem)
	}
	if cache.putCalls != 1 {
		t.Fatalf("cache writes = %d, want one across both parse attempts", cache.putCalls)
	}
	if !strings.Contains(strings.Join(res.Warnings, "\n"), "Visible_Data") {
		t.Fatalf("warnings = %v, want sheet-name normalization warning", res.Warnings)
	}
}
