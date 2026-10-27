// Package-level note: this file is a TEMPORARY stopgap, not a permanent fix.
//
// Problem it works around: office_oxide's IR JSON (the structured output the
// Go DOCX parser builds sections from) does NOT expose paragraph style names
// for non-heading paragraphs. It resolves Word heading styles into
// type "heading" + level, but a paragraph styled with the built-in "Caption"
// style (or the Chinese "题注") is emitted as a plain {"type":"paragraph"}
// with no style field. As a result the Go parser cannot tell a caption
// paragraph from ordinary body text, so a table's visible caption is lost
// (see the user-reported case "设备流转明细").
//
// The correct, robust fix lives upstream: teach office_oxide to emit the
// paragraph style name so this detection is unnecessary. That is tracked in
// yfedoseev/office_oxide#455 ("Expose paragraph style name (w:pStyle) in the
// IR JSON for non-heading paragraphs").
//
// Until that lands and office_oxide is upgraded in this repo, we recover the
// caption the cheap way: re-parse word/document.xml ourselves and pair a
// caption-styled paragraph with the table that immediately follows it. This
// MUST be removed once office_oxide emits paragraph styles — do not extend it,
// do not rely on it from new code. The position-based pairing (document-body
// table order vs office_oxide IR table order) is approximate and documented at
// buildDOCXJSONSectionsCore.
package parser

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// docxTableMeta holds per-table metadata recovered from word/document.xml
// that office_oxide does not yet emit in its IR JSON: the preceding caption
// paragraph text and the w:tblLook firstRow conditional formatting flag.
type docxTableMeta struct {
	caption  string
	firstRow *bool
}

func boolPtr(v bool) *bool {
	return &v
}

// docxCaptionStyleNames are the Word paragraph style identifiers that mark a
// paragraph as a table caption for the temporary detection above. The built-in
// caption style's styleId is "Caption" in every locale; "题注" covers documents
// where a caption style was created with the localized display name.
var docxCaptionStyleNames = map[string]bool{
	"caption": true,
	"题注":      true,
}

func isDOCXCaptionStyle(styleID string) bool {
	return docxCaptionStyleNames[strings.ToLower(strings.TrimSpace(styleID))]
}

// parseDOCXTblLookFirstRow extracts the first-row header conditional formatting
// flag from <w:tblLook>. Explicit w:firstRow ("1"/"0"/"true"/"false"/"on"/"off")
// takes precedence over the legacy w:val bitmask (bit 0x0020).
func parseDOCXTblLookFirstRow(attrs []xml.Attr) *bool {
	for _, a := range attrs {
		if a.Name.Local == "firstRow" {
			switch strings.ToLower(strings.TrimSpace(a.Value)) {
			case "1", "true", "on":
				return boolPtr(true)
			case "0", "false", "off":
				return boolPtr(false)
			}
		}
	}
	for _, a := range attrs {
		if a.Name.Local == "val" {
			if mask, err := strconv.ParseUint(strings.TrimSpace(a.Value), 16, 16); err == nil {
				return boolPtr((mask & 0x0020) != 0)
			}
		}
	}
	return nil
}

// extractDOCXTableMeta re-parses word/document.xml to recover per-table
// caption and w:tblLook firstRow metadata in document-body reading order: the
// trimmed text of the paragraph immediately above the table when that
// paragraph carries a caption style (otherwise ""), and the table's w:tblLook
// firstRow flag when present.
//
// It is intentionally limited to the common case: paragraphs and tables that
// are direct children of w:body, in order. Nested tables (e.g. inside a text
// box) are ignored, matching how office_oxide flattens the typical document.
// Errors are returned to the caller, which falls back to no metadata.
//
// TEMPORARY: see the package note at the top of this file.
func extractDOCXTableMeta(docx []byte) ([]docxTableMeta, error) {
	zr, err := zip.NewReader(bytes.NewReader(docx), int64(len(docx)))
	if err != nil {
		return nil, fmt.Errorf("docx caption scan: open zip: %w", err)
	}
	var bodyXML []byte
	for _, f := range zr.File {
		if f.Name == "word/document.xml" {
			rc, err := f.Open()
			if err != nil {
				return nil, fmt.Errorf("docx caption scan: open document.xml: %w", err)
			}
			bodyXML, err = io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return nil, fmt.Errorf("docx caption scan: read document.xml: %w", err)
			}
			break
		}
	}
	if bodyXML == nil {
		return nil, fmt.Errorf("docx caption scan: word/document.xml not found")
	}

	dec := xml.NewDecoder(bytes.NewReader(bodyXML))
	var tables []docxTableMeta
	var pendingCaption string

	var inBody bool
	var bodyDepth int // depth relative to <w:body>; 1 == direct child of body
	var inPara bool
	var paraIsCaption bool
	var inText bool
	var paraText strings.Builder
	var inTblPr bool

	flushPara := func() {
		if paraIsCaption {
			pendingCaption = strings.TrimSpace(paraText.String())
		} else {
			// Any non-caption paragraph clears a caption that was pending
			// for the next table, so only an *immediately* preceding caption
			// is attached.
			pendingCaption = ""
		}
		inPara = false
		paraIsCaption = false
		inText = false
		paraText.Reset()
	}

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("docx caption scan: xml: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			local := t.Name.Local
			if !inBody {
				if local == "body" {
					inBody = true
					bodyDepth = 0
				}
				continue
			}
			bodyDepth++
			switch {
			case bodyDepth == 1 && local == "p":
				inPara = true
				paraIsCaption = false
				inText = false
				paraText.Reset()
			case bodyDepth == 1 && local == "tbl":
				tables = append(tables, docxTableMeta{caption: pendingCaption})
				pendingCaption = ""
				inTblPr = false
			case bodyDepth == 2 && local == "tblPr" && len(tables) > 0:
				inTblPr = true
			case inTblPr && bodyDepth == 3 && local == "tblLook" && len(tables) > 0:
				tables[len(tables)-1].firstRow = parseDOCXTblLookFirstRow(t.Attr)
			case inPara && local == "pStyle":
				for _, a := range t.Attr {
					if a.Name.Local == "val" {
						paraIsCaption = isDOCXCaptionStyle(a.Value)
					}
				}
			case inPara && local == "t":
				inText = true
			}
		case xml.CharData:
			if inPara && inText {
				paraText.Write(t)
			}
		case xml.EndElement:
			local := t.Name.Local
			if !inBody {
				continue
			}
			switch {
			case bodyDepth == 1 && local == "p" && inPara:
				flushPara()
			case bodyDepth == 2 && local == "tblPr":
				inTblPr = false
			case inPara && local == "t":
				inText = false
			case inPara && local == "pStyle":
				// nothing to reset; paraIsCaption stays until paragraph end
			}
			bodyDepth--
			if local == "body" {
				inBody = false
			}
		}
	}
	// A trailing caption paragraph with no following table is discarded.
	return tables, nil
}
