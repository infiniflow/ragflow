// Copied from github.com/mgilbir/gowemf, file color_state.go, at commit
// 377668cb7b13 (D7, docs/UPSTREAM.md). Its MIT license is in
// internal/gowemf/LICENSE.

package gowemf

// ColorPlaybackState is an immutable snapshot for GDI commands. ICMMode is 1
// (off), 2 (on), or 4 (conversion done outside the DC). Source is the selected
// logical color space. OutputProfile and Proof describe explicit profile records;
// renderers decide the final output space and call the conversion/proofing APIs.
// Adjustment is retained for halftone playback; its pixel algorithm is not guessed.
type ColorPlaybackState struct {
	ICMMode       uint32
	Source        ColorSpace
	Adjustment    ColorAdjustment
	OutputProfile *ColorProfile
	Proof         *ColorProfile
	handle        uint32
	generation    uint64
}
type colorObjectEntry struct {
	space      ColorSpace
	generation uint64
}

func initialColorState() *ColorPlaybackState {
	return &ColorPlaybackState{ICMMode: 1, Source: ColorSpace{Type: ColorSRGB}, Adjustment: ColorAdjustment{RedGamma: 10000, GreenGamma: 10000, BlueGamma: 10000, ReferenceWhite: 10000}}
}

func (s *streamState) changeColorState(r Record, body any) error {
	if r.Format != EMF {
		return nil
	}
	next := *s.color
	switch r.Type {
	case EMRSetICMMode:
		mode := body.(Value).Value
		if mode < 1 || mode > 4 {
			return malformed(r.Offset, "ICM mode")
		}
		if mode == 3 {
			return nil
		}
		next.ICMMode = mode
	case EMRSetColorSpace:
		id := body.(Value).Value
		entry, ok := s.colorObjects[id]
		if !ok {
			return malformed(r.Offset, "undefined logical color space")
		}
		next.Source, next.handle, next.generation = entry.space, id, entry.generation
	case EMRSetColorAdjustment:
		next.Adjustment = body.(ColorAdjustment)
	case EMRSetICMProfileA, EMRSetICMProfileW:
		profile := body.(ColorProfile)
		next.OutputProfile = &profile
	case EMRColorMatchToTargetW:
		profile := body.(ColorProfile)
		switch profile.Action {
		case 1:
			next.Proof = &profile
		case 2:
			next.Proof = nil
		case 3:
			return nil
		}
	default:
		return nil
	}
	s.color = &next
	return nil
}

func (s *streamState) deleteColorObject(id uint32) {
	delete(s.colorObjects, id)
	if s.color.handle == id && id != 0 {
		next := *s.color
		next.Source = ColorSpace{Type: ColorSRGB}
		next.handle = 0
		next.generation = 0
		s.color = &next
	}
}
