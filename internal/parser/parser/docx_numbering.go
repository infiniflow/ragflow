//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package parser

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"strconv"
	"strings"
)

type docxNumberedHeading struct {
	Text         string
	NumberedText string
	Level        int
}

type docxXMLValue struct {
	Value string `xml:"val,attr"`
}

type docxXMLNumberingRef struct {
	Level *docxXMLValue `xml:"ilvl"`
	ID    *docxXMLValue `xml:"numId"`
}

type docxXMLParagraphProperties struct {
	Style        *docxXMLValue        `xml:"pStyle"`
	NumberingRef *docxXMLNumberingRef `xml:"numPr"`
	OutlineLevel *docxXMLValue        `xml:"outlineLvl"`
}

type docxXMLParagraph struct {
	Properties *docxXMLParagraphProperties `xml:"pPr"`
	InnerXML   []byte                      `xml:",innerxml"`
}

type docxXMLStyle struct {
	ID         string                      `xml:"styleId,attr"`
	Name       *docxXMLValue               `xml:"name"`
	BasedOn    *docxXMLValue               `xml:"basedOn"`
	Properties *docxXMLParagraphProperties `xml:"pPr"`
}

type docxXMLStyles struct {
	Styles []docxXMLStyle `xml:"style"`
}

type docxXMLNumberingLevel struct {
	Level          int           `xml:"ilvl,attr"`
	Start          *docxXMLValue `xml:"start"`
	Format         *docxXMLValue `xml:"numFmt"`
	Text           *docxXMLValue `xml:"lvlText"`
	ParagraphStyle *docxXMLValue `xml:"pStyle"`
	RestartLevel   *docxXMLValue `xml:"lvlRestart"`
}

type docxXMLAbstractNumbering struct {
	ID     int                     `xml:"abstractNumId,attr"`
	Levels []docxXMLNumberingLevel `xml:"lvl"`
}

type docxXMLLevelOverride struct {
	Level         int                    `xml:"ilvl,attr"`
	StartOverride *docxXMLValue          `xml:"startOverride"`
	LevelDef      *docxXMLNumberingLevel `xml:"lvl"`
}

type docxXMLNumberingInstance struct {
	ID         int                    `xml:"numId,attr"`
	AbstractID *docxXMLValue          `xml:"abstractNumId"`
	Overrides  []docxXMLLevelOverride `xml:"lvlOverride"`
}

type docxXMLNumbering struct {
	Abstracts []docxXMLAbstractNumbering `xml:"abstractNum"`
	Instances []docxXMLNumberingInstance `xml:"num"`
}

type docxNumberingLevel struct {
	Start          int
	Format         string
	Text           string
	ParagraphStyle string
	RestartLevel   *int
}

type docxNumberingInstance struct {
	AbstractID int
	Overrides  map[int]docxNumberingLevel
}

type docxNumberingDefinitions struct {
	Abstracts map[int]map[int]docxNumberingLevel
	Instances map[int]docxNumberingInstance
	Order     []int
}

type docxParagraphNumbering struct {
	ID    int
	Level int
}

type docxResolvedStyle struct {
	Name              string
	NumberingRef      *docxParagraphNumbering
	NumberingDisabled bool
	OutlineLevel      *int
}

// extractDOCXNumberedHeadingsIfEnabled avoids parsing numbering definitions
// when automatic numbering extraction is disabled in the DOCX setup.
func extractDOCXNumberedHeadingsIfEnabled(data []byte, enabled bool) []docxNumberedHeading {
	if !enabled {
		return nil
	}
	return extractDOCXNumberedHeadings(data)
}

// extractDOCXNumberedHeadings performs best-effort materialization of Word's
// display-only heading numbers. Word stores paragraph text in document.xml and
// the visible marker separately in numbering.xml, so office_oxide's text runs
// cannot contain it.
func extractDOCXNumberedHeadings(data []byte) []docxNumberedHeading {
	parts := readDOCXXMLParts(data, "word/document.xml", "word/styles.xml", "word/numbering.xml")
	if len(parts["word/document.xml"]) == 0 || len(parts["word/numbering.xml"]) == 0 {
		return nil
	}

	paragraphs := parseDOCXParagraphs(parts["word/document.xml"])
	styles := parseDOCXStyles(parts["word/styles.xml"])
	numbering := parseDOCXNumbering(parts["word/numbering.xml"])
	if len(paragraphs) == 0 || len(numbering.Instances) == 0 {
		return nil
	}

	counters := make(map[int][]int)
	resolvedStyles := make(map[string]docxResolvedStyle)
	var headings []docxNumberedHeading
	for _, paragraph := range paragraphs {
		styleID := ""
		if paragraph.Properties != nil && paragraph.Properties.Style != nil {
			styleID = paragraph.Properties.Style.Value
		}
		style, ok := resolvedStyles[styleID]
		if !ok {
			style = resolveDOCXStyle(styleID, styles, nil)
			resolvedStyles[styleID] = style
		}
		text := paragraphText(paragraph)
		headingLevel, isHeading := docxHeadingLevel(styleID, style, paragraph.Properties)
		if !isHeading || strings.TrimSpace(text) == "" {
			continue
		}
		ref := paragraphNumberingRef(paragraph.Properties)
		numberingDisabled := isParagraphNumberingDisabled(paragraph.Properties)
		if ref == nil && !numberingDisabled {
			ref = style.NumberingRef
			numberingDisabled = style.NumberingDisabled
		}
		if ref == nil && !numberingDisabled {
			ref = numberingRefForParagraphStyle(styleID, numbering)
		}
		if ref == nil {
			continue
		}

		level, ok := numbering.resolveLevel(ref.ID, ref.Level)
		if !ok || level.Format == "bullet" || level.Format == "none" || level.Text == "" {
			continue
		}
		values := counters[ref.ID]
		if len(values) < 9 {
			values = make([]int, 9)
		}
		if values[ref.Level] == 0 {
			values[ref.Level] = level.Start
		} else {
			values[ref.Level]++
		}
		resetDOCXLowerLevelCounters(values, ref.Level, ref.ID, numbering)
		counters[ref.ID] = values

		marker := renderDOCXNumberingMarker(level.Text, ref.ID, values, numbering)
		marker = strings.TrimSpace(marker)
		if marker == "" {
			continue
		}
		numberedText := strings.TrimSpace(text)
		if !hasDOCXNumberingMarker(numberedText, marker) {
			numberedText = marker + " " + numberedText
		}
		headings = append(headings, docxNumberedHeading{
			Text:         strings.TrimSpace(text),
			NumberedText: numberedText,
			Level:        headingLevel,
		})
	}
	return headings
}

func hasDOCXNumberingMarker(text, marker string) bool {
	return text == marker || strings.HasPrefix(text, marker+" ")
}

func readDOCXXMLParts(data []byte, names ...string) map[string][]byte {
	wanted := make(map[string]bool, len(names))
	for _, name := range names {
		wanted[name] = true
	}
	parts := make(map[string][]byte, len(names))
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return parts
	}
	for _, file := range zr.File {
		if !wanted[file.Name] {
			continue
		}
		r, err := file.Open()
		if err != nil {
			continue
		}
		content, readErr := io.ReadAll(r)
		closeErr := r.Close()
		if readErr == nil && closeErr == nil {
			parts[file.Name] = content
		}
	}
	return parts
}

func parseDOCXParagraphs(data []byte) []docxXMLParagraph {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var paragraphs []docxXMLParagraph
	for {
		token, err := decoder.Token()
		if err != nil {
			break
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "p" {
			continue
		}
		var paragraph docxXMLParagraph
		if err := decoder.DecodeElement(&paragraph, &start); err != nil {
			break
		}
		paragraphs = append(paragraphs, paragraph)
	}
	return paragraphs
}

func parseDOCXStyles(data []byte) map[string]docxXMLStyle {
	var document docxXMLStyles
	_ = xml.Unmarshal(data, &document)
	styles := make(map[string]docxXMLStyle, len(document.Styles))
	for _, style := range document.Styles {
		styles[style.ID] = style
	}
	return styles
}

func parseDOCXNumbering(data []byte) docxNumberingDefinitions {
	var document docxXMLNumbering
	_ = xml.Unmarshal(data, &document)
	definitions := docxNumberingDefinitions{
		Abstracts: make(map[int]map[int]docxNumberingLevel),
		Instances: make(map[int]docxNumberingInstance),
	}
	for _, abstract := range document.Abstracts {
		levels := make(map[int]docxNumberingLevel, len(abstract.Levels))
		for _, level := range abstract.Levels {
			levels[level.Level] = convertDOCXNumberingLevel(level)
		}
		definitions.Abstracts[abstract.ID] = levels
	}
	for _, instance := range document.Instances {
		abstractID, ok := parseDOCXXMLInt(instance.AbstractID)
		if !ok {
			continue
		}
		resolved := docxNumberingInstance{AbstractID: abstractID, Overrides: make(map[int]docxNumberingLevel)}
		for _, override := range instance.Overrides {
			level, exists := definitions.Abstracts[abstractID][override.Level]
			if override.LevelDef != nil {
				overriddenLevel := convertDOCXNumberingLevel(*override.LevelDef)
				// Word ignores lvlRestart inside a level override.
				overriddenLevel.RestartLevel = level.RestartLevel
				level = overriddenLevel
				exists = true
			}
			if !exists {
				continue
			}
			if start, ok := parseDOCXXMLInt(override.StartOverride); ok {
				level.Start = start
			}
			resolved.Overrides[override.Level] = level
		}
		definitions.Instances[instance.ID] = resolved
		definitions.Order = append(definitions.Order, instance.ID)
	}
	return definitions
}

func convertDOCXNumberingLevel(level docxXMLNumberingLevel) docxNumberingLevel {
	start, ok := parseDOCXXMLInt(level.Start)
	if !ok {
		start = 1
	}
	result := docxNumberingLevel{Start: start}
	if level.Format != nil {
		result.Format = level.Format.Value
	}
	if level.Text != nil {
		result.Text = level.Text.Value
	}
	if level.ParagraphStyle != nil {
		result.ParagraphStyle = level.ParagraphStyle.Value
	}
	if restartLevel, ok := parseDOCXXMLInt(level.RestartLevel); ok && restartLevel >= 0 {
		result.RestartLevel = &restartLevel
	}
	return result
}

func (definitions docxNumberingDefinitions) resolveLevel(id, level int) (docxNumberingLevel, bool) {
	if level < 0 || level >= 9 {
		return docxNumberingLevel{}, false
	}
	instance, ok := definitions.Instances[id]
	if !ok {
		return docxNumberingLevel{}, false
	}
	if override, ok := instance.Overrides[level]; ok {
		return override, true
	}
	resolved, ok := definitions.Abstracts[instance.AbstractID][level]
	return resolved, ok
}

func paragraphNumberingRef(properties *docxXMLParagraphProperties) *docxParagraphNumbering {
	if properties == nil || properties.NumberingRef == nil {
		return nil
	}
	id, ok := parseDOCXXMLInt(properties.NumberingRef.ID)
	if !ok || id == 0 {
		return nil
	}
	level, _ := parseDOCXXMLInt(properties.NumberingRef.Level)
	return &docxParagraphNumbering{ID: id, Level: level}
}

func isParagraphNumberingDisabled(properties *docxXMLParagraphProperties) bool {
	if properties == nil || properties.NumberingRef == nil {
		return false
	}
	id, ok := parseDOCXXMLInt(properties.NumberingRef.ID)
	return ok && id == 0
}

func resolveDOCXStyle(id string, styles map[string]docxXMLStyle, seen map[string]bool) docxResolvedStyle {
	if id == "" {
		return docxResolvedStyle{}
	}
	if seen == nil {
		seen = make(map[string]bool)
	}
	if seen[id] {
		return docxResolvedStyle{}
	}
	seen[id] = true
	style, ok := styles[id]
	if !ok {
		return docxResolvedStyle{}
	}
	var resolved docxResolvedStyle
	if style.BasedOn != nil {
		resolved = resolveDOCXStyle(style.BasedOn.Value, styles, seen)
	}
	if style.Name != nil {
		resolved.Name = style.Name.Value
	}
	if isParagraphNumberingDisabled(style.Properties) {
		resolved.NumberingRef = nil
		resolved.NumberingDisabled = true
	} else if ref := paragraphNumberingRef(style.Properties); ref != nil {
		resolved.NumberingRef = ref
		resolved.NumberingDisabled = false
	}
	if style.Properties != nil {
		if level, ok := parseDOCXXMLInt(style.Properties.OutlineLevel); ok {
			resolved.OutlineLevel = &level
		}
	}
	return resolved
}

func numberingRefForParagraphStyle(styleID string, definitions docxNumberingDefinitions) *docxParagraphNumbering {
	if styleID == "" {
		return nil
	}
	for _, id := range definitions.Order {
		instance := definitions.Instances[id]
		for level, definition := range definitions.Abstracts[instance.AbstractID] {
			if definition.ParagraphStyle == styleID {
				return &docxParagraphNumbering{ID: id, Level: level}
			}
		}
	}
	return nil
}

func docxHeadingLevel(styleID string, style docxResolvedStyle, properties *docxXMLParagraphProperties) (int, bool) {
	headingLevel, isHeading := docxHeadingStyleLevel(styleID, style.Name)
	if !isHeading {
		return 0, false
	}
	if properties != nil {
		if level, ok := parseDOCXXMLInt(properties.OutlineLevel); ok && level >= 0 && level < 9 {
			return level + 1, true
		}
	}
	if style.OutlineLevel != nil && *style.OutlineLevel >= 0 && *style.OutlineLevel < 9 {
		return *style.OutlineLevel + 1, true
	}
	return headingLevel, true
}

func docxHeadingStyleLevel(names ...string) (int, bool) {
	for _, name := range names {
		lower := strings.ToLower(strings.ReplaceAll(name, " ", ""))
		for _, prefix := range []string{"heading", "заголовок"} {
			if !strings.HasPrefix(lower, prefix) {
				continue
			}
			level, err := strconv.Atoi(strings.TrimPrefix(lower, prefix))
			if err == nil && level >= 1 && level <= 9 {
				return level, true
			}
		}
	}
	return 0, false
}

func resetDOCXLowerLevelCounters(counters []int, currentLevel, id int, definitions docxNumberingDefinitions) {
	for levelIndex := currentLevel + 1; levelIndex < len(counters); levelIndex++ {
		level, ok := definitions.resolveLevel(id, levelIndex)
		restartLevel := levelIndex
		if ok && level.RestartLevel != nil {
			if *level.RestartLevel == 0 {
				continue
			}
			restartLevel = *level.RestartLevel
		}
		if currentLevel < restartLevel {
			counters[levelIndex] = 0
		}
	}
}

func paragraphText(paragraph docxXMLParagraph) string {
	decoder := xml.NewDecoder(bytes.NewReader(paragraph.InnerXML))
	var builder strings.Builder
	for {
		token, err := decoder.Token()
		if err != nil {
			break
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		switch start.Name.Local {
		case "t":
			var text string
			if err := decoder.DecodeElement(&text, &start); err == nil {
				builder.WriteString(text)
			}
		case "tab":
			builder.WriteByte('\t')
		}
	}
	return builder.String()
}

func renderDOCXNumberingMarker(template string, id int, counters []int, definitions docxNumberingDefinitions) string {
	marker := template
	for index := 0; index < 9; index++ {
		placeholder := "%" + strconv.Itoa(index+1)
		if !strings.Contains(marker, placeholder) {
			continue
		}
		value := counters[index]
		level, ok := definitions.resolveLevel(id, index)
		if value == 0 && ok {
			value = level.Start
		}
		format := "decimal"
		if ok && level.Format != "" {
			format = level.Format
		}
		marker = strings.ReplaceAll(marker, placeholder, formatDOCXNumber(value, format))
	}
	return marker
}

func formatDOCXNumber(value int, format string) string {
	switch format {
	case "lowerLetter":
		return formatDOCXLetters(value, false)
	case "upperLetter":
		return formatDOCXLetters(value, true)
	case "lowerRoman":
		return strings.ToLower(formatDOCXRoman(value))
	case "upperRoman":
		return formatDOCXRoman(value)
	case "decimalZero":
		if value >= 0 && value < 10 {
			return "0" + strconv.Itoa(value)
		}
	}
	return strconv.Itoa(value)
}

func formatDOCXLetters(value int, upper bool) string {
	if value <= 0 {
		return strconv.Itoa(value)
	}
	var result []rune
	base := 'a'
	if upper {
		base = 'A'
	}
	for value > 0 {
		value--
		result = append([]rune{base + rune(value%26)}, result...)
		value /= 26
	}
	return string(result)
}

func formatDOCXRoman(value int) string {
	if value <= 0 || value > 3999 {
		return strconv.Itoa(value)
	}
	values := []struct {
		value  int
		symbol string
	}{{1000, "M"}, {900, "CM"}, {500, "D"}, {400, "CD"}, {100, "C"}, {90, "XC"}, {50, "L"}, {40, "XL"}, {10, "X"}, {9, "IX"}, {5, "V"}, {4, "IV"}, {1, "I"}}
	var builder strings.Builder
	for _, item := range values {
		for value >= item.value {
			builder.WriteString(item.symbol)
			value -= item.value
		}
	}
	return builder.String()
}

func parseDOCXXMLInt(value *docxXMLValue) (int, bool) {
	if value == nil {
		return 0, false
	}
	parsed, err := strconv.Atoi(value.Value)
	return parsed, err == nil
}

func applyDOCXNumberedHeadingsToSections(items []map[string]any, headings []docxNumberedHeading) []map[string]any {
	if len(headings) == 0 {
		return items
	}
	result := make([]map[string]any, 0, len(items))
	headingIndex := 0
	for _, item := range items {
		text, ok := item["text"].(string)
		if !ok || text == "" {
			result = append(result, item)
			continue
		}
		matchedIndex := -1
		for index := headingIndex; index < len(headings); index++ {
			if strings.TrimSpace(text) == headings[index].Text {
				matchedIndex = index
				break
			}
		}
		if matchedIndex < 0 {
			result = append(result, item)
			continue
		}
		copyItem := make(map[string]any, len(item)+1)
		for key, value := range item {
			copyItem[key] = value
		}
		heading := headings[matchedIndex]
		copyItem["text"] = heading.NumberedText
		copyItem["ck_type"] = "heading"
		headingIndex = matchedIndex + 1
		result = append(result, copyItem)
	}
	return result
}

func applyDOCXNumberedHeadingsToMarkdown(markdown string, headings []docxNumberedHeading) string {
	if markdown == "" || len(headings) == 0 {
		return markdown
	}
	lines := strings.Split(markdown, "\n")
	headingIndex := 0
	for i, line := range lines {
		content, ok := docxMarkdownListOrHeadingText(line)
		isSetext := false
		if !ok {
			content, ok = docxSetextHeadingText(lines, i)
			isSetext = ok
		}
		if !ok {
			continue
		}
		for j := headingIndex; j < len(headings); j++ {
			heading := headings[j]
			if content != heading.Text {
				continue
			}
			level := heading.Level
			if level < 1 {
				level = 1
			}
			if level > 6 {
				level = 6
			}
			if isSetext {
				lines[i] = heading.NumberedText
			} else {
				lines[i] = strings.Repeat("#", level) + " " + heading.NumberedText
			}
			headingIndex = j + 1
			break
		}
	}
	return strings.Join(lines, "\n")
}

func docxMarkdownListOrHeadingText(line string) (string, bool) {
	line = strings.TrimSpace(line)
	markerEnd := 0
	if strings.HasPrefix(line, "#") {
		for markerEnd < len(line) && line[markerEnd] == '#' {
			markerEnd++
		}
	} else if len(line) >= 2 && (line[0] == '-' || line[0] == '*' || line[0] == '+') && line[1] == ' ' {
		markerEnd = 1
	} else {
		for markerEnd < len(line) && line[markerEnd] >= '0' && line[markerEnd] <= '9' {
			markerEnd++
		}
		if markerEnd == 0 || markerEnd >= len(line) || (line[markerEnd] != '.' && line[markerEnd] != ')') {
			return "", false
		}
		markerEnd++
	}
	if markerEnd >= len(line) || line[markerEnd] != ' ' {
		return "", false
	}
	content := strings.TrimSpace(line[markerEnd:])
	content = strings.Trim(content, "*_~`")
	return content, true
}

func docxSetextHeadingText(lines []string, index int) (string, bool) {
	if index < 0 || index+1 >= len(lines) {
		return "", false
	}
	content := strings.TrimSpace(lines[index])
	if content == "" || strings.Contains(content, "|") {
		return "", false
	}
	if _, isListOrATX := docxMarkdownListOrHeadingText(content); isListOrATX {
		return "", false
	}
	underline := strings.TrimSpace(lines[index+1])
	if underline == "" || (underline[0] != '=' && underline[0] != '-') {
		return "", false
	}
	for markerIndex := 1; markerIndex < len(underline); markerIndex++ {
		if underline[markerIndex] != underline[0] {
			return "", false
		}
	}
	return strings.Trim(content, "*_~`"), true
}

func appendDOCXNumberedHeadingOutlines(outlines []docxOutline, headings []docxNumberedHeading) []docxOutline {
	for _, heading := range headings {
		outlines = append(outlines, docxOutline{Title: heading.NumberedText, Level: heading.Level - 1})
	}
	return outlines
}
