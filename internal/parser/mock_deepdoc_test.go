package parser

import (
	"fmt"
	"image"
)

// MockDocAnalyzer returns predefined data for unit tests.
type MockDocAnalyzer struct {
	DLARegions []DLARegion
	TSRCells   []TSRCell
	OCRBoxes   []OCRBox
	OCRTexts     []OCRText
	// OCRBatchTexts returns per-image texts for OCRRecognizeBatch.
	// If nil, OCRTexts is returned for every image.
	OCRBatchTexts [][]OCRText
	// OCRBatchErr makes OCRRecognizeBatch return an error for image i.
	OCRBatchErr func(i int) error
	Healthy      bool
}

func (m *MockDocAnalyzer) DLA(image.Image) ([]DLARegion, error)   { return m.DLARegions, nil }
func (m *MockDocAnalyzer) TSR(image.Image) ([]TSRCell, error)     { return m.TSRCells, nil }
func (m *MockDocAnalyzer) OCRDetect(image.Image) ([]OCRBox, error) { return m.OCRBoxes, nil }
func (m *MockDocAnalyzer) OCRRecognize(image.Image) ([]OCRText, error) {
	return m.OCRTexts, nil
}
func (m *MockDocAnalyzer) OCRRecognizeBatch(cropped []image.Image) ([][]OCRText, []error) {
	results := make([][]OCRText, len(cropped))
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
