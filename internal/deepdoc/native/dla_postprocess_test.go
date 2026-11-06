//go:build cgo && integration

package native

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestDLARealEnPaperMatchesGolden pins the dla_real_en_paper layout output to
// the golden generated from the OSS Python serving post-process (cv2
// preprocessing + per-class nms(0.45), class ids remapped through DLA_CLASS_MAP
// to match Go's Wire()). The layout .ort graph contains NO NMS op — output0 is
// raw candidate boxes — but the serving pipeline applies per-class nms(0.45)
// before handing boxes to the caller, and that post-NMS output is the wire
// contract. Go's dlaPostprocess re-runs that same nms so its caller-visible
// boxes match; this is the single NMS the serving applies (native_analyzer maps
// the boxes through as-is, no second NMS), so it is not a double-NMS. The
// golden is regenerated from real Python in-process inference, so any drift in
// Go's preprocessing, NMS, or class remap shows up here as a per-coordinate
// diff beyond CmpTolCoord.
func TestDLARealEnPaperMatchesGolden(t *testing.T) {
	skipIfNoModels(t)
	img, err := Decode(filepath.Join("testdata", "dla_real_en_paper.png"))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	res, err := RunDLA(context.Background(), os.Getenv("MODEL_DIR"), img)
	if err != nil {
		t.Fatalf("RunDLA: %v", err)
	}
	var got struct {
		Boxes [][]float64 `json:"bboxes"`
	}
	if err := json.Unmarshal([]byte(res.Wire()), &got); err != nil {
		t.Fatalf("parse Go wire: %v", err)
	}
	gold := LoadGoldenBoxes(t, filepath.Join("testdata", "dla_real_en_paper.dla.golden.json"))
	// CompareBoxes fails on any per-coordinate diff beyond CmpTolCoord (3.5px).
	CompareBoxes(t, gold, got.Boxes)
}
