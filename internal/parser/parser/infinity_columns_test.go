package parser

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"

	"ragflow/internal/ingestion/task/indexdoc"
	"ragflow/internal/utility"
)

// TestParserItemKeysReachInfinityOnlyAsColumns runs real parser items through
// the index boundary. A key that survives it reaches the Infinity insert, and
// Infinity rejects a column the table does not have (InfinityException 3013):
// every PPTX failed with "Column slide_number not found" (#20500).
func TestParserItemKeysReachInfinityOnlyAsColumns(t *testing.T) {
	budget := func() *embeddedMediaBudget {
		return &embeddedMediaBudget{maxImageBytes: 4, maxTotalBytes: 16, maxItems: 10}
	}
	pptx, err := buildPPTXJSONSections(`{"sections":[{"elements":[
		{"type":"paragraph","content":[{"type":"text","text":"slide text"}]},
		{"type":"image","data":"YWJj"},
		{"type":"image","data":"YWJjZGVm"}
	]}]}`, budget())
	if err != nil {
		t.Fatalf("buildPPTXJSONSections: %v", err)
	}
	docx := buildDOCXJSONSections(`{"sections":[{"elements":[{"type":"table","rows":[{"cells":[
		{"content":[{"type":"image","data":"YWJj"}]},
		{"content":[{"type":"image","data":"YWJjZGVm"}]}
	]}]}]}]}`, budget())

	var picture bytes.Buffer
	if err := png.Encode(&picture, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatalf("encode fixture: %v", err)
	}
	f := excelize.NewFile()
	defer f.Close()
	if err := f.SetCellValue("Sheet1", "A1", "value"); err != nil {
		t.Fatalf("SetCellValue: %v", err)
	}
	if err := f.AddPictureFromBytes("Sheet1", "C3", &excelize.Picture{Extension: ".png", File: picture.Bytes()}); err != nil {
		t.Fatalf("AddPictureFromBytes: %v", err)
	}
	var workbook bytes.Buffer
	if err := f.Write(&workbook); err != nil {
		t.Fatalf("write workbook: %v", err)
	}
	xp, err := NewXLSXParser("")
	if err != nil {
		t.Fatalf("NewXLSXParser: %v", err)
	}
	xlsx := xp.ParseWithResult(t.Context(), "sheet.xlsx", workbook.Bytes())
	if xlsx.Err != nil {
		t.Fatalf("XLSX ParseWithResult: %v", xlsx.Err)
	}

	ep := NewEmailParser()
	ep.ConfigureFromSetup(map[string]any{
		"output_format": "json",
		"fields":        []string{"from", "to", "cc", "bcc", "date", "subject", "body"},
	})
	email := ep.ParseWithResult(t.Context(), "mail.eml", []byte(strings.Join([]string{
		"From: a@example.com",
		"To: b@example.com",
		"Cc: c@example.com",
		"Bcc: d@example.com",
		"Date: Mon, 07 Jul 2025 10:00:00 +0000",
		"Subject: Test Email",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"body",
	}, "\r\n")))
	if email.Err != nil {
		t.Fatalf("email ParseWithResult: %v", email.Err)
	}

	columns := infinityMappingColumns(t)
	for name, items := range map[string][]map[string]any{
		"pptx": pptx, "docx": docx, "xlsx": xlsx.JSON, "email": email.JSON,
	} {
		if len(items) == 0 {
			t.Fatalf("%s: parser emitted no items", name)
		}
		for _, item := range items {
			ck := make(map[string]any, len(item))
			for key, value := range item {
				ck[key] = value
			}
			if _, err := indexdoc.ProcessChunksForPipeline([]map[string]any{ck}, "doc-1", "doc", time.Now()); err != nil {
				t.Fatalf("%s: ProcessChunksForPipeline: %v", name, err)
			}
			for key := range item {
				if _, kept := ck[key]; kept && !columns[key] {
					t.Errorf("%s: parser key %q reaches the Infinity insert but is not a column of conf/infinity_mapping.json", name, key)
				}
			}
		}
	}
}

// infinityMappingColumns reads the column set the chunk table is created from.
func infinityMappingColumns(t *testing.T) map[string]bool {
	t.Helper()
	path, err := utility.FindConfFileInProject("infinity_mapping.json")
	if err != nil || path == nil {
		t.Fatalf("locate infinity_mapping.json: %v", err)
	}
	data, err := os.ReadFile(*path)
	if err != nil {
		t.Fatalf("read %s: %v", *path, err)
	}
	var columns map[string]json.RawMessage
	if err := json.Unmarshal(data, &columns); err != nil {
		t.Fatalf("parse %s: %v", *path, err)
	}
	out := make(map[string]bool, len(columns))
	for name := range columns {
		out[name] = true
	}
	return out
}
