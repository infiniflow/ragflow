// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package media

import (
	"bytes"
	"encoding/binary"
	"strings"
	"sync"
	"unicode/utf16"

	"github.com/xo/okage/svg"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
)

var metafileSystemFonts = sync.OnceValue(func() map[string]bool {
	db := svg.NewFontDatabase()
	db.LoadOSFonts()
	fonts := make(map[string]bool)
	for face := range db.Faces() {
		for _, family := range face.Families {
			fonts[strings.ToLower(family.Name)] = true
		}
	}
	return fonts
})

// substituteMetafileFonts replaces missing Windows font names in a private
// copy, keeping their character sets, record sizes and drawing commands.
func substituteMetafileFonts(data []byte, fonts map[string]bool) []byte {
	var replaced []byte
	replace := func(offset, size, width int, name string) {
		fallback := metafileFontFallback(name, size/width, fonts)
		if fallback == "" {
			return
		}
		if replaced == nil {
			replaced = bytes.Clone(data)
		}
		field := replaced[offset : offset+size]
		clear(field)
		// All fallback names are ASCII, including when encoded as UTF-16LE.
		for i := range len(fallback) {
			field[i*width] = fallback[i]
		}
	}

	switch rawMetafileMIME(data) {
	case "image/x-wmf":
		offset := 18 // META_HEADER
		if bytes.HasPrefix(data, []byte{0xd7, 0xcd, 0xc6, 0x9a}) {
			offset += 22 // Optional placeable header.
		}
		for len(data)-offset >= 6 {
			record := data[offset:]
			size := int64(binary.LittleEndian.Uint32(record)) * 2
			if size < 6 || size > int64(len(record)) {
				break
			}
			kind := binary.LittleEndian.Uint16(record[4:])
			if kind == 0 { // META_EOF
				break
			}
			if kind == 0x02fb && size > 24 { // META_CREATEFONTINDIRECT
				field := record[24:min(int(size), 56)]
				replace(offset+24, len(field), 1, wmfFaceName(field, record[19]))
			}
			offset += int(size)
		}
	case "image/x-emf":
		for offset := 0; len(data)-offset >= 8; {
			record := data[offset:]
			kind := binary.LittleEndian.Uint32(record)
			size := int64(binary.LittleEndian.Uint32(record[4:]))
			if size < 8 || size%4 != 0 || size > int64(len(record)) || kind == 14 { // EMR_EOF
				break
			}
			if emfUsesGlyphIndexes(kind, record[:int(size)]) {
				// Glyph IDs belong to the original face. Preserve its identity so
				// Ivy can reject an unavailable font instead of drawing wrong glyphs.
				return data
			}
			if kind == 82 && size >= 104 { // EMR_EXTCREATEFONTINDIRECTW
				field := record[40:104]
				var name [32]uint16
				length := 0
				for length < len(name) {
					unit := binary.LittleEndian.Uint16(field[length*2:])
					if unit == 0 {
						break
					}
					name[length] = unit
					length++
				}
				replace(offset+40, len(field), 2, string(utf16.Decode(name[:length])))
			}
			offset += int(size)
		}
	}
	if replaced != nil {
		return replaced
	}
	return data
}

func emfUsesGlyphIndexes(kind uint32, record []byte) bool {
	const glyphIndex = 0x0010 // ETO_GLYPH_INDEX
	switch kind {
	case 84: // EMR_EXTTEXTOUTW
		return len(record) >= 56 && binary.LittleEndian.Uint32(record[52:])&glyphIndex != 0
	case 108: // EMR_SMALLTEXTOUT
		return len(record) >= 24 && binary.LittleEndian.Uint32(record[20:])&glyphIndex != 0
	case 97: // EMR_POLYTEXTOUTW
		if len(record) < 40 {
			return false
		}
		count := binary.LittleEndian.Uint32(record[36:])
		texts := record[40:]
		for range count {
			if len(texts) < 24 {
				return false
			}
			options := binary.LittleEndian.Uint32(texts[16:])
			if options&glyphIndex != 0 {
				return true
			}
			size := 24
			if options&0x0100 == 0 { // ETO_NO_RECT
				size += 16
			}
			if len(texts) < size {
				return false
			}
			texts = texts[size:]
		}
	}
	return false
}

func wmfFaceName(field []byte, charset byte) string {
	name, _, _ := bytes.Cut(field, []byte{0})
	switch charset {
	case 134: // GB2312_CHARSET
		name, _ = simplifiedchinese.GBK.NewDecoder().Bytes(name)
	case 136: // CHINESEBIG5_CHARSET
		name, _ = traditionalchinese.Big5.NewDecoder().Bytes(name)
	}
	return string(name)
}

func metafileFontFallback(name string, capacity int, fonts map[string]bool) string {
	key := strings.ToLower(strings.TrimSpace(name))
	if fonts[key] {
		return ""
	}
	var alternatives []string
	switch key {
	case "宋体", "simsun", "新宋体", "nsimsun", "仿宋", "fangsong", "fangsong_gb2312":
		alternatives = []string{"Noto Serif CJK SC", "Noto Sans CJK SC", "Droid Sans Fallback"}
	case "黑体", "simhei", "微软雅黑", "microsoft yahei", "楷体", "kaiti", "kaiti_gb2312":
		alternatives = []string{"Noto Sans CJK SC", "Noto Serif CJK SC", "Droid Sans Fallback"}
	case "細明體", "mingliu", "新細明體", "pmingliu":
		alternatives = []string{"Noto Serif CJK TC", "Noto Sans CJK TC", "Droid Sans Fallback"}
	case "微軟正黑體", "microsoft jhenghei":
		alternatives = []string{"Noto Sans CJK TC", "Noto Serif CJK TC", "Droid Sans Fallback"}
	}
	for _, family := range alternatives {
		if len(family) < capacity && fonts[strings.ToLower(family)] {
			return family
		}
	}
	return ""
}
