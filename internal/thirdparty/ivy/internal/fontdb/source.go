package fontdb

import (
	"github.com/xo/okage/svg"

	"ragflow/internal/thirdparty/ivy/internal/font"
)

// Source returns the font file of the face id of db, at the coordinates of
// variations, for a writer of PDF that embeds the face (D58). The bytes
// come from FaceData of db, so the faces of one file share them. It returns
// false when db does not hold the bytes of the face.
func Source(db *svg.FontDatabase, id svg.FontID, variations []svg.FontVariation) (font.Source, bool) {
	data, index, ok := db.FaceData(id)
	if !ok || len(data) == 0 {
		return font.Source{}, false
	}
	src := font.Source{Data: data, Index: int(index)}
	if len(variations) > 0 {
		src.Variations = make([]font.Variation, len(variations))
		for i, v := range variations {
			src.Variations[i] = font.Variation{Tag: v.Tag, Value: v.Value}
		}
	}
	return src, true
}

// NewFace returns the face id of db at the coordinates of variations, with
// its source, or nil when db cannot parse it. The face shares the font that
// db parses once (okage D57).
func NewFace(db *svg.FontDatabase, id svg.FontID, variations []svg.FontVariation) (f *font.Face) {
	defer func() {
		// go-text can panic on a malformed font (D5, D18).
		if recover() != nil {
			f = nil
		}
	}()
	face, ok := db.NewFace(id, variations)
	if !ok {
		return nil
	}
	src, ok := Source(db, id, variations)
	if !ok {
		return font.FromFace(face)
	}
	return font.FromFaceSource(face, src)
}
