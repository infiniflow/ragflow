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
	"unicode/utf16"
)

const (
	xlsxWorkbookXML       = "xl/workbook.xml"
	maxXLSXSheetNameUnits = 31
)

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
	var out bytes.Buffer
	used := make([]string, 0)
	changed := false
	var warnings []string

	lastWrite := 0
	for offset := 0; offset < len(workbook); {
		start := bytes.IndexByte(workbook[offset:], '<')
		if start < 0 {
			break
		}
		start += offset
		if end, skipped, err := xmlNonElementEnd(workbook, start); err != nil {
			return nil, nil, false, err
		} else if skipped {
			offset = end + 1
			continue
		}
		end, err := xmlStartTagEnd(workbook, start)
		if err != nil {
			return nil, nil, false, err
		}
		tag := workbook[start : end+1]
		if !isSheetStartTag(tag) {
			offset = end + 1
			continue
		}

		valueStart, valueEnd, quote, found, err := sheetNameAttributeBounds(tag)
		if err != nil {
			return nil, nil, false, err
		}
		if !found {
			offset = end + 1
			continue
		}
		original, err := decodeXMLAttributeValue(tag[valueStart:valueEnd], quote)
		if err != nil {
			return nil, nil, false, fmt.Errorf("decode XLSX sheet name: %w", err)
		}
		normalized := normalizedXLSXSheetName(original, used)
		used = append(used, normalized)
		if normalized != original {
			out.Write(workbook[lastWrite:start])
			out.Write(tag[:valueStart])
			out.WriteString(encodeXMLAttributeValue(normalized, quote))
			out.Write(tag[valueEnd:])
			lastWrite = end + 1
			offset = end + 1
			changed = true
			warnings = append(warnings, fmt.Sprintf("XLSX normalized sheet name %q to %q", original, normalized))
			continue
		}
		offset = end + 1
	}
	if !changed {
		return workbook, warnings, false, nil
	}
	out.Write(workbook[lastWrite:])
	return out.Bytes(), warnings, true, nil
}

func xmlNonElementEnd(data []byte, start int) (int, bool, error) {
	for _, marker := range []struct {
		prefix []byte
		suffix []byte
	}{
		{[]byte("<!--"), []byte("-->")},
		{[]byte("<![CDATA["), []byte("]]>")},
		{[]byte("<?"), []byte("?>")},
	} {
		if !bytes.HasPrefix(data[start:], marker.prefix) {
			continue
		}
		end := bytes.Index(data[start+len(marker.prefix):], marker.suffix)
		if end < 0 {
			return 0, false, fmt.Errorf("scan workbook XML: unterminated markup at byte %d", start)
		}
		return start + len(marker.prefix) + end + len(marker.suffix) - 1, true, nil
	}
	return 0, false, nil
}

func xmlStartTagEnd(data []byte, start int) (int, error) {
	var quote byte
	for i := start + 1; i < len(data); i++ {
		switch data[i] {
		case '\'', '"':
			if quote == 0 {
				quote = data[i]
			} else if quote == data[i] {
				quote = 0
			}
		case '>':
			if quote == 0 {
				return i, nil
			}
		}
	}
	return 0, fmt.Errorf("scan workbook XML: unterminated tag at byte %d", start)
}

func isSheetStartTag(tag []byte) bool {
	if len(tag) < len("<sheet") || tag[0] != '<' {
		return false
	}
	i := 1
	for i < len(tag) && !isXMLSpace(tag[i]) && tag[i] != '/' && tag[i] != '>' {
		i++
	}
	name := tag[1:i]
	if colon := bytes.LastIndexByte(name, ':'); colon >= 0 {
		name = name[colon+1:]
	}
	return bytes.Equal(name, []byte("sheet"))
}

func sheetNameAttributeBounds(tag []byte) (int, int, byte, bool, error) {
	i := 1
	for i < len(tag) && !isXMLSpace(tag[i]) && tag[i] != '/' && tag[i] != '>' {
		i++
	}
	for i < len(tag) {
		for i < len(tag) && isXMLSpace(tag[i]) {
			i++
		}
		if i >= len(tag) || tag[i] == '/' || tag[i] == '>' {
			return 0, 0, 0, false, nil
		}
		nameStart := i
		for i < len(tag) && !isXMLSpace(tag[i]) && tag[i] != '=' && tag[i] != '/' && tag[i] != '>' {
			i++
		}
		name := tag[nameStart:i]
		for i < len(tag) && isXMLSpace(tag[i]) {
			i++
		}
		if i >= len(tag) || tag[i] != '=' {
			return 0, 0, 0, false, fmt.Errorf("scan workbook XML: invalid sheet attribute %q", name)
		}
		i++
		for i < len(tag) && isXMLSpace(tag[i]) {
			i++
		}
		if i >= len(tag) || (tag[i] != '\'' && tag[i] != '"') {
			return 0, 0, 0, false, fmt.Errorf("scan workbook XML: unquoted sheet attribute %q", name)
		}
		quote := tag[i]
		i++
		valueStart := i
		for i < len(tag) && tag[i] != quote {
			i++
		}
		if i >= len(tag) {
			return 0, 0, 0, false, fmt.Errorf("scan workbook XML: unterminated sheet attribute %q", name)
		}
		valueEnd := i
		i++
		if xmlLocalName(name) == "name" {
			return valueStart, valueEnd, quote, true, nil
		}
	}
	return 0, 0, 0, false, nil
}

func decodeXMLAttributeValue(value []byte, quote byte) (string, error) {
	var attr struct {
		Value string `xml:"value,attr"`
	}
	encoded := make([]byte, 0, len(value)+20)
	encoded = append(encoded, "<sheet value="...)
	encoded = append(encoded, quote)
	encoded = append(encoded, value...)
	encoded = append(encoded, quote, '/', '>')
	if err := xml.Unmarshal(encoded, &attr); err != nil {
		return "", err
	}
	return attr.Value, nil
}

func encodeXMLAttributeValue(value string, quote byte) string {
	var out strings.Builder
	for _, r := range value {
		switch r {
		case '&':
			out.WriteString("&amp;")
		case '<':
			out.WriteString("&lt;")
		case '>':
			out.WriteString("&gt;")
		case '\'':
			if quote == '\'' {
				out.WriteString("&apos;")
			} else {
				out.WriteRune(r)
			}
		case '"':
			if quote == '"' {
				out.WriteString("&quot;")
			} else {
				out.WriteRune(r)
			}
		case '\t':
			out.WriteString("&#x9;")
		case '\n':
			out.WriteString("&#xA;")
		case '\r':
			out.WriteString("&#xD;")
		default:
			out.WriteRune(r)
		}
	}
	return out.String()
}

func isXMLSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}

func xmlLocalName(name []byte) string {
	if colon := bytes.LastIndexByte(name, ':'); colon >= 0 {
		name = name[colon+1:]
	}
	return string(name)
}

func normalizedXLSXSheetName(name string, used []string) string {
	var b strings.Builder
	for _, r := range strings.Trim(name, "'") {
		switch r {
		case ':', '\\', '/', '?', '*', '[', ']':
			b.WriteByte('_')
		default:
			b.WriteRune(r)
		}
	}
	base := truncateSheetName(b.String(), maxXLSXSheetNameUnits)
	if base == "" {
		base = "Sheet"
	}
	name = base
	for suffix := 2; ; suffix++ {
		if !containsEqualFold(used, name) {
			return name
		}
		tail := fmt.Sprintf("_%d", suffix)
		name = truncateSheetName(base, maxXLSXSheetNameUnits-utf16UnitCount(tail)) + tail
	}
}

func containsEqualFold(names []string, name string) bool {
	for _, candidate := range names {
		if strings.EqualFold(candidate, name) {
			return true
		}
	}
	return false
}

func truncateSheetName(name string, maxUnits int) string {
	units := 0
	for i, r := range name {
		width := utf16.RuneLen(r)
		if units+width > maxUnits {
			return name[:i]
		}
		units += width
	}
	return name
}

func utf16UnitCount(name string) int {
	units := 0
	for _, r := range name {
		units += utf16.RuneLen(r)
	}
	return units
}
