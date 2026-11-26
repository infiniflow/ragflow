// Copied from github.com/mgilbir/gowemf, file metafile.go, at commit
// 377668cb7b13 (D7, docs/UPSTREAM.md). Its MIT license is in
// internal/gowemf/LICENSE.

// Package gowemf parses WMF, EMF and embedded EMF+ containers, decodes supported
// record families, and streams typed commands with object/state checks. It also
// decodes supported bitmap encodings. Walk checks framing only; Decode and Stream
// provide progressively stronger checks. Vector playback and text shaping belong
// to the consumer. See COVERAGE.md for the supported records and explicit limits.
package gowemf

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// Format identifies the namespace of a record type.
type Format uint8

const (
	WMF Format = iota + 1
	EMF
	EMFPlus
)

// Limits bound work before records are delivered. Zero fields use defaults;
// there is no unlimited mode. Limits apply to outer and nested records together.
type Limits struct {
	MaxBytes       uint64
	MaxRecordBytes uint64
	MaxRecords     uint64
}

func (l Limits) defaults() Limits {
	if l.MaxBytes == 0 {
		l.MaxBytes = 64 << 20
	}
	if l.MaxRecordBytes == 0 {
		l.MaxRecordBytes = 16 << 20
	}
	if l.MaxRecords == 0 {
		l.MaxRecords = 1_000_000
	}
	return l
}

var (
	ErrFormat    = errors.New("unrecognized metafile format")
	ErrMalformed = errors.New("malformed metafile")
	ErrLimit     = errors.New("metafile resource limit exceeded")
)

// ParseError locates a framing failure at a byte offset in the original input.
type ParseError struct {
	Offset int
	Field  string
	Err    error
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("gowemf: byte %d: %s: %v", e.Offset, e.Field, e.Err)
}
func (e *ParseError) Unwrap() error { return e.Err }

// Rect is a signed rectangle. Units depend on the containing header field.
type Rect struct{ Left, Top, Right, Bottom int32 }
type Size struct{ X, Y int32 }

// PlaceableHeader specifies WMF placement in logical units.
type PlaceableHeader struct {
	Bounds       Rect
	UnitsPerInch uint16
}

type WMFHeader struct {
	Type, Version, Objects    uint16
	SizeWords, MaxRecordWords uint32
}

// EMFHeader exposes the common 88-byte header. Extensions remain in the raw
// EMR_HEADER record; Bounds are logical units and Frame is in .01 millimeters.
type EMFHeader struct {
	Description             []byte // UTF-16LE, including any terminators
	Extension1              *EMFHeaderExtension1
	Micrometers             *Size
	Bounds, Frame           Rect
	Version, Bytes, Records uint32
	Handles                 uint16
	PaletteEntries          uint32
	Device, Millimeters     Size
}

type EMFHeaderExtension1 struct {
	OpenGL      bool
	PixelFormat []byte
}

type EMFPlusHeader struct {
	Dual                                     bool
	Version, Flags, LogicalDpiX, LogicalDpiY uint32
}

// Header contains metadata collected by a complete successful walk.
type Header struct {
	Format    Format
	Placeable *PlaceableHeader
	WMF       *WMFHeader
	EMF       *EMFHeader
	EMFPlus   *EMFPlusHeader
}

// Record is a zero-copy view. Raw includes the record header and padding; Data
// excludes the framing header (and, for EMF+, padding). Type is format-specific.
// ParentOffset is -1 for outer records, or the containing EMR_COMMENT offset.
// Slices alias the caller's input and must not be modified during Walk.
type Record struct {
	Format               Format
	Type                 uint32
	Flags                uint16
	Offset, ParentOffset int
	Raw, Data            []byte
}

// Walk validates a complete single metafile and visits records in file order.
// EMF headers and EOF records are visited; WMF fixed headers are metadata only.
// EMF+ records are visited immediately after their containing EMR_COMMENT.
// Unknown records are preserved, not interpreted or silently discarded.
// A nil visit validates framing only. Visitor errors stop immediately and are
// returned unchanged. Earlier callbacks may have run when a later error occurs;
// callers must not treat their effects as a successfully validated document.
// The input must remain immutable throughout the call, including in callbacks.
// No input-sized allocations or record collections are made by Walk.
func Walk(data []byte, limits Limits, visit func(Record) error) (Header, error) {
	p := parser{data: data, limits: limits.defaults(), visit: visit}
	if uint64(len(data)) > p.limits.MaxBytes {
		return Header{}, failure(0, "file size", ErrLimit)
	}
	var err error
	switch {
	case len(data) >= 4 && u32(data) == 0x9ac6cdd7:
		err = p.wmf(true)
	case len(data) >= 4 && u32(data) == 1:
		err = p.emf()
	case len(data) >= 4 && (u16(data) == 1 || u16(data) == 2) && u16(data[2:]) == 9:
		err = p.wmf(false)
	default:
		err = failure(0, "signature", ErrFormat)
	}
	if err != nil {
		return Header{}, err
	}
	return p.header, nil
}

type parser struct {
	data      []byte
	limits    Limits
	visit     func(Record) error
	header    Header
	count     uint64
	plusEnded bool
}

func failure(off int, field string, err error) error { return &ParseError{off, field, err} }
func malformed(off int, field string) error          { return failure(off, field, ErrMalformed) }
func u16(b []byte) uint16                            { return binary.LittleEndian.Uint16(b) }
func u32(b []byte) uint32                            { return binary.LittleEndian.Uint32(b) }
func rect(b []byte) Rect {
	return Rect{int32(u32(b)), int32(u32(b[4:])), int32(u32(b[8:])), int32(u32(b[12:]))}
}
func size(b []byte) Size { return Size{int32(u32(b)), int32(u32(b[4:]))} }

// Check sizes in uint64 before converting to int, including on 32-bit hosts.
func (p *parser) record(off int, n uint64, available, minimum, alignment int) error {
	if n < uint64(minimum) || n%uint64(alignment) != 0 {
		return malformed(off, "record size")
	}
	if n > p.limits.MaxRecordBytes {
		return failure(off, "record size", ErrLimit)
	}
	if n > uint64(available) {
		return malformed(off, "truncated record")
	}
	if p.count >= p.limits.MaxRecords {
		return failure(off, "record count", ErrLimit)
	}
	p.count++
	return nil
}

func (p *parser) emit(f Format, typ uint32, flags uint16, off, parent int, raw, data []byte) error {
	if p.visit == nil {
		return nil
	}
	return p.visit(Record{f, typ, flags, off, parent, raw[:len(raw):len(raw)], data[:len(data):len(data)]})
}
