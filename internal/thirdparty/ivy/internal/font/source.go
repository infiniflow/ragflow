package font

import (
	"encoding/binary"
	"strconv"
	"strings"
)

// Source is the font file of a face, which a writer of PDF embeds (D58).
type Source struct {
	// Data is the bytes of a TrueType, OpenType or collection font. A
	// caller must not change them.
	Data []byte
	// Index is the font of a collection, from 0. It is 0 for a font that
	// is not a collection.
	Index int
	// Variations are the coordinates of the axes of a variable font, in
	// the units of the axes, such as 700 for the axis wght.
	Variations []Variation
}

// Variation is the coordinate of one axis of a variable font.
type Variation struct {
	Tag   [4]byte
	Value float32
}

// Key returns a key of the source: two sources with the same key are the
// same face of the same bytes. The variations are in the key only when the
// font has the table fvar, so the faces of a font without axes share one
// key.
func (s Source) Key() SourceKey {
	k := SourceKey{n: len(s.Data), index: s.Index}
	if len(s.Data) > 0 {
		k.data = &s.Data[0]
	}
	if len(s.Variations) > 0 && s.hasTable("fvar") {
		var b strings.Builder
		for _, v := range s.Variations {
			b.Write(v.Tag[:])
			b.WriteString(strconv.FormatFloat(float64(v.Value), 'g', -1, 32))
			b.WriteByte(' ')
		}
		k.vars = b.String()
	}
	return k
}

// SourceKey identifies a Source. It is comparable.
type SourceKey struct {
	data     *byte
	n, index int
	vars     string
}

// HasVariations reports whether the key holds the variations of a
// variable font.
func (k SourceKey) HasVariations() bool {
	return k.vars != ""
}

// The bits of the field fsType of the table OS/2 that forbid a program to
// embed a subset of the outlines of a font (OpenType 1.9, OS/2).
const (
	// fsTypeUsage holds the permissions of the embedding, bits 0 to 3.
	fsTypeUsage = 0x000F
	// fsTypeRestricted is the value of the permissions that forbids the
	// embedding.
	fsTypeRestricted = 0x0002
	// fsTypeNoSubsetting forbids a subset of the font.
	fsTypeNoSubsetting = 0x0100
	// fsTypeBitmapOnly lets a program embed only the bitmaps of the font.
	fsTypeBitmapOnly = 0x0200
)

// Embeddable reports whether the license of the font lets a program embed
// a subset of its outlines, from the field fsType of its table OS/2. The
// permissions of bits 0 to 3 forbid it only when their value is 2, because
// a program takes the least restrictive bit that is set (OpenType 1.9).
// Bit 8 forbids a subset, and bit 9 lets a program embed only bitmaps. A
// font without the table OS/2 can be embedded.
func (s Source) Embeddable() bool {
	os2, ok := s.table("OS/2")
	if !ok || len(os2) < 10 {
		return true
	}
	fsType := binary.BigEndian.Uint16(os2[8:])
	return fsType&fsTypeUsage != fsTypeRestricted && fsType&(fsTypeNoSubsetting|fsTypeBitmapOnly) == 0
}

// hasTable reports whether the font has the table tag.
func (s Source) hasTable(tag string) bool {
	_, ok := s.table(tag)
	return ok
}

// table returns the bytes of the table tag of the font of the source, cut
// at the end of the data. It returns false when the font has no such
// table, or when the table directory is not valid.
func (s Source) table(tag string) ([]byte, bool) {
	data := s.Data
	size := uint64(len(data))
	if size < 12 || s.Index < 0 {
		return nil, false
	}
	off := uint64(0)
	if string(data[:4]) == "ttcf" {
		n := uint64(binary.BigEndian.Uint32(data[8:]))
		i := uint64(s.Index)
		if i >= n || 12+4*i+4 > size {
			return nil, false
		}
		off = uint64(binary.BigEndian.Uint32(data[12+4*i:]))
	} else if s.Index != 0 {
		return nil, false
	}
	if off+12 > size {
		return nil, false
	}
	n := uint64(binary.BigEndian.Uint16(data[off+4:]))
	for i := range n {
		rec := off + 12 + 16*i
		if rec+16 > size {
			return nil, false
		}
		if string(data[rec:rec+4]) != tag {
			continue
		}
		start := uint64(binary.BigEndian.Uint32(data[rec+8:]))
		length := uint64(binary.BigEndian.Uint32(data[rec+12:]))
		if start > size {
			return nil, false
		}
		return data[start:min(start+length, size)], true
	}
	return nil, false
}
