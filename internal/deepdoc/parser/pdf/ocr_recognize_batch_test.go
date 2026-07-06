//go:build cgo

package pdf

import (
	"context"
	"errors"
	"image"
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
	"testing"
)

func TestOCRRecognizeBatch_EmptyList(t *testing.T) {
	mock := &MockDocAnalyzer{Healthy: true}
	results, errs := mock.OCRRecognizeBatch(context.Background(), nil)
	if len(results) != 0 {
		t.Errorf("nil input: expected 0 results, got %d", len(results))
	}
	if len(errs) != 0 {
		t.Errorf("nil input: expected 0 errs, got %d", len(errs))
	}
	results, errs = mock.OCRRecognizeBatch(context.Background(), []image.Image{})
	if len(results) != 0 || len(errs) != 0 {
		t.Error("empty input: expected 0 results/errs")
	}
}

func TestOCRRecognizeBatch_SingleImage(t *testing.T) {
	mock := &MockDocAnalyzer{
		Healthy:  true,
		OCRTexts: []pdf.OCRText{{Text: "hello", Confidence: 0.9}},
	}
	dummy := image.NewRGBA(image.Rect(0, 0, 10, 10))
	results, errs := mock.OCRRecognizeBatch(context.Background(), []image.Image{dummy})
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if len(results[0]) != 1 || results[0][0].Text != "hello" {
		t.Errorf("expected 'hello', got %v", results[0])
	}
	if errs[0] != nil {
		t.Errorf("expected nil err, got %v", errs[0])
	}
}

func TestOCRRecognizeBatch_MultipleImages(t *testing.T) {
	mock := &MockDocAnalyzer{
		Healthy: true,
		OCRBatchTexts: [][]pdf.OCRText{
			{{Text: "img0", Confidence: 0.9}},
			{{Text: "img1", Confidence: 0.8}},
			{{Text: "img2", Confidence: 0.7}},
		},
	}
	dummy := image.NewRGBA(image.Rect(0, 0, 10, 10))
	results, errs := mock.OCRRecognizeBatch(context.Background(), []image.Image{dummy, dummy, dummy})
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	for i, want := range []string{"img0", "img1", "img2"} {
		if len(results[i]) != 1 || results[i][0].Text != want {
			t.Errorf("image[%d]: expected %q, got %v", i, want, results[i])
		}
		if errs[i] != nil {
			t.Errorf("image[%d]: expected nil err, got %v", i, errs[i])
		}
	}
}

func TestOCRRecognizeBatch_NilImage(t *testing.T) {
	mock := &MockDocAnalyzer{
		Healthy:  true,
		OCRTexts: []pdf.OCRText{{Text: "ok", Confidence: 0.9}},
	}
	dummy := image.NewRGBA(image.Rect(0, 0, 10, 10))
	results, errs := mock.OCRRecognizeBatch(context.Background(), []image.Image{dummy, nil, dummy})
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	if len(results[0]) == 0 || results[0][0].Text != "ok" {
		t.Errorf("image[0]: expected 'ok', got %v", results[0])
	}
	if results[1] != nil {
		t.Errorf("image[1]: nil image should get nil result, got %v", results[1])
	}
	if errs[1] == nil {
		t.Error("image[1]: nil image should get error")
	}
	if len(results[2]) == 0 || results[2][0].Text != "ok" {
		t.Errorf("image[2]: expected 'ok' after nil, got %v", results[2])
	}
}

func TestOCRRecognizeBatch_ErrorHandling(t *testing.T) {
	mock := &MockDocAnalyzer{
		Healthy:  true,
		OCRTexts: []pdf.OCRText{{Text: "ok", Confidence: 0.9}},
		OCRBatchErr: func(i int) error {
			if i == 1 {
				return errors.New("simulated error")
			}
			return nil
		},
	}
	dummy := image.NewRGBA(image.Rect(0, 0, 10, 10))
	results, errs := mock.OCRRecognizeBatch(context.Background(), []image.Image{dummy, dummy, dummy})
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	// Image 0: OK
	if errs[0] != nil {
		t.Errorf("image[0]: expected nil err, got %v", errs[0])
	}
	// Image 1: error
	if errs[1] == nil {
		t.Error("image[1]: expected error")
	}
	// Image 2: OK (error only for index 1)
	if errs[2] != nil {
		t.Errorf("image[2]: expected nil err, got %v", errs[2])
	}
	// Results should still be returned alongside errors
	if results[0] == nil || results[0][0].Text != "ok" {
		t.Error("image[0]: result should be returned despite error on other image")
	}
	if results[2] == nil || results[2][0].Text != "ok" {
		t.Error("image[2]: result should be returned despite error on other image")
	}
}

func TestOCRRecognizeBatch_EmptyText(t *testing.T) {
	mock := &MockDocAnalyzer{
		Healthy:  true,
		OCRTexts: []pdf.OCRText{}, // empty — simulate no text recognized
	}
	dummy := image.NewRGBA(image.Rect(0, 0, 10, 10))
	results, errs := mock.OCRRecognizeBatch(context.Background(), []image.Image{dummy})
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if len(results[0]) != 0 {
		t.Errorf("expected empty texts, got %v", results[0])
	}
	if errs[0] != nil {
		t.Errorf("expected nil err for empty text, got %v", errs[0])
	}
}

func TestOCRRecognizeBatch_FallbackToOCRTexts(t *testing.T) {
	// When OCRBatchTexts is nil, fall back to OCRTexts for every image.
	mock := &MockDocAnalyzer{
		Healthy:  true,
		OCRTexts: []pdf.OCRText{{Text: "default", Confidence: 0.5}},
	}
	dummy := image.NewRGBA(image.Rect(0, 0, 10, 10))
	results, errs := mock.OCRRecognizeBatch(context.Background(), []image.Image{dummy, dummy, dummy})
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	for i := 0; i < 3; i++ {
		if len(results[i]) != 1 || results[i][0].Text != "default" {
			t.Errorf("image[%d]: expected 'default', got %v", i, results[i])
		}
		if errs[i] != nil {
			t.Errorf("image[%d]: expected nil err, got %v", i, errs[i])
		}
	}
}

func TestOCRRecognizeBatch_PartialBatchTexts(t *testing.T) {
	// OCRBatchTexts shorter than images — remaining fall back to OCRTexts.
	mock := &MockDocAnalyzer{
		Healthy:  true,
		OCRTexts: []pdf.OCRText{{Text: "fallback", Confidence: 0.5}},
		OCRBatchTexts: [][]pdf.OCRText{
			{{Text: "custom0", Confidence: 0.9}},
		},
	}
	dummy := image.NewRGBA(image.Rect(0, 0, 10, 10))
	results, errs := mock.OCRRecognizeBatch(context.Background(), []image.Image{dummy, dummy, dummy})
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	if results[0][0].Text != "custom0" {
		t.Errorf("image[0]: expected 'custom0', got %q", results[0][0].Text)
	}
	if results[1][0].Text != "fallback" {
		t.Errorf("image[1]: expected 'fallback', got %q", results[1][0].Text)
	}
	if results[2][0].Text != "fallback" {
		t.Errorf("image[2]: expected 'fallback', got %q", results[2][0].Text)
	}
	if errs[0] != nil || errs[1] != nil || errs[2] != nil {
		t.Error("all errors should be nil")
	}
}
