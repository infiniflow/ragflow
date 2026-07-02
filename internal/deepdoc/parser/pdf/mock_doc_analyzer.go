package pdf

import (
	"context"
	"fmt"
	"image"
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// MockDocAnalyzer returns predefined data for unit tests.
// Set an Err field to non-nil to exercise the corresponding error path.
type MockDocAnalyzer struct {
	DLARegions []pdf.DLARegion
	TSRCells   []pdf.TSRCell
	OCRBoxes   []pdf.OCRBox
	OCRTexts   []pdf.OCRText
	// OCRBatchTexts returns per-image texts for OCRRecognizeBatch.
	// If nil, OCRTexts is returned for every image.
	OCRBatchTexts [][]pdf.OCRText
	// OCRBatchErr makes OCRRecognizeBatch return an error for image i.
	OCRBatchErr func(i int) error
	// Per-method error injection for testing failure paths.
	DLAErr          error
	TSRErr          error
	OCRDetectErr    error
	OCRRecognizeErr error

	Healthy bool
}

func (m *MockDocAnalyzer) DLA(_ context.Context, _ image.Image) ([]pdf.DLARegion, error) {
	if m.DLAErr != nil {
		return nil, m.DLAErr
	}
	return m.DLARegions, nil
}
func (m *MockDocAnalyzer) TSR(_ context.Context, _ image.Image) ([]pdf.TSRCell, error) {
	if m.TSRErr != nil {
		return nil, m.TSRErr
	}
	return m.TSRCells, nil
}
func (m *MockDocAnalyzer) OCRDetect(_ context.Context, _ image.Image) ([]pdf.OCRBox, error) {
	if m.OCRDetectErr != nil {
		return nil, m.OCRDetectErr
	}
	return m.OCRBoxes, nil
}
func (m *MockDocAnalyzer) OCRRecognize(_ context.Context, _ image.Image) ([]pdf.OCRText, error) {
	if m.OCRRecognizeErr != nil {
		return nil, m.OCRRecognizeErr
	}
	return m.OCRTexts, nil
}
func (m *MockDocAnalyzer) OCRRecognizeBatch(_ context.Context, cropped []image.Image) ([][]pdf.OCRText, []error) {
	results := make([][]pdf.OCRText, len(cropped))
	errs := make([]error, len(cropped))
	for i, img := range cropped {
		if img == nil {
			errs[i] = fmt.Errorf("image[%d] is nil", i)
			continue
		}
		if m.OCRBatchErr != nil {
			errs[i] = m.OCRBatchErr(i)
		}
		if m.OCRBatchTexts != nil && i < len(m.OCRBatchTexts) {
			results[i] = m.OCRBatchTexts[i]
		} else {
			results[i] = m.OCRTexts
		}
	}
	return results, errs
}
func (m *MockDocAnalyzer) Health() bool { return m.Healthy }
