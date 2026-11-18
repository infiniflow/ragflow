//go:build cgo

package parser

import (
	"bytes"
	"context"
	"encoding/binary"
	"strings"
	"testing"
	"unicode/utf16"

	"ragflow/internal/entity"

	officeOxide "github.com/yfedoseev/office_oxide/go"
)

// buildSyntheticDoc constructs a minimal but valid legacy Word binary (.doc)
// entirely in memory.
func buildSyntheticDoc(t *testing.T, text string) []byte {
	t.Helper()
	const sector = 512
	const textOffset = 426
	fc := uint32(0x40000000) | uint32(textOffset*2)

	clx := make([]byte, 21)
	clx[0] = 0x02
	binary.LittleEndian.PutUint32(clx[1:], 16)
	binary.LittleEndian.PutUint32(clx[5:], 0)
	binary.LittleEndian.PutUint32(clx[9:], uint32(len(text)))
	binary.LittleEndian.PutUint16(clx[13:], 0)
	binary.LittleEndian.PutUint32(clx[15:], fc)
	binary.LittleEndian.PutUint16(clx[19:], 0)

	wordDoc := make([]byte, sector)
	binary.LittleEndian.PutUint16(wordDoc[0:], 0xA5EC)
	binary.LittleEndian.PutUint16(wordDoc[2:], 0x00C1)
	binary.LittleEndian.PutUint16(wordDoc[0x0A:], 0x0000)
	binary.LittleEndian.PutUint32(wordDoc[0x4C:], uint32(len(text)))
	binary.LittleEndian.PutUint32(wordDoc[0x01A2:], 0)
	binary.LittleEndian.PutUint32(wordDoc[0x01A6:], uint32(len(clx)))
	copy(wordDoc[textOffset:], []byte(text))

	header := make([]byte, sector)
	copy(header[0:8], []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1})
	binary.LittleEndian.PutUint16(header[0x18:], 0x003E)
	binary.LittleEndian.PutUint16(header[0x1A:], 3)
	binary.LittleEndian.PutUint16(header[0x1C:], 0xFFFE)
	binary.LittleEndian.PutUint16(header[0x1E:], 9)
	binary.LittleEndian.PutUint16(header[0x20:], 6)
	binary.LittleEndian.PutUint32(header[0x2C:], 1)
	binary.LittleEndian.PutUint32(header[0x30:], 1)
	binary.LittleEndian.PutUint32(header[0x38:], 4096)
	binary.LittleEndian.PutUint32(header[0x3C:], 0xFFFFFFFE)
	binary.LittleEndian.PutUint32(header[0x44:], 0xFFFFFFFE)
	for i := 0; i < 109; i++ {
		off := 0x4C + i*4
		if i == 0 {
			binary.LittleEndian.PutUint32(header[off:], 0)
		} else {
			binary.LittleEndian.PutUint32(header[off:], 0xFFFFFFFE)
		}
	}

	fat := bytes.Repeat([]byte{0xFF}, sector)
	binary.LittleEndian.PutUint32(fat[0*4:], 0xFFFFFFFD)
	binary.LittleEndian.PutUint32(fat[1*4:], 0xFFFFFFFE)
	binary.LittleEndian.PutUint32(fat[2*4:], 0xFFFFFFFE)
	binary.LittleEndian.PutUint32(fat[3*4:], 0xFFFFFFFE)

	dir := make([]byte, sector)
	copy(dir[0*128:], dirEntry("Root Entry", 5, 0xFFFFFFFF, 0))
	copy(dir[1*128:], dirEntry("WordDocument", 2, 2, uint64(len(wordDoc))))
	copy(dir[2*128:], dirEntry("0Table", 2, 3, uint64(len(clx))))
	copy(dir[3*128:], dirEntry("", 0, 0xFFFFFFFF, 0))
	binary.LittleEndian.PutUint32(dir[0*128+0x4C:], 1)
	binary.LittleEndian.PutUint32(dir[1*128+0x44:], 2)

	tableSector := make([]byte, sector)
	copy(tableSector, clx)

	out := make([]byte, 5*sector)
	copy(out[0*sector:], header)
	copy(out[1*sector:], fat)
	copy(out[2*sector:], dir)
	copy(out[3*sector:], wordDoc)
	copy(out[4*sector:], tableSector)
	return out
}

func dirEntry(name string, entryType byte, startSector uint32, streamSize uint64) []byte {
	e := make([]byte, 128)
	if name != "" {
		utf16Name := utf16.Encode([]rune(name))
		var nameBytes []byte
		for _, r := range utf16Name {
			nameBytes = append(nameBytes, byte(r&0xFF), byte(r>>8))
		}
		nameBytes = append(nameBytes, 0, 0)
		copy(e[0:], nameBytes)
		binary.LittleEndian.PutUint16(e[0x40:], uint16(len(nameBytes)))
	}
	e[0x42] = entryType
	e[0x43] = 1
	binary.LittleEndian.PutUint32(e[0x44:], 0xFFFFFFFF)
	binary.LittleEndian.PutUint32(e[0x48:], 0xFFFFFFFF)
	binary.LittleEndian.PutUint32(e[0x4C:], 0xFFFFFFFF)
	binary.LittleEndian.PutUint32(e[0x74:], startSector)
	binary.LittleEndian.PutUint32(e[0x78:], uint32(streamSize))
	return e
}

// TestDOCParser_SyntheticDocRoundTrip verifies the legacy DOC path end-to-end
// against a .doc generated at runtime. The JSON path emits structured sections
// (one per paragraph/table/list), so this test asserts the recovered text
// fragments appear across the JSON items.
func TestDOCParser_SyntheticDocRoundTrip(t *testing.T) {
	const body = "SYNTHETIC DOC\rFirst para alpha.\rSecond para beta."
	data := buildSyntheticDoc(t, body)

	doc, err := officeOxide.OpenFromBytes(data, "doc")
	if err != nil {
		t.Fatalf("OpenFromBytes(doc) failed: %v", err)
	}
	defer doc.Close()

	pr := NewDOCParser().ParseWithResult(context.Background(), "synthetic.doc", data)
	if pr.Err != nil {
		t.Fatalf("ParseWithResult error: %v", pr.Err)
	}
	if !pr.Produced() {
		t.Fatalf("produced = %t, want true", pr.Produced())
	}
	if len(pr.JSON) == 0 {
		t.Fatalf("ParseWithResult returned 0 JSON items, want at least 1")
	}

	var b strings.Builder
	for _, it := range pr.JSON {
		m := it.ToMap()
		if txt, _ := m["text"].(string); txt != "" {
			b.WriteString(txt)
			b.WriteString("\n")
		}
		if td, _ := m["table"].(*entity.TableData); td != nil {
			for _, row := range td.CellTexts() {
				for _, c := range row {
					b.WriteString(c)
					b.WriteString(" ")
				}
			}
		}
	}
	got := b.String()
	for _, want := range []string{"SYNTHETIC DOC", "First para alpha.", "Second para beta."} {
		if !strings.Contains(got, want) {
			t.Errorf("parsed JSON missing %q; got:\n%s", want, got)
		}
	}
}
