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
	"strings"
)

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

// extractDOCXTableCaptions re-parses word/document.xml to recover table
// captions. It returns one entry per table in document-body reading order: the
// trimmed text of the paragraph immediately above the table when that
// paragraph carries a caption style, otherwise "".
//
// It is intentionally limited to the common case: paragraphs and tables that
// are direct children of w:body, in order. Nested tables (e.g. inside a text
// box) are ignored, matching how office_oxide flattens the typical document.
// Errors are returned to the caller, which falls back to no captions.
//
// TEMPORARY: see the package note at the top of this file.
func extractDOCXTableCaptions(docx []byte) ([]string, error) {
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
	var captions []string
	var pendingCaption string

	var inBody bool
	var bodyDepth int // depth relative to <w:body>; 1 == direct child of body
	var inPara bool
	var paraIsCaption bool
	var inText bool
	var paraText strings.Builder

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
				captions = append(captions, pendingCaption)
				pendingCaption = ""
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
	return captions, nil
}
