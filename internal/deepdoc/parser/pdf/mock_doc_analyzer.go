package pdf

import (
	"context"
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
func (m *MockDocAnalyzer) Health() bool { return m.Healthy }
