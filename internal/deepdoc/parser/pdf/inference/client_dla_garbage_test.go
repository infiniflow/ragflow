package inference

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestDeepDocHTTP_DLA_GarbageGate pins the Python-parity 0.4 garbage gate
// (deepdoc/vision/layout_recognizer.py:97): a region whose layout type is a
// garbage layout (footer/header/reference) and whose confidence is strictly
// below 0.4 is dropped; everything else is kept.
//
// The OSS default 10-class DLA taxonomy only emits "reference" as a garbage
// type, but footer/header are covered defensively via pdf.GarbageLayoutTypes.
// Each subtest drives a mock /predict/dla backend returning one bbox and
// asserts the resulting region set.
func TestDeepDocHTTP_DLA_GarbageGate(t *testing.T) {
	newClient := func(t *testing.T, bboxes [][]float64) *Client {
		t.Helper()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/predict/dla" {
				t.Errorf("path = %q, want /predict/dla", r.URL.Path)
			}
			if err := json.NewEncoder(w).Encode(map[string]any{"bboxes": bboxes}); err != nil {
				// Handler runs in its own goroutine; fail the test, don't panic.
				t.Errorf("encode response: %v", err)
			}
		}))
		t.Cleanup(srv.Close)
		return mustNewDeepDocClient(t, srv.URL)
	}

	// bbox = [x0, y0, x1, y1, confidence, classId]; classId 2 = "reference".
	const referenceClass = 2

	t.Run("low_conf_reference_dropped", func(t *testing.T) {
		client := newClient(t, [][]float64{
			{50, 10, 500, 50, 0.30, referenceClass}, // reference, low confidence
		})
		regions, err := client.DLA(t.Context(), testImage())
		if err != nil {
			t.Fatal(err)
		}
		if len(regions) != 0 {
			t.Fatalf("got %d regions, want 0 (low-confidence 'reference' must be dropped by the 0.4 garbage gate)", len(regions))
		}
	})

	t.Run("high_conf_reference_kept", func(t *testing.T) {
		client := newClient(t, [][]float64{
			{50, 10, 500, 50, 0.90, referenceClass}, // reference, high confidence
		})
		regions, err := client.DLA(t.Context(), testImage())
		if err != nil {
			t.Fatal(err)
		}
		if len(regions) != 1 || regions[0].Label != "reference" {
			t.Fatalf("got %v, want exactly one 'reference' region (conf >= 0.4 is kept)", regions)
		}
	})

	t.Run("boundary_conf_0.4_kept", func(t *testing.T) {
		// Gate is strict (< 0.4); confidence exactly 0.4 is the boundary and kept.
		client := newClient(t, [][]float64{
			{50, 10, 500, 50, 0.40, referenceClass}, // reference, exactly 0.4
		})
		regions, err := client.DLA(t.Context(), testImage())
		if err != nil {
			t.Fatal(err)
		}
		if len(regions) != 1 || regions[0].Label != "reference" {
			t.Fatalf("got %v, want exactly one 'reference' region (conf == 0.4 is the gate boundary and kept)", regions)
		}
	})

	t.Run("low_conf_garbage_and_text", func(t *testing.T) {
		// A low-confidence reference is dropped while an unrelated text region
		// (non-garbage) is kept — the gate only removes garbage-layout regions
		// below 0.4; non-garbage regions pass through at any confidence.
		client := newClient(t, [][]float64{
			{50, 10, 500, 50, 0.30, referenceClass}, // reference, low confidence -> dropped
			{50, 100, 500, 300, 0.90, 1},            // text, high confidence -> kept
		})
		regions, err := client.DLA(t.Context(), testImage())
		if err != nil {
			t.Fatal(err)
		}
		if len(regions) != 1 {
			t.Fatalf("got %d regions, want 1 (low-confidence 'reference' dropped, 'text' kept)", len(regions))
		}
		if regions[0].Label != "text" {
			t.Errorf("regions[0].Label = %q, want 'text'", regions[0].Label)
		}
	})
}
