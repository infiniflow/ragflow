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

package parser

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

const xlsxWorkbookXML = "xl/workbook.xml"

// normalizeXLSXForRead rewrites only invalid workbook sheet names so Excelize
// can read otherwise intact sheet data. It leaves every non-workbook ZIP entry
// untouched and returns changed=false when no name needs normalization.
func normalizeXLSXForRead(data []byte) ([]byte, []string, bool, error) {
	src, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, nil, false, fmt.Errorf("open XLSX archive: %w", err)
	}

	var workbook []byte
	for _, file := range src.File {
		if file.Name != xlsxWorkbookXML {
			continue
		}
		r, err := file.Open()
		if err != nil {
			return nil, nil, false, fmt.Errorf("open %s: %w", xlsxWorkbookXML, err)
		}
		workbook, err = io.ReadAll(r)
		closeErr := r.Close()
		if err != nil {
			return nil, nil, false, fmt.Errorf("read %s: %w", xlsxWorkbookXML, err)
		}
		if closeErr != nil {
			return nil, nil, false, fmt.Errorf("close %s: %w", xlsxWorkbookXML, closeErr)
		}
		break
	}
	if workbook == nil {
		return nil, nil, false, fmt.Errorf("XLSX archive is missing %s", xlsxWorkbookXML)
	}

	normalizedWorkbook, warnings, changed, err := normalizeWorkbookSheetNames(workbook)
	if err != nil || !changed {
		return data, warnings, changed, err
	}

	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	for _, file := range src.File {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: file.Name, Method: zip.Deflate})
		if err != nil {
			return nil, nil, false, fmt.Errorf("write XLSX entry %q: %w", file.Name, err)
		}
		if file.Name == xlsxWorkbookXML {
			if _, err := w.Write(normalizedWorkbook); err != nil {
				return nil, nil, false, fmt.Errorf("write %s: %w", xlsxWorkbookXML, err)
			}
			continue
		}
		r, err := file.Open()
		if err != nil {
			return nil, nil, false, fmt.Errorf("open XLSX entry %q: %w", file.Name, err)
		}
		_, copyErr := io.Copy(w, r)
		closeErr := r.Close()
		if copyErr != nil {
			return nil, nil, false, fmt.Errorf("copy XLSX entry %q: %w", file.Name, copyErr)
		}
		if closeErr != nil {
			return nil, nil, false, fmt.Errorf("close XLSX entry %q: %w", file.Name, closeErr)
		}
	}
	if err := zw.Close(); err != nil {
		return nil, nil, false, fmt.Errorf("finish normalized XLSX archive: %w", err)
	}
	return out.Bytes(), warnings, true, nil
}

func normalizeWorkbookSheetNames(workbook []byte) ([]byte, []string, bool, error) {
	dec := xml.NewDecoder(bytes.NewReader(workbook))
	var out bytes.Buffer
	enc := xml.NewEncoder(&out)
	used := make(map[string]struct{})
	changed := false
	var warnings []string

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, false, fmt.Errorf("decode workbook XML: %w", err)
		}
		if start, ok := tok.(xml.StartElement); ok && start.Name.Local == "sheet" {
			for i := range start.Attr {
				if start.Attr[i].Name.Local != "name" {
					continue
				}
				original := start.Attr[i].Value
				normalized := normalizedXLSXSheetName(original, used)
				used[strings.ToLower(normalized)] = struct{}{}
				if normalized != original {
					start.Attr[i].Value = normalized
					changed = true
					warnings = append(warnings, fmt.Sprintf("XLSX normalized sheet name %q to %q", original, normalized))
				}
			}
			tok = start
		}
		if err := enc.EncodeToken(tok); err != nil {
			return nil, nil, false, fmt.Errorf("encode workbook XML: %w", err)
		}
	}
	if err := enc.Flush(); err != nil {
		return nil, nil, false, fmt.Errorf("finish workbook XML: %w", err)
	}
	return out.Bytes(), warnings, changed, nil
}

func normalizedXLSXSheetName(name string, used map[string]struct{}) string {
	var b strings.Builder
	for _, r := range strings.Trim(name, "'") {
		switch r {
		case ':', '\\', '/', '?', '*', '[', ']':
			b.WriteByte('_')
		default:
			b.WriteRune(r)
		}
	}
	base := truncateSheetName(b.String(), 31)
	if base == "" {
		base = "Sheet"
	}
	name = base
	for suffix := 2; ; suffix++ {
		if _, exists := used[strings.ToLower(name)]; !exists {
			return name
		}
		tail := fmt.Sprintf("_%d", suffix)
		name = truncateSheetName(base, 31-len([]rune(tail))) + tail
	}
}

func truncateSheetName(name string, max int) string {
	runes := []rune(name)
	if len(runes) <= max {
		return name
	}
	return string(runes[:max])
}
