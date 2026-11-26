// Copied from github.com/mgilbir/gowemf, file stream.go, at commit
// 377668cb7b13 (D7, docs/UPSTREAM.md). Its MIT license is in
// internal/gowemf/LICENSE.

package gowemf

import (
	"container/heap"
	"errors"
)

// StreamOptions controls typed command streaming. PreferGDI selects the GDI
// fallback in EMF+ Dual files; requesting it for EMF+ Only returns ErrUnsupported.
// Zero budgets default to 65,536 object slots and 1,024 saved states.
type StreamOptions struct {
	Framing        Limits
	Decoding       DecodeLimits
	MaxObjects     uint32
	MaxSavedStates uint32
	PreferGDI      bool
}

// Command carries a typed body and its source record. For object creation,
// HasObjectID is true and ObjectID is the allocated WMF slot or explicit EMF(+)
// handle. Complete EMF+ objects replace fragment bodies with their decoded type;
// incomplete fragments do not produce a command. Source points to the final
// fragment for a continued object. Bodies/views must be treated as immutable.
// Effect identifies the most recent serialized effect for DrawImagePoints
// commands with the E flag. It is nil for other commands, including DrawImage.
// ColorState is an immutable saved/restored GDI color-state snapshot; it is nil
// for EMF+ commands. It records selections without automatically filtering pixels.
type Command struct {
	ColorState  *ColorPlaybackState
	Effect      *PlusEffect
	Source      Record
	Body        any
	ObjectID    uint32
	HasObjectID bool
}

// Stream validates framing, decodes the selected command stream, and checks
// object lifetimes/types, save/restore references and EMF path brackets before
// delivering commands. It does not rasterize, shape text, or claim support for
// every rendering mode encoded by a decoded record. Unsupported typed records
// are errors, never silently skipped. GDI fallback records are intentionally
// excluded when playing EMF+, except within EmfPlusGetDC intervals.
//
// Like Walk, callbacks can precede a later semantic failure. For transactional
// consumers, call Stream with a nil visitor first, then stream the immutable
// input to the renderer. The framing pre-pass makes Header available internally
// without collecting records. Object kinds, bounded stacks and the latest
// serialized effect description are retained;
// callers retaining commands are responsible for their own memory budget.
func Stream(data []byte, options StreamOptions, visit func(Command) error) (Header, error) {
	h, err := Walk(data, options.Framing, nil)
	if err != nil {
		return Header{}, err
	}
	if options.PreferGDI && h.EMFPlus != nil && !h.EMFPlus.Dual {
		return Header{}, failure(0, "EMF+ Only has no GDI fallback", ErrUnsupported)
	}
	if options.MaxObjects == 0 {
		options.MaxObjects = 65536
	}
	if options.MaxSavedStates == 0 {
		options.MaxSavedStates = 1024
	}
	var slots uint32
	if h.WMF != nil {
		slots = uint32(h.WMF.Objects)
	} else {
		slots = uint32(h.EMF.Handles) + 1
	}
	if slots > options.MaxObjects {
		return Header{}, failure(0, "object table size", ErrLimit)
	}
	s := streamState{options: options, kinds: make([]uint8, int(slots)), paletteCounts: make([]uint32, int(slots)), assembler: PlusAssembler{Limits: options.Decoding}}
	s.color = initialColorState()
	s.colorObjects = make(map[uint32]colorObjectEntry)
	if h.WMF != nil {
		s.free = make(freeHandles, int(slots))
		s.generations = make([]uint64, int(slots))
		for i := range s.free {
			s.free[i] = uint32(i)
		}
	}
	_, err = Walk(data, options.Framing, func(r Record) error {
		if h.EMFPlus != nil {
			if options.PreferGDI && r.Format == EMFPlus {
				return nil
			}
			if !options.PreferGDI {
				if r.Format == EMFPlus {
					s.getDC = r.Type == 0x4004
				} else if r.Type != 1 && r.Type != 14 && r.Type != 70 && !s.getDC {
					return nil
				}
			}
		}
		body, err := Decode(r, options.Decoding)
		if err != nil {
			return err
		}
		cmd := Command{Source: r, Body: body}
		if r.Format == EMFPlus {
			if f, ok := body.(PlusObjectFragment); ok {
				if uint32(f.ID) >= options.MaxObjects {
					return failure(r.Offset, "EMF+ object slot limit", ErrLimit)
				}
				data, err := s.assembler.Add(f)
				if err != nil {
					return streamError(r, err)
				}
				if data == nil {
					return nil
				}
				body, err = DecodePlusObject(f.Type, data, options.Decoding)
				if err != nil {
					return streamError(r, err)
				}
				s.plusKinds[f.ID] = f.Type
				cmd.Body = body
				cmd.ObjectID = uint32(f.ID)
				cmd.HasObjectID = true
			} else if err := s.plus(r, body); err == errIgnored {
				return nil
			} else if err != nil {
				return err
			}
		} else if err := s.gdi(&cmd); err == errIgnored {
			return nil
		} else if err != nil {
			return err
		}
		if r.Format != EMFPlus {
			cmd.ColorState = s.color
		}
		if v, ok := cmd.Body.(PlusImageDraw); ok && v.Effect {
			cmd.Effect = s.effect
		}
		if visit != nil {
			return visit(cmd)
		}
		return nil
	})
	if err != nil {
		return Header{}, err
	}
	if err = s.assembler.Finish(); err != nil {
		return Header{}, err
	}
	if s.path == 1 {
		return Header{}, malformed(len(data), "unclosed EMF path bracket")
	}
	return h, nil
}

// ivy: errIgnored marks a record that Stream drops, as Windows does: the
// record refers to an object that does not exist or that has another type,
// or it creates an object that the table cannot hold. Windows plays the
// other records of such a file, and Stream gives them to the visitor.
var errIgnored = errors.New("gowemf: ignored record")

func streamError(r Record, err error) error {
	if pe, ok := err.(*ParseError); ok {
		return &ParseError{Offset: r.Offset, Field: pe.Field, Err: pe.Err}
	}
	return err
}

type streamState struct {
	color            *ColorPlaybackState
	colorObjects     map[uint32]colorObjectEntry
	colorGeneration  uint64
	savedColors      []*ColorPlaybackState
	effect           *PlusEffect
	options          StreamOptions
	kinds            []uint8
	paletteCounts    []uint32
	generations      []uint64
	paletteSelection paletteReference
	savedPalettes    []paletteReference
	free             freeHandles
	plusKinds        [64]uint8
	saved            uint32
	plusSaved        []plusSavedState
	path             uint8 // 0 none, 1 constructing, 2 selected
	getDC            bool
	assembler        PlusAssembler
}
type plusSavedState struct {
	index     uint32
	container bool
}
type paletteReference struct {
	index      uint32
	generation uint64
	selected   bool
}

const (
	kindPen        uint8 = 1
	kindBrush      uint8 = 2
	kindFont       uint8 = 3
	kindPalette    uint8 = 4
	kindRegion     uint8 = 5
	kindColorSpace uint8 = 6
)

func (s *streamState) gdi(cmd *Command) error {
	r, b := cmd.Source, cmd.Body
	var kind uint8
	var id uint32
	switch v := b.(type) {
	case ColorSpaceObject:
		kind, id = kindColorSpace, v.Handle
	case Pen:
		kind, id = kindPen, v.Handle
	case Brush:
		kind, id = kindBrush, v.Handle
	case PatternBrush:
		kind, id = kindBrush, v.Handle
	case PackedPatternBrush:
		kind = kindBrush
	case BitmapPatternBrush:
		kind = kindBrush
	case WMFRegion:
		kind = kindRegion
	case Font:
		kind, id = kindFont, v.Handle
	case Palette:
		if (r.Format == EMF && r.Type == 49) || (r.Format == WMF && r.Type&255 == 0xf7) {
			kind, id = kindPalette, v.Handle
		}
	}
	if kind != 0 {
		// ivy: an object that the table cannot hold is dropped. An EMF
		// object that takes a handle in use replaces the object.
		if r.Format == WMF {
			if len(s.free) == 0 {
				return errIgnored
			}
			id = heap.Pop(&s.free).(uint32)
		} else if id == 0 || uint64(id) >= uint64(len(s.kinds)) {
			return errIgnored
		} else if s.kinds[id] == kindColorSpace {
			s.deleteColorObject(id)
		}
		s.kinds[id] = kind
		if v, ok := b.(ColorSpaceObject); ok {
			s.colorGeneration++
			s.colorObjects[id] = colorObjectEntry{v.Space, s.colorGeneration}
		}
		if r.Format == WMF {
			s.generations[id]++
		}
		if p, ok := b.(Palette); ok {
			s.paletteCounts[id] = p.Count
		}
		cmd.ObjectID = id
		cmd.HasObjectID = true
		return nil
	}
	typ := r.Type
	if r.Format == WMF {
		typ &= 255
	}
	if r.Format == WMF {
		if err := s.wmfObjects(cmd); err != nil {
			return err
		}
	}
	if r.Format == EMF && (typ == 50 || typ == 51 || typ == EMRColorCorrectPalette) {
		p := b.(Palette)
		if uint64(p.Handle) >= uint64(len(s.kinds)) || s.kinds[p.Handle] != kindPalette {
			return errIgnored // ivy: as Windows does
		}
		if typ != 51 && uint64(p.Start)+uint64(p.Count) > uint64(s.paletteCounts[p.Handle]) {
			return malformed(r.Offset, "palette entry range")
		}
		if typ == 51 {
			s.paletteCounts[p.Handle] = p.Count
		}
	}
	if paint, ok := b.(EMFRegionPaint); ok && paint.HasBrush {
		id := paint.Brush
		if id&0x80000000 != 0 {
			stock := id & 0x7fffffff
			if stock > 5 && stock != 18 {
				return errIgnored // ivy: as Windows does
			}
		} else if uint64(id) >= uint64(len(s.kinds)) || s.kinds[id] != kindBrush {
			return errIgnored // ivy: as Windows does
		}
	}
	selectObject, deleteObject, save, restore := typ == 37, typ == 40, typ == 33, typ == 34
	if r.Format == EMF && (typ == EMRSetColorSpace || typ == EMRDeleteColorSpace) {
		id = b.(Value).Value
		if uint64(id) >= uint64(len(s.kinds)) || s.kinds[id] != kindColorSpace {
			return errIgnored // ivy: as Windows does
		}
		if typ == EMRDeleteColorSpace {
			s.kinds[id] = 0
			s.deleteColorObject(id)
		}
		return s.changeColorState(r, b)
	}
	if r.Format == WMF {
		selectObject, deleteObject, save, restore = typ == 0x2d, typ == 0xf0, typ == 0x1e, typ == 0x27
	}
	if selectObject || deleteObject {
		id = b.(Value).Value
		if r.Format == EMF && id&0x80000000 != 0 {
			n := id & 0x7fffffff
			if deleteObject || n > 19 || n == 9 || n == 15 {
				return errIgnored // ivy: as Windows does
			}
			return nil
		}
		// ivy: Windows ignores a selection or a deletion of an object that
		// does not exist, and the selection of a palette or a color space.
		if uint64(id) >= uint64(len(s.kinds)) || s.kinds[id] == 0 {
			return errIgnored
		}
		if selectObject && (s.kinds[id] == kindPalette || s.kinds[id] == kindColorSpace) {
			return errIgnored
		}
		if deleteObject {
			if s.kinds[id] == kindColorSpace {
				s.deleteColorObject(id)
			}
			s.kinds[id] = 0
			if r.Format == WMF {
				heap.Push(&s.free, id)
			}
		}
	}
	if r.Format == EMF && typ == 48 {
		id = b.(Value).Value
		if id != 0x8000000f && (uint64(id) >= uint64(len(s.kinds)) || s.kinds[id] != kindPalette) {
			return errIgnored // ivy: as Windows does
		}
	}
	if save {
		if s.saved >= s.options.MaxSavedStates {
			return failure(r.Offset, "saved states", ErrLimit)
		}
		s.saved++
		s.savedColors = append(s.savedColors, s.color)
		if r.Format == WMF {
			s.savedPalettes = append(s.savedPalettes, s.paletteSelection)
		}
	}
	if restore {
		n := int64(b.(SignedValue).Value)
		var level int64
		if n < 0 {
			level = int64(s.saved) + n
		} else {
			level = n - 1
		}
		if n == 0 || level < 0 || level >= int64(s.saved) {
			return malformed(r.Offset, "restore state index")
		}
		s.saved = uint32(level)
		s.color = s.savedColors[int(level)]
		s.savedColors = s.savedColors[:int(level)]
		if s.color.handle != 0 {
			entry, ok := s.colorObjects[s.color.handle]
			if !ok || entry.generation != s.color.generation {
				next := *s.color
				next.Source = ColorSpace{Type: ColorSRGB}
				next.handle = 0
				next.generation = 0
				s.color = &next
			}
		}
		if r.Format == WMF {
			s.paletteSelection = s.savedPalettes[int(level)]
			s.savedPalettes = s.savedPalettes[:int(level)]
		}
	}
	if r.Format == EMF {
		switch typ {
		case 59:
			if s.path == 1 {
				return malformed(r.Offset, "nested path bracket")
			}
			s.path = 1
		case 60:
			if s.path != 1 {
				return malformed(r.Offset, "EndPath without BeginPath")
			}
			s.path = 2
		case 61:
			if s.path != 1 {
				return malformed(r.Offset, "CloseFigure outside path")
			}
		case 62, 63, 64, 67:
			if s.path != 2 {
				return malformed(r.Offset, "missing completed path")
			}
			s.path = 0
		case 65, 66:
			if s.path != 2 {
				return malformed(r.Offset, "missing completed path")
			}
		case 68:
			s.path = 0
		}
	}
	return s.changeColorState(r, b)
}

func (s *streamState) plus(r Record, body any) error {
	ref := func(id uint32, typ uint8) error {
		if id > 63 || s.plusKinds[id] != typ {
			return errIgnored // ivy: as GDI+ does
		}
		return nil
	}
	brush := func(id uint32, solid bool) error {
		if solid {
			return nil
		}
		return ref(id, 1)
	}
	switch v := body.(type) {
	case PlusRects:
		if r.Type == 0x400a {
			return brush(v.BrushID, v.Solid)
		}
		return ref(uint32(v.ObjectID), 2)
	case PlusPoly:
		if r.Type == 0x400c {
			return brush(v.BrushID, v.Solid)
		}
		return ref(uint32(v.ObjectID), 2)
	case PlusEllipse:
		if r.Type == 0x400e || r.Type == 0x4010 {
			return brush(v.BrushID, v.Solid)
		}
		return ref(uint32(v.ObjectID), 2)
	case PlusCurve:
		if r.Type == 0x4016 {
			return brush(v.BrushID, v.Solid)
		}
		return ref(uint32(v.ObjectID), 2)
	case PlusDriverString:
		if err := ref(uint32(v.FontID), 6); err != nil {
			return err
		}
		return brush(v.BrushID, v.Solid)
	case PlusContainer:
		if uint64(len(s.plusSaved)) >= uint64(s.options.MaxSavedStates) {
			return failure(r.Offset, "EMF+ saved states", ErrLimit)
		}
		s.plusSaved = append(s.plusSaved, plusSavedState{v.StackIndex, true})
	case PlusPathDraw:
		kind := uint8(3)
		if r.Type == 0x4013 {
			kind = 4
		}
		if err := ref(uint32(v.ObjectID), kind); err != nil {
			return err
		}
		if r.Type == 0x4015 {
			return ref(v.PaintID, 2)
		}
		return brush(v.PaintID, v.Solid)
	case PlusText:
		if err := ref(uint32(v.FontID), 6); err != nil {
			return err
		}
		if v.FormatID != 0xffffffff {
			if err := ref(v.FormatID, 7); err != nil {
				return err
			}
		}
		return brush(v.BrushID, v.Solid)
	case PlusImageDraw:
		if v.Effect && s.effect == nil {
			return malformed(r.Offset, "image effect without earlier serialized object")
		}
		if err := ref(uint32(v.ImageID), 5); err != nil {
			return err
		}
		if v.AttributesID != 0xffffffff {
			return ref(v.AttributesID, 8)
		}
	case PlusClip:
		if v.Mode > 5 {
			return malformed(r.Offset, "EMF+ combine mode")
		}
		if r.Type == 0x4033 {
			return ref(uint32(v.ObjectID), 3)
		}
		if r.Type == 0x4034 {
			return ref(uint32(v.ObjectID), 4)
		}
	case PlusEffect:
		s.effect = &v
	case Value:
		if r.Type == 0x4025 || r.Type == 0x4028 {
			if uint64(len(s.plusSaved)) >= uint64(s.options.MaxSavedStates) {
				return failure(r.Offset, "EMF+ saved states", ErrLimit)
			}
			s.plusSaved = append(s.plusSaved, plusSavedState{v.Value, r.Type == 0x4028})
		} else if r.Type == 0x4026 || r.Type == 0x4029 {
			found := -1
			for i := len(s.plusSaved) - 1; i >= 0; i-- {
				if s.plusSaved[i].index == v.Value && s.plusSaved[i].container == (r.Type == 0x4029) {
					found = i
					break
				}
			}
			if found < 0 {
				return malformed(r.Offset, "EMF+ saved state index")
			}
			s.plusSaved = s.plusSaved[:found]
		}
	}
	return nil
}

// A min-heap avoids quadratic rescanning of WMF's lowest-available-slot rule
// when adversarial streams repeatedly delete/recreate a low-numbered object.
type freeHandles []uint32

func (h freeHandles) Len() int           { return len(h) }
func (h freeHandles) Less(i, j int) bool { return h[i] < h[j] }
func (h freeHandles) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *freeHandles) Push(v any)        { *h = append(*h, v.(uint32)) }
func (h *freeHandles) Pop() any          { a := *h; v := a[len(a)-1]; *h = a[:len(a)-1]; return v }
