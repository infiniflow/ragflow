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
//

package component

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"testing"

	imageparser "ragflow/internal/deepdoc/parser/image"
	deepdoctype "ragflow/internal/deepdoc/parser/type"
	"ragflow/internal/entity"
	"ragflow/internal/parser/parser"
	"ragflow/internal/utility"
)

type pictureTableAnalyzer struct {
	deepdoctype.DocAnalyzer
}

func (*pictureTableAnalyzer) Health() bool { return true }

func (*pictureTableAnalyzer) DLA(context.Context, image.Image) ([]deepdoctype.DLARegion, error) {
	return []deepdoctype.DLARegion{{X0: 0, Y0: 0, X1: 240, Y1: 120, Label: "table", Confidence: 0.9}}, nil
}

func (*pictureTableAnalyzer) TSR(context.Context, image.Image) ([]deepdoctype.TSRCell, error) {
	return []deepdoctype.TSRCell{
		{X0: 0, Y0: 0, X1: 240, Y1: 120, Label: "table"},
		{X0: 10, Y0: 10, X1: 110, Y1: 50, Label: "table column"},
		{X0: 110, Y0: 10, X1: 230, Y1: 50, Label: "table column"},
		{X0: 10, Y0: 10, X1: 230, Y1: 50, Label: "table row"},
		{X0: 10, Y0: 60, X1: 230, Y1: 100, Label: "table row"},
		{X0: 10, Y0: 10, X1: 230, Y1: 50, Label: "table column header"},
	}, nil
}

func (*pictureTableAnalyzer) OCRDetect(context.Context, image.Image) ([]deepdoctype.OCRBox, error) {
	return []deepdoctype.OCRBox{
		{X0: 20, Y0: 20, X1: 60, Y1: 20, X2: 60, Y2: 40, X3: 20, Y3: 40},
		{X0: 120, Y0: 20, X1: 170, Y1: 20, X2: 170, Y2: 40, X3: 120, Y3: 40},
		{X0: 20, Y0: 90, X1: 50, Y1: 90, X2: 50, Y2: 110, X3: 20, Y3: 110},
		{X0: 120, Y0: 90, X1: 180, Y1: 90, X2: 180, Y2: 110, X3: 120, Y3: 110},
	}, nil
}

func (*pictureTableAnalyzer) OCRRecognize(_ context.Context, img image.Image) ([]deepdoctype.OCRText, error) {
	text := map[int]string{30: "A", 40: "Name", 50: "Value", 60: "42"}[img.Bounds().Dx()]
	return []deepdoctype.OCRText{{Text: text, Confidence: 0.99}}, nil
}

func tablePicturePNG(t *testing.T) []byte {
	t.Helper()
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 240, 140))); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

func TestMaybeDispatchImageEmitsTableItem(t *testing.T) {
	original := deepdoctype.NativeDocAnalyzerFactory
	t.Cleanup(func() { deepdoctype.NativeDocAnalyzerFactory = original })
	analyzer := &pictureTableAnalyzer{}
	deepdoctype.NativeDocAnalyzerFactory = func() (deepdoctype.DocAnalyzer, bool) { return analyzer, true }

	result, handled, err := maybeDispatchImage(t.Context(), parser.NewPictureParser(), nil, utility.FileTypeVISUAL, "table.png", tablePicturePNG(t), map[string]any{"tenant_id": "t1"}, defaultSetups(), false, visionSettings{})
	if err != nil {
		t.Fatalf("maybeDispatchImage: %v", err)
	}
	if !handled {
		t.Fatal("expected the image branch to handle the file")
	}
	if len(result.JSON) != 1 {
		t.Fatalf("JSON len = %d, want one table item: %+v", len(result.JSON), result.JSON)
	}
	item := result.JSON[0].ToMap()
	if item["doc_type_kwd"] != "table" {
		t.Fatalf("doc_type_kwd = %v, want table", item["doc_type_kwd"])
	}
	table, ok := item["table"].(*entity.TableData)
	if !ok || table == nil {
		t.Fatalf("table payload = %#v", item["table"])
	}
	if table.HeaderRows != 1 || len(table.Grid) != 2 || len(table.Grid[0]) != 2 {
		t.Fatalf("table shape = %+v", table)
	}
	if table.Grid[0][0].Text != "Name" || table.Grid[1][1].Text != "42" {
		t.Fatalf("table grid = %+v", table.Grid)
	}
	if item["image"] == nil || item["image"] == "" {
		t.Fatal("table item lost its crop preview")
	}
	if _, has := item["positions"]; has {
		t.Fatal("image table item must not carry PDF positions")
	}
	if len(result.Warnings) != 0 {
		t.Fatalf("warnings = %v", result.Warnings)
	}
}

func TestImageBlockItemsEmitsFigureItem(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	items := imageBlockItems([]imageparser.Block{{
		Kind:  imageparser.BlockImage,
		Text:  "chart",
		Image: img,
	}})
	if len(items) != 1 {
		t.Fatalf("items = %d, want one figure item", len(items))
	}
	item := items[0].ToMap()
	if item["doc_type_kwd"] != "image" {
		t.Fatalf("doc_type_kwd = %v, want image", item["doc_type_kwd"])
	}
	if item["text"] != "chart" {
		t.Fatalf("text = %v, want chart", item["text"])
	}
	if item["layout"] != "figure" {
		t.Fatalf("layout = %v, want figure", item["layout"])
	}
	if item["image"] == nil || item["image"] == "" {
		t.Fatal("figure item lost its crop preview")
	}
}
