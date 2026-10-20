package models

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strings"

	"ragflow/internal/utility"
)

type paddleOCREntry struct {
	LogID     string          `json:"logId"`
	ErrorCode int             `json:"errorCode"`
	ErrorMsg  string          `json:"errorMsg"`
	Result    paddleOCRResult `json:"result"`
}

type paddleOCRPrunedResult struct {
	PageIndex *int    `json:"page_index"`
	Width     float64 `json:"width"`
	Height    float64 `json:"height"`
	Blocks    []struct {
		Text  string          `json:"block_content"`
		Label string          `json:"block_label"`
		BBox  json.RawMessage `json:"block_bbox"`
	} `json:"parsing_res_list"`
	RecTexts []string          `json:"rec_texts"`
	RecBoxes []json.RawMessage `json:"rec_boxes"`
}

type paddleOCRResult struct {
	LayoutParsingResults []struct {
		Markdown struct {
			Text string `json:"text"`
		} `json:"markdown"`
		PrunedResult paddleOCRPrunedResult `json:"prunedResult"`
	} `json:"layoutParsingResults"`
	OcrResults []struct {
		PrunedResult paddleOCRPrunedResult `json:"prunedResult"`
	} `json:"ocrResults"`
	DataInfo struct {
		Pages []struct {
			Width  float64 `json:"width"`
			Height float64 `json:"height"`
		} `json:"pages"`
	} `json:"dataInfo"`
}

// parsePaddleOCRResultBody accepts JSON arrays, objects and JSONL. Rejecting
// malformed JSON prevents inferred page numbers from shifting silently. Failed
// entries are omitted only while subsequent source page identity stays known.
func parsePaddleOCRResultBody(body []byte) (*OCRFileResponse, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	var entries []paddleOCREntry
	for {
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err == io.EOF {
			break
		} else if err != nil {
			return nil, fmt.Errorf("paddleocr result: invalid JSON after byte %d: %w", decoder.InputOffset(), err)
		}
		if len(raw) > 0 && raw[0] == '[' {
			var group []paddleOCREntry
			if err := json.Unmarshal(raw, &group); err != nil {
				return nil, fmt.Errorf("paddleocr result: invalid entries: %w", err)
			}
			entries = append(entries, group...)
		} else {
			var entry paddleOCREntry
			if err := json.Unmarshal(raw, &entry); err != nil {
				return nil, fmt.Errorf("paddleocr result: invalid entry: %w", err)
			}
			entries = append(entries, entry)
		}
	}
	response := &OCRFileResponse{}
	var text strings.Builder
	nextPage := 0
	pageIdentityKnown := true
	for i, entry := range entries {
		pages := paddleOCRPages(entry.Result, nextPage)
		var firstIndex *int
		if len(entry.Result.LayoutParsingResults) > 0 {
			firstIndex = entry.Result.LayoutParsingResults[0].PrunedResult.PageIndex
		} else if len(entry.Result.OcrResults) > 0 {
			firstIndex = entry.Result.OcrResults[0].PrunedResult.PageIndex
		}
		if firstIndex != nil && *firstIndex >= 0 {
			pageIdentityKnown = true
		}
		if pageIdentityKnown {
			for _, page := range pages {
				if page.Index < nextPage {
					return nil, fmt.Errorf("paddleocr result: entry %d (log_id=%q): source page index %d repeats or precedes an earlier page (next index %d)", i+1, entry.LogID, page.Index, nextPage)
				}
				nextPage = page.Index + 1
			}
		}
		if entry.ErrorCode != 0 || len(pages) == 0 {
			warning := fmt.Sprintf("paddleocr result: entry %d (log_id=%q)", i+1, entry.LogID)
			if len(pages) > 0 && pageIdentityKnown {
				warning += fmt.Sprintf(" (source pages %d..%d)", pages[0].Index+1, nextPage)
			}
			if entry.ErrorCode != 0 {
				warning += fmt.Sprintf(": errorCode %d: %s", entry.ErrorCode, entry.ErrorMsg)
			} else {
				warning += ": contains no pages"
			}
			response.Warnings = append(response.Warnings, warning)
			if len(pages) == 0 {
				pageIdentityKnown = false
			}
			continue
		}
		if !pageIdentityKnown {
			return nil, fmt.Errorf("paddleocr result: entry %d (log_id=%q): source page identity unknown after failed entry", i+1, entry.LogID)
		}
		response.Pages = append(response.Pages, pages...)
		text.WriteString(paddleOCRText(entry.Result))
		text.WriteString("\n\n")
	}
	content := strings.TrimSpace(text.String())
	if content == "" {
		if len(response.Warnings) > 0 {
			return nil, fmt.Errorf("paddleocr result: no usable text; %s", response.Warnings[0])
		}
		return nil, fmt.Errorf("paddleocr result: parsed empty text")
	}
	response.Text = &content
	return response, nil
}

func paddleOCRPages(result paddleOCRResult, nextPage int) []OCRPage {
	var pages []OCRPage
	appendPage := func(pruned paddleOCRPrunedResult, markdown string, ordinal int) {
		page := OCRPage{Index: nextPage, Width: pruned.Width, Height: pruned.Height}
		if pruned.PageIndex != nil && *pruned.PageIndex >= 0 {
			page.Index = *pruned.PageIndex
		}
		// dataInfo pages are ordinal within this result. An explicit source
		// index differing from that ordinal makes this fallback ambiguous.
		if ordinal < len(result.DataInfo.Pages) && (pruned.PageIndex == nil || *pruned.PageIndex == ordinal) {
			size := result.DataInfo.Pages[ordinal]
			if page.Width <= 0 {
				page.Width = size.Width
			}
			if page.Height <= 0 {
				page.Height = size.Height
			}
		}
		page.Blocks = pruned.blocks()
		hasText := false
		for _, block := range page.Blocks {
			if strings.TrimSpace(utility.StripMarkdownImages(block.Text)) != "" {
				hasText = true
				break
			}
		}
		if !hasText && strings.TrimSpace(markdown) != "" {
			page.Blocks = []OCRBlock{{Text: markdown}}
		}
		pages = append(pages, page)
		nextPage = page.Index + 1
	}
	for i, layout := range result.LayoutParsingResults {
		appendPage(layout.PrunedResult, layout.Markdown.Text, i)
	}
	if len(result.LayoutParsingResults) == 0 {
		for i, ocr := range result.OcrResults {
			appendPage(ocr.PrunedResult, "", i)
		}
	}
	return pages
}

func (pruned paddleOCRPrunedResult) blocks() []OCRBlock {
	var blocks []OCRBlock
	for _, raw := range pruned.Blocks {
		// Match PaddleX PaddleOCR-VL's default markdown_ignore_labels for
		// structured output: paddlex/configs/pipelines/PaddleOCR-VL.yaml (release/3.7).
		switch strings.ToLower(strings.TrimSpace(raw.Label)) {
		case "header", "header_image", "footer", "footer_image", "number", "aside_text", "footnote":
			continue
		}
		blocks = append(blocks, OCRBlock{Text: raw.Text, Label: raw.Label, BBox: paddleOCRBox(raw.BBox)})
	}
	if len(pruned.Blocks) == 0 {
		for i, text := range pruned.RecTexts {
			var box []float64
			if i < len(pruned.RecBoxes) {
				box = paddleOCRBox(pruned.RecBoxes[i])
			}
			blocks = append(blocks, OCRBlock{Text: text, Label: "text", BBox: box})
		}
	}
	return blocks
}

func paddleOCRBox(raw json.RawMessage) []float64 {
	var coordinates []*float64
	if err := json.Unmarshal(raw, &coordinates); err != nil {
		return nil
	}
	if len(coordinates) != 4 && len(coordinates) != 8 {
		return nil
	}
	box := make([]float64, len(coordinates))
	for i, value := range coordinates {
		if value == nil || math.IsNaN(*value) || math.IsInf(*value, 0) {
			return nil
		}
		box[i] = *value
	}
	if len(box) == 8 {
		left, top, right, bottom := box[0], box[1], box[0], box[1]
		for i := 2; i < len(box); i += 2 {
			left = math.Min(left, box[i])
			right = math.Max(right, box[i])
			top = math.Min(top, box[i+1])
			bottom = math.Max(bottom, box[i+1])
		}
		box = []float64{left, top, right, bottom}
	}
	if box[2] <= box[0] || box[3] <= box[1] {
		return nil
	}
	return box
}

// paddleOCRText projects the service text for image OCR consumers. PDF parsing
// uses the independently preserved blocks and page geometry.
func paddleOCRText(result paddleOCRResult) string {
	var text strings.Builder
	appendTexts := func(pruned paddleOCRPrunedResult) {
		for _, block := range pruned.Blocks {
			text.WriteString(block.Text)
			text.WriteString("\n")
		}
		if len(pruned.Blocks) == 0 {
			for _, line := range pruned.RecTexts {
				text.WriteString(line)
				text.WriteString("\n")
			}
		}
	}
	for _, layout := range result.LayoutParsingResults {
		if strings.TrimSpace(layout.Markdown.Text) != "" {
			text.WriteString(layout.Markdown.Text)
		} else {
			appendTexts(layout.PrunedResult)
		}
		text.WriteString("\n\n")
	}
	if len(result.LayoutParsingResults) == 0 {
		for _, ocr := range result.OcrResults {
			appendTexts(ocr.PrunedResult)
		}
	}
	return strings.TrimSpace(text.String())
}
