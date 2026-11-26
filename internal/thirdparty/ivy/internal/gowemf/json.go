// Copied from github.com/mgilbir/gowemf, file json.go, at commit
// 377668cb7b13 (D7, docs/UPSTREAM.md). Its MIT license is in
// internal/gowemf/LICENSE.

package gowemf

import "encoding/json"

func (r EffectRectangles) MarshalJSON() ([]byte, error) {
	rects := make([]Rect, r.Len())
	for i := range rects {
		rects[i] = r.At(i)
	}
	return json.Marshal(rects)
}

func (v GradientVertices) MarshalJSON() ([]byte, error) {
	vertices := make([]GradientVertex, v.Len())
	for i := range vertices {
		vertices[i] = v.At(i)
	}
	return json.Marshal(vertices)
}

// MarshalJSON emits coordinate values, not the private wire representation.
// Serialization allocates output proportional to the (already bounded) array.
func (p Points) MarshalJSON() ([]byte, error) {
	v := make([]Point, p.Len())
	for i := range v {
		v[i] = p.At(i)
	}
	return json.Marshal(v)
}
func (a Integers) MarshalJSON() ([]byte, error) {
	v := make([]uint32, a.Len())
	for i := range v {
		v[i] = a.At(i)
	}
	return json.Marshal(v)
}
func (b Boxes) MarshalJSON() ([]byte, error) {
	v := make([]Box, b.Len())
	for i := range v {
		v[i] = b.At(i)
	}
	return json.Marshal(v)
}

func (f Format) String() string {
	switch f {
	case WMF:
		return "WMF"
	case EMF:
		return "EMF"
	case EMFPlus:
		return "EMF+"
	default:
		return "unknown"
	}
}
