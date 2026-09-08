//go:build cgo

package parser

import (
	"bytes"
	"context"
	"encoding/binary"
	"testing"
	"unicode/utf16"

	officeOxide "github.com/yfedoseev/office_oxide/go"
)

// buildSyntheticDoc constructs a minimal but valid legacy Word binary (.doc)
// entirely in memory — an OLE2/CFB container with a WordDocument stream
// (FIB + piece-table-referenced text) and a 0Table stream (CLX). No external
// tool or committed fixture is required, so the .doc parser can be exercised
// end-to-end against dynamically generated input.
//
// Layout (sector size = 512):
//
//	sector 0: FAT
//	sector 1: directory (Root Entry, WordDocument, 0Table)
//	sector 2: WordDocument stream (FIB + CP1252 text piece)
//	sector 3: 0Table stream (CLX)
//
// The structure follows office_oxide's reader expectations
// (src/cfb/*, src/doc/fib.rs, src/doc/piece_table.rs).
func buildSyntheticDoc(t *testing.T, text string) []byte {
	t.Helper()
	const sector = 512

	// ── CLX (piece table) in the 0Table stream ────────────────────────────
	// 1 piece covering all text, compressed (CP1252): fc bit 30 set,
	// real byte offset = (fc & ~0x40000000) / 2.
	const textOffset = 426 // safely past the FIB reads (max offset 0x1AA=426)
	fc := uint32(0x40000000) | uint32(textOffset*2)

	clx := make([]byte, 21)
	clx[0] = 0x02                                             // Pcdt
	binary.LittleEndian.PutUint32(clx[1:], 16)                // PlcPcd size
	binary.LittleEndian.PutUint32(clx[5:], 0)                 // CP[0] = 0
	binary.LittleEndian.PutUint32(clx[9:], uint32(len(text))) // CP[1] = text_len
	binary.LittleEndian.PutUint16(clx[13:], 0)                // PCD u16 unused
	binary.LittleEndian.PutUint32(clx[15:], fc)               // PCD fc
	binary.LittleEndian.PutUint16(clx[19:], 0)                // PCD prm

	// ── WordDocument stream: FIB + text ───────────────────────────────────
	wordDoc := make([]byte, sector)
	binary.LittleEndian.PutUint16(wordDoc[0:], 0xA5EC)                // wIdent (Word 97+)
	binary.LittleEndian.PutUint16(wordDoc[2:], 0x00C1)                // nFib
	binary.LittleEndian.PutUint16(wordDoc[0x0A:], 0x0000)             // flags: bit9=0 → 0Table
	binary.LittleEndian.PutUint32(wordDoc[0x4C:], uint32(len(text)))  // ccpText
	binary.LittleEndian.PutUint32(wordDoc[0x01A2:], 0)                // fcClx (offset 0 in 0Table)
	binary.LittleEndian.PutUint32(wordDoc[0x01A6:], uint32(len(clx))) // lcbClx
	copy(wordDoc[textOffset:], []byte(text))

	// ── CFB header ────────────────────────────────────────────────────────
	header := make([]byte, sector)
	copy(header[0:8], []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1})
	binary.LittleEndian.PutUint16(header[0x18:], 0x003E)     // minor version
	binary.LittleEndian.PutUint16(header[0x1A:], 3)          // major version (v3)
	binary.LittleEndian.PutUint16(header[0x1C:], 0xFFFE)     // byte order
	binary.LittleEndian.PutUint16(header[0x1E:], 9)          // sector shift (512)
	binary.LittleEndian.PutUint16(header[0x20:], 6)          // mini sector shift (64)
	binary.LittleEndian.PutUint32(header[0x2C:], 1)          // FAT sector count
	binary.LittleEndian.PutUint32(header[0x30:], 1)          // first directory sector
	binary.LittleEndian.PutUint32(header[0x38:], 4096)       // mini-stream cutoff
	binary.LittleEndian.PutUint32(header[0x3C:], 0xFFFFFFFE) // first mini-FAT = END
	binary.LittleEndian.PutUint32(header[0x44:], 0xFFFFFFFE) // first DIFAT = END
	for i := 0; i < 109; i++ {
		off := 0x4C + i*4
		if i == 0 {
			binary.LittleEndian.PutUint32(header[off:], 0) // FAT lives in sector 0
		} else {
			binary.LittleEndian.PutUint32(header[off:], 0xFFFFFFFE) // ignored
		}
	}

	// ── FAT sector ────────────────────────────────────────────────────────
	fat := bytes.Repeat([]byte{0xFF}, sector)            // all FREE (0xFFFFFFFF)
	binary.LittleEndian.PutUint32(fat[0*4:], 0xFFFFFFFD) // sector 0 = FAT itself
	binary.LittleEndian.PutUint32(fat[1*4:], 0xFFFFFFFE) // directory = END
	binary.LittleEndian.PutUint32(fat[2*4:], 0xFFFFFFFE) // WordDocument = END
	binary.LittleEndian.PutUint32(fat[3*4:], 0xFFFFFFFE) // 0Table = END

	// ── Directory sector: 4 entries ───────────────────────────────────────
	dir := make([]byte, sector)
	copy(dir[0*128:], dirEntry("Root Entry", 5, 0xFFFFFFFF, 0))
	copy(dir[1*128:], dirEntry("WordDocument", 2, 2, uint64(len(wordDoc))))
	copy(dir[2*128:], dirEntry("0Table", 2, 3, uint64(len(clx))))
	copy(dir[3*128:], dirEntry("", 0, 0xFFFFFFFF, 0))

	// ── Assemble ──────────────────────────────────────────────────────────
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
		nameBytes = append(nameBytes, 0, 0) // UTF-16LE null terminator
		copy(e[0:], nameBytes)
		binary.LittleEndian.PutUint16(e[0x40:], uint16(len(nameBytes)))
	}
	e[0x42] = entryType
	e[0x43] = 1 // color (black)
	binary.LittleEndian.PutUint32(e[0x44:], 0xFFFFFFFF)
	binary.LittleEndian.PutUint32(e[0x48:], 0xFFFFFFFF)
	binary.LittleEndian.PutUint32(e[0x4C:], 0xFFFFFFFF)
	binary.LittleEndian.PutUint32(e[0x74:], startSector)
	binary.LittleEndian.PutUint32(e[0x78:], uint32(streamSize)) // v3: low 32 bits
	return e
}

// TestDOCParser_SyntheticDocRoundTrip verifies the legacy DOC path end-to-end
// against a .doc that is generated at runtime (no committed binary, no external
// converter). It exercises office_oxide.OpenFromBytes(data, "doc") and the
// extractDocText fallback chain (IR → Markdown → PlainText) inside
// ParseWithResult.
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
	if pr.OutputFormat != "text" {
		t.Fatalf("OutputFormat = %q, want %q", pr.OutputFormat, "text")
	}
	if pr.Text == "" {
		t.Fatal("ParseWithResult returned empty text — synthetic .doc was not parsed")
	}
	for _, want := range []string{"SYNTHETIC DOC", "First para alpha.", "Second para beta."} {
		if !bytes.Contains([]byte(pr.Text), []byte(want)) {
			t.Errorf("parsed text missing %q; got:\n%s", want, pr.Text)
		}
	}
}
