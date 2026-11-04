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

// Package imageparser runs a standalone image through the DeepDoc page
// pipeline. An image is a one-page document with no embedded characters, so
// OCR, layout analysis, table structure recognition, reading order, and table
// construction all reuse the PDF page path.
package imageparser

import (
	"context"
	"errors"
	"image"
	"math"
	"strings"

	deepdocpdf "ragflow/internal/deepdoc/parser/pdf"
	util "ragflow/internal/deepdoc/parser/pdf/util"
	deepdoctype "ragflow/internal/deepdoc/parser/type"
	"ragflow/internal/entity"
	"ragflow/internal/parser/tableutil"
)

// BlockKind identifies one structured block in reading order.
type BlockKind uint8

const (
	BlockText BlockKind = iota
	BlockTable
	BlockImage
)

// Block is one text, table, or image block. Image is the table or figure crop
// for the chunk preview; it is nil for text blocks.
type Block struct {
	Kind  BlockKind
	Text  string
	Table *entity.TableData
	Image image.Image
}

// pageEngine adapts a decoded image to the PDFEngine interface. ExtractChars is
// empty so the page takes the full-OCR path, and RawData is nil so the cgo
// renderer falls back to RenderPageImage instead of rasterising PDF bytes.
type pageEngine struct {
	img image.Image
}

var errNoImagePage = errors.New("image page engine: no image page")

func (e *pageEngine) PageCount() (int, error) { return 1, nil }

func (e *pageEngine) ExtractChars(int) ([]deepdoctype.TextChar, error) { return nil, nil }

func (e *pageEngine) RenderPage(int, float64) ([]byte, error) { return nil, errNoImagePage }

func (e *pageEngine) RenderPageImage(page int, _ float64) (image.Image, error) {
	if page != 0 || e.img == nil {
		return nil, errNoImagePage
	}
	return e.img, nil
}

func (e *pageEngine) RawData() []byte { return nil }

func (e *pageEngine) Outlines() ([]deepdoctype.Outline, error) { return nil, nil }

func (e *pageEngine) Close() error { return nil }

// Parse runs the image through the page pipeline and returns text and table
// blocks in reading order. The analyzer performs OCR, layout recognition, and
// table structure recognition.
func Parse(ctx context.Context, img image.Image, analyzer deepdoctype.DocAnalyzer) ([]Block, error) {
	if img == nil {
		return nil, errors.New("image parse: nil image")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if analyzer == nil || !analyzer.Health() {
		return nil, errors.New("local OCR analyzer is unavailable")
	}

	cfg := deepdoctype.DefaultParserConfig()
	// The image is already the page render. A higher-DPI retry would reuse the
	// same pixels while changing the page's coordinate scale.
	cfg.Zoom = 0

	result, err := deepdocpdf.NewParser(cfg).ParseRaw(ctx, &pageEngine{img: img}, analyzer)
	if err != nil {
		return nil, err
	}
	defer result.Close()
	return blocksFromSections(img, result.Sections), nil
}

// blocksFromSections converts the page pipeline's sections into image blocks.
// Table sections carry table HTML, figure sections carry a crop region, and
// text sections keep their OCR text.
func blocksFromSections(img image.Image, sections []deepdoctype.Section) []Block {
	blocks := make([]Block, 0, len(sections))
	for _, section := range sections {
		switch section.LayoutType {
		case deepdoctype.LayoutTypeTable:
			table, err := tableutil.ParseTableHTML(section.Text)
			if err == nil && table != nil && len(table.Grid) > 0 {
				blocks = append(blocks, Block{
					Kind:  BlockTable,
					Table: table,
					Image: cropSection(img, section.Positions),
				})
				continue
			}
			// A DLA table region can keep its plain OCR text when TSR produced
			// no cells: no table HTML replaces the boxes, but they still carry
			// LayoutType=table. Preserve that text instead of dropping it.
			lower := strings.ToLower(section.Text)
			if strings.Contains(lower, "<table") || strings.Contains(lower, "<tr") {
				continue
			}

		case deepdoctype.LayoutTypeFigure:
			if crop := cropSection(img, section.Positions); crop != nil {
				blocks = append(blocks, Block{
					Kind:  BlockImage,
					Text:  strings.TrimSpace(section.Text),
					Image: crop,
				})
				continue
			}
		}

		if text := strings.TrimSpace(section.Text); text != "" {
			blocks = append(blocks, Block{Kind: BlockText, Text: text})
		}
	}
	return blocks
}

// cropSection crops the first position, which is the table region. Caption
// positions appended by MergeCaptions follow it.
func cropSection(img image.Image, positions []deepdoctype.Position) image.Image {
	if img == nil || len(positions) == 0 {
		return nil
	}
	position := positions[0]
	return cropImage(img,
		position.Left*deepdoctype.DlaScale,
		position.Top*deepdoctype.DlaScale,
		position.Right*deepdoctype.DlaScale,
		position.Bottom*deepdoctype.DlaScale)
}

func cropImage(img image.Image, x0, y0, x1, y1 float64) image.Image {
	bounds := img.Bounds()
	left := clampInt(int(math.Floor(x0)), bounds.Min.X, bounds.Max.X)
	top := clampInt(int(math.Floor(y0)), bounds.Min.Y, bounds.Max.Y)
	right := clampInt(int(math.Ceil(x1)), bounds.Min.X, bounds.Max.X)
	bottom := clampInt(int(math.Ceil(y1)), bounds.Min.Y, bounds.Max.Y)
	if left >= right || top >= bottom {
		return nil
	}
	return util.FastCrop(img, left, top, right, bottom)
}

func clampInt(value, low, high int) int {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}
