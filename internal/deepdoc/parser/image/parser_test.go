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

package imageparser

import (
	"context"
	"image"
	"image/color"
	"image/draw"
	"strings"
	"testing"

	deepdoctype "ragflow/internal/deepdoc/parser/type"
)

type testAnalyzer struct {
	boxes     []deepdoctype.OCRBox
	texts     map[color.RGBA]string
	regions   []deepdoctype.DLARegion
	cells     []deepdoctype.TSRCell
	unhealthy bool
}

func (a *testAnalyzer) DLA(context.Context, image.Image) ([]deepdoctype.DLARegion, error) {
	return a.regions, nil
}

func (a *testAnalyzer) TSR(context.Context, image.Image) ([]deepdoctype.TSRCell, error) {
	return a.cells, nil
}

func (a *testAnalyzer) OCRDetect(context.Context, image.Image) ([]deepdoctype.OCRBox, error) {
	return a.boxes, nil
}

func (a *testAnalyzer) OCRRecognize(_ context.Context, img image.Image) ([]deepdoctype.OCRText, error) {
	bounds := img.Bounds()
	center := color.RGBAModel.Convert(img.At(
		bounds.Min.X+bounds.Dx()/2,
		bounds.Min.Y+bounds.Dy()/2,
	)).(color.RGBA)
	text, ok := a.texts[center]
	if !ok {
		return nil, nil
	}
	return []deepdoctype.OCRText{{Text: text, Confidence: 0.9}}, nil
}

func (a *testAnalyzer) Health() bool { return !a.unhealthy }

type testCell struct {
	rect image.Rectangle
	text string
	ink  color.RGBA
}

func newTestImage(width, height int, cells []testCell) (image.Image, []deepdoctype.OCRBox, map[color.RGBA]string) {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(img, img.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
	boxes := make([]deepdoctype.OCRBox, 0, len(cells))
	texts := make(map[color.RGBA]string, len(cells))
	for _, cell := range cells {
		draw.Draw(img, cell.rect, &image.Uniform{C: cell.ink}, image.Point{}, draw.Src)
		boxes = append(boxes, deepdoctype.OCRBox{
			X0: float64(cell.rect.Min.X), Y0: float64(cell.rect.Min.Y),
			X1: float64(cell.rect.Max.X), Y1: float64(cell.rect.Min.Y),
			X2: float64(cell.rect.Max.X), Y2: float64(cell.rect.Max.Y),
			X3: float64(cell.rect.Min.X), Y3: float64(cell.rect.Max.Y),
		})
		texts[cell.ink] = cell.text
	}
	return img, boxes, texts
}

func tableCells() []deepdoctype.TSRCell {
	return []deepdoctype.TSRCell{
		{X0: 0, Y0: 0, X1: 240, Y1: 120, Label: "table"},
		{X0: 10, Y0: 10, X1: 110, Y1: 50, Label: "table column"},
		{X0: 110, Y0: 10, X1: 230, Y1: 50, Label: "table column"},
		{X0: 10, Y0: 10, X1: 230, Y1: 50, Label: "table row"},
		{X0: 10, Y0: 60, X1: 230, Y1: 100, Label: "table row"},
		{X0: 10, Y0: 10, X1: 230, Y1: 50, Label: "table column header"},
	}
}

func TestPageEngine(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 20, 10))
	engine := &pageEngine{img: img}

	if count, err := engine.PageCount(); err != nil || count != 1 {
		t.Fatalf("PageCount() = %d, %v", count, err)
	}
	if chars, err := engine.ExtractChars(0); err != nil || chars != nil {
		t.Fatalf("ExtractChars() = %v, %v", chars, err)
	}
	rendered, err := engine.RenderPageImage(0, 216)
	if err != nil || rendered != img {
		t.Fatalf("RenderPageImage() = %v, %v", rendered, err)
	}
	if _, err := engine.RenderPageImage(1, 216); err == nil {
		t.Fatal("expected an error for a missing page")
	}
	if _, err := engine.RenderPage(0, 216); err == nil {
		t.Fatal("expected an error for PDF byte rendering")
	}
	if engine.RawData() != nil {
		t.Fatal("RawData() must be nil so the renderer falls back to the image")
	}
	if outlines, err := engine.Outlines(); err != nil || outlines != nil {
		t.Fatalf("Outlines() = %v, %v", outlines, err)
	}
	if err := engine.Close(); err != nil {
		t.Fatalf("Close() = %v", err)
	}
}

func TestParseFindsTable(t *testing.T) {
	img, boxes, texts := newTestImage(240, 140, []testCell{
		{image.Rect(20, 20, 60, 40), "Name", color.RGBA{R: 255, A: 255}},
		{image.Rect(120, 20, 160, 40), "Value", color.RGBA{G: 255, A: 255}},
		{image.Rect(20, 80, 50, 100), "A", color.RGBA{B: 255, A: 255}},
		{image.Rect(120, 80, 150, 100), "42", color.RGBA{R: 255, G: 255, A: 255}},
	})
	analyzer := &testAnalyzer{
		boxes:   boxes,
		texts:   texts,
		regions: []deepdoctype.DLARegion{{X0: 0, Y0: 0, X1: 240, Y1: 120, Label: "table", Confidence: 0.9}},
		cells:   tableCells(),
	}

	blocks, err := Parse(t.Context(), img, analyzer)
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 1 || blocks[0].Kind != BlockTable {
		t.Fatalf("blocks = %+v, want one table block", blocks)
	}
	table := blocks[0].Table
	if table == nil || len(table.Grid) != 2 || len(table.Grid[0]) != 2 {
		t.Fatalf("table = %+v", table)
	}
	if table.HeaderRows != 1 {
		t.Fatalf("header rows = %d, want 1", table.HeaderRows)
	}
	if got := table.Grid[0][0].Text; got != "Name" {
		t.Errorf("cell[0][0] = %q", got)
	}
	if got := table.Grid[1][1].Text; got != "42" {
		t.Errorf("cell[1][1] = %q", got)
	}
	if blocks[0].Image == nil {
		t.Error("table preview is nil")
	}
}

func TestParseKeepsTextWhenNoTable(t *testing.T) {
	img, boxes, texts := newTestImage(240, 140, []testCell{
		{image.Rect(20, 20, 60, 40), "Name", color.RGBA{R: 255, A: 255}},
		{image.Rect(120, 20, 160, 40), "Value", color.RGBA{G: 255, A: 255}},
		{image.Rect(20, 80, 50, 100), "A", color.RGBA{B: 255, A: 255}},
		{image.Rect(120, 80, 150, 100), "42", color.RGBA{R: 255, G: 255, A: 255}},
	})
	blocks, err := Parse(t.Context(), img, &testAnalyzer{boxes: boxes, texts: texts})
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) == 0 {
		t.Fatal("expected text blocks")
	}
	var text strings.Builder
	for _, block := range blocks {
		if block.Kind != BlockText {
			t.Fatalf("block = %+v, want text", block)
		}
		text.WriteString(block.Text)
		text.WriteByte('\n')
	}
	for _, want := range []string{"Name", "Value", "A", "42"} {
		if !strings.Contains(text.String(), want) {
			t.Errorf("text %q does not contain %q", text.String(), want)
		}
	}
}

func TestParseMergesTableCaption(t *testing.T) {
	img, boxes, texts := newTestImage(240, 180, []testCell{
		{image.Rect(20, 45, 80, 55), "Table 1", color.RGBA{R: 128, G: 128, B: 128, A: 255}},
		{image.Rect(40, 70, 80, 90), "Name", color.RGBA{R: 255, A: 255}},
		{image.Rect(150, 70, 190, 90), "Value", color.RGBA{G: 255, A: 255}},
		{image.Rect(40, 120, 70, 140), "A", color.RGBA{B: 255, A: 255}},
		{image.Rect(150, 120, 180, 140), "42", color.RGBA{R: 255, G: 255, A: 255}},
	})
	analyzer := &testAnalyzer{
		boxes:   boxes,
		texts:   texts,
		regions: []deepdoctype.DLARegion{{X0: 0, Y0: 60, X1: 240, Y1: 160, Label: "table", Confidence: 0.9}},
		cells: []deepdoctype.TSRCell{
			{X0: 0, Y0: 0, X1: 240, Y1: 130, Label: "table"},
			{X0: 10, Y0: 40, X1: 110, Y1: 110, Label: "table column"},
			{X0: 110, Y0: 40, X1: 230, Y1: 110, Label: "table column"},
			{X0: 10, Y0: 40, X1: 230, Y1: 60, Label: "table row"},
			{X0: 10, Y0: 90, X1: 230, Y1: 110, Label: "table row"},
			{X0: 10, Y0: 40, X1: 230, Y1: 60, Label: "table column header"},
		},
	}

	blocks, err := Parse(t.Context(), img, analyzer)
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 1 || blocks[0].Kind != BlockTable {
		t.Fatalf("blocks = %+v, want one table block", blocks)
	}
	if got := blocks[0].Table.Caption; got != "Table 1" {
		t.Fatalf("caption = %q", got)
	}
	if got := blocks[0].Table.Grid[0][0].Text; got != "Name" {
		t.Fatalf("header = %q", got)
	}
}

func TestParseKeepsSingleColumnTable(t *testing.T) {
	img, boxes, texts := newTestImage(240, 140, []testCell{
		{image.Rect(20, 20, 60, 40), "Name", color.RGBA{R: 255, A: 255}},
		{image.Rect(20, 80, 50, 100), "A", color.RGBA{B: 255, A: 255}},
	})
	analyzer := &testAnalyzer{
		boxes:   boxes,
		texts:   texts,
		regions: []deepdoctype.DLARegion{{X0: 0, Y0: 0, X1: 240, Y1: 120, Label: "table", Confidence: 0.9}},
		cells: []deepdoctype.TSRCell{
			{X0: 0, Y0: 0, X1: 240, Y1: 120, Label: "table"},
			{X0: 10, Y0: 10, X1: 230, Y1: 50, Label: "table row"},
			{X0: 10, Y0: 60, X1: 230, Y1: 100, Label: "table row"},
		},
	}

	blocks, err := Parse(t.Context(), img, analyzer)
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 1 || blocks[0].Kind != BlockTable {
		t.Fatalf("blocks = %+v, want one table block", blocks)
	}
	table := blocks[0].Table
	if table == nil || len(table.Grid) != 2 || len(table.Grid[0]) != 1 {
		t.Fatalf("table = %+v", table)
	}
	if table.Grid[0][0].Text != "Name" || table.Grid[1][0].Text != "A" {
		t.Fatalf("table grid = %+v", table.Grid)
	}
	if blocks[0].Image == nil {
		t.Error("table preview is nil")
	}
}

func TestParseEmitsFigureImage(t *testing.T) {
	img, boxes, texts := newTestImage(240, 140, []testCell{
		{image.Rect(20, 20, 60, 40), "Chart", color.RGBA{R: 10, G: 20, B: 30, A: 255}},
	})
	analyzer := &testAnalyzer{
		boxes:   boxes,
		texts:   texts,
		regions: []deepdoctype.DLARegion{{X0: 0, Y0: 0, X1: 240, Y1: 120, Label: "figure", Confidence: 0.9}},
	}

	blocks, err := Parse(t.Context(), img, analyzer)
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 1 || blocks[0].Kind != BlockImage {
		t.Fatalf("blocks = %+v, want one image block", blocks)
	}
	if blocks[0].Image == nil {
		t.Fatal("figure crop is nil")
	}
}

func TestParseTableAndFigure(t *testing.T) {
	img, boxes, texts := newTestImage(300, 140, []testCell{
		{image.Rect(20, 20, 60, 40), "Name", color.RGBA{R: 255, A: 255}},
		{image.Rect(20, 80, 50, 100), "A", color.RGBA{B: 255, A: 255}},
		{image.Rect(220, 20, 260, 40), "Chart", color.RGBA{R: 10, G: 20, B: 30, A: 255}},
	})
	analyzer := &testAnalyzer{
		boxes: boxes,
		texts: texts,
		regions: []deepdoctype.DLARegion{
			{X0: 0, Y0: 0, X1: 180, Y1: 120, Label: "table", Confidence: 0.9},
			{X0: 200, Y0: 0, X1: 300, Y1: 120, Label: "figure", Confidence: 0.9},
		},
		cells: []deepdoctype.TSRCell{
			{X0: 0, Y0: 0, X1: 180, Y1: 120, Label: "table"},
			{X0: 10, Y0: 10, X1: 170, Y1: 50, Label: "table row"},
			{X0: 10, Y0: 60, X1: 170, Y1: 100, Label: "table row"},
		},
	}

	blocks, err := Parse(t.Context(), img, analyzer)
	if err != nil {
		t.Fatal(err)
	}
	var tables, images int
	for _, block := range blocks {
		switch block.Kind {
		case BlockTable:
			tables++
		case BlockImage:
			images++
			if block.Image == nil {
				t.Fatal("figure crop is nil")
			}
		}
	}
	if tables != 1 || images != 1 {
		t.Fatalf("blocks = %+v, want one table and one image", blocks)
	}
}

func TestParseKeepsTextWhenTableStructureMissing(t *testing.T) {
	img, boxes, texts := newTestImage(240, 140, []testCell{
		{image.Rect(20, 20, 60, 40), "Name", color.RGBA{R: 255, A: 255}},
		{image.Rect(20, 80, 50, 100), "A", color.RGBA{B: 255, A: 255}},
	})
	analyzer := &testAnalyzer{
		boxes:   boxes,
		texts:   texts,
		regions: []deepdoctype.DLARegion{{X0: 0, Y0: 0, X1: 240, Y1: 120, Label: "table", Confidence: 0.9}},
		// No TSR cells: the table region keeps its plain OCR text.
	}

	blocks, err := Parse(t.Context(), img, analyzer)
	if err != nil {
		t.Fatal(err)
	}
	for _, block := range blocks {
		if block.Kind == BlockTable {
			t.Fatalf("table structure is missing; got a table block: %+v", block)
		}
	}
	var text strings.Builder
	for _, block := range blocks {
		if block.Kind == BlockText {
			text.WriteString(block.Text)
			text.WriteByte('\n')
		}
	}
	for _, want := range []string{"Name", "A"} {
		if !strings.Contains(text.String(), want) {
			t.Fatalf("text %q does not contain %q", text.String(), want)
		}
	}
}

func TestParseNilImage(t *testing.T) {
	if _, err := Parse(context.Background(), nil, &testAnalyzer{}); err == nil {
		t.Fatal("expected an error for a nil image")
	}
}

func TestParseUnhealthyAnalyzer(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	if _, err := Parse(context.Background(), img, &testAnalyzer{unhealthy: true}); err == nil {
		t.Fatal("expected an error for an unhealthy analyzer")
	}
}
