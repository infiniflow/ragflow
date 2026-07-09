package inference

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"io"
	"log/slog"
	"mime/multipart"
	"net"
	"net/http"
	"sync"
	"time"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
	util "ragflow/internal/deepdoc/parser/pdf/util"

	"github.com/cenkalti/backoff/v5"
)

// Client wraps the DeepDoc HTTP API.
type Client struct {
	baseURL    string
	httpClient *http.Client

	// Label tables for class_id → label string mapping.
	// Set by the service layer (model-specific) to reflect the model's taxonomy.
	DLALabels []string
	TSRLabels []string
}

// BaseURL returns the configured DeepDoc service URL.
func (c *Client) BaseURL() string { return c.baseURL }

// NewClient creates a client.  baseURL must be provided by the caller
// (e.g. from the DEEPDOC_URL environment variable).  Returns an error if empty.
func NewClient(baseURL string) (*Client, error) {
	if baseURL == "" {
		return nil, fmt.Errorf("deepdoc client: baseURL is required (set DEEPDOC_URL)")
	}
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
		DLALabels: DefaultDLALabels(),
		TSRLabels: DefaultTSRLabels(),
	}, nil
}

// DefaultDLALabels returns the 10-class DLA taxonomy matching Python's
// deepdoc/vision/dla_cli.py:10-21.  Duplicates at indices 4, 7, 9 are
// kept verbatim for backward compatibility with existing inference servers.
func DefaultDLALabels() []string {
	return []string{
		pdf.LayoutTypeTitle, pdf.LayoutTypeText, pdf.LayoutTypeReference,
		pdf.LayoutTypeFigure, pdf.DLALabelFigureCaption,
		pdf.LayoutTypeTable, pdf.DLALabelTableCaption, pdf.DLALabelTableCaption,
		pdf.LayoutTypeEquation, pdf.DLALabelFigureCaption,
	}
}

// DefaultTSRLabels returns the 6-class TSR taxonomy matching Python's
// deepdoc/server/adapters/tsr_adapter.py:21-26.
func DefaultTSRLabels() []string {
	return []string{
		"table", "table column", "table row",
		"table column header", "table projected row header",
		"table spanning cell",
	}
}

type bboxesResponse struct {
	BBoxes [][]float64 `json:"bboxes"`
}

// DLA analyzes a full page image and returns labeled regions.
func (c *Client) DLA(ctx context.Context, pageImage image.Image) ([]pdf.DLARegion, error) {
	data, err := util.EncodeJPEG(pageImage)
	if err != nil {
		return nil, fmt.Errorf("dla: encode: %w", err)
	}
	var resp bboxesResponse
	if err := c.post(ctx, "/predict/dla", data, "dla.jpeg", &resp); err != nil {
		return nil, fmt.Errorf("dla: %w", err)
	}
	regions := make([]pdf.DLARegion, 0, len(resp.BBoxes))
	for _, b := range resp.BBoxes {
		if len(b) < 6 {
			continue
		}
		labels := c.DLALabels
		label := ""
		if clsID := int(b[5]); clsID >= 0 && clsID < len(labels) {
			label = labels[clsID]
		}
		regions = append(regions, pdf.DLARegion{
			X0: b[0], Y0: b[1], X1: b[2], Y1: b[3],
			Confidence: b[4],
			Label:      label,
		})
	}
	return regions, nil
}

// TSR recognises table structure from a cropped image.
func (c *Client) TSR(ctx context.Context, cropped image.Image) ([]pdf.TSRCell, error) {
	data, err := util.EncodeJPEG(cropped)
	if err != nil {
		return nil, fmt.Errorf("tsr: encode: %w", err)
	}
	var resp bboxesResponse
	if err := c.post(ctx, "/predict/tsr", data, "tsr.jpeg", &resp); err != nil {
		return nil, fmt.Errorf("tsr: %w", err)
	}
	cells := make([]pdf.TSRCell, 0, len(resp.BBoxes))
	for _, b := range resp.BBoxes {
		if len(b) < 5 {
			continue
		}
		tlabels := c.TSRLabels
		label := ""
		if len(b) >= 6 {
			if cls := int(b[5]); cls >= 0 && cls < len(tlabels) {
				label = tlabels[cls]
			}
		}
		cells = append(cells, pdf.TSRCell{
			X0: b[0], Y0: b[1], X1: b[2], Y1: b[3],
			Label: label,
		})
	}
	return cells, nil
}

// ocrDetectResponse matches DeepDoc /predict/ocr?operator=det output:
//
//	{"output": [[[[[[x0,y0],[x1,y1],[x2,y2],[x3,y3]], ...]]]]}
type ocrDetectResponse struct {
	Output [][][][][]float64 `json:"output"`
}

// ocrRecognizeResponse matches DeepDoc /predict/ocr?operator=rec output:
//
//	{"output": [[[["text", confidence], ...]]]}
type ocrRecognizeResponse struct {
	Output [][][][]any `json:"output"`
}

// OCRDetect detects text regions (bounding boxes) in an image.
// DeepDoc /predict/ocr with operator=det returns quad boxes: [[[x0,y0],[x1,y1],[x2,y2],[x3,y3]], ...]
func (c *Client) OCRDetect(ctx context.Context, cropped image.Image) ([]pdf.OCRBox, error) {
	data, err := util.EncodeJPEG(cropped)
	if err != nil {
		return nil, fmt.Errorf("ocr detect: encode: %w", err)
	}

	// First decode outer envelope as RawMessage so we can log on format mismatch.
	var rawEnvelope struct {
		Output json.RawMessage `json:"output"`
	}
	if err := c.post(ctx, "/predict/ocr", data, "ocr_detect.jpeg", &rawEnvelope, "operator", "det"); err != nil {
		return nil, fmt.Errorf("ocr detect: %w", err)
	}

	var result ocrDetectResponse
	if err := json.Unmarshal(rawEnvelope.Output, &result.Output); err != nil {
		rawStr := string(rawEnvelope.Output)
		if len(rawStr) > 1000 {
			rawStr = rawStr[:1000]
		}
		slog.Warn("ocr detect: output format mismatch", "err", err, "raw_output", rawStr)
		return nil, fmt.Errorf("ocr detect: %w", err)
	}

	var boxes []pdf.OCRBox
	for _, outer := range result.Output {
		for _, page := range outer {
			for _, box := range page {
				if len(box) < 4 {
					continue
				}
				boxes = append(boxes, pdf.OCRBox{
					X0: box[0][0], Y0: box[0][1],
					X1: box[1][0], Y1: box[1][1],
					X2: box[2][0], Y2: box[2][1],
					X3: box[3][0], Y3: box[3][1],
				})
			}
		}
	}
	return boxes, nil
}

// OCRRecognize recognizes text in a cropped image region.
// DeepDoc /predict/ocr with operator=rec returns [[["text", confidence], ...]]
func (c *Client) OCRRecognize(ctx context.Context, cropped image.Image) ([]pdf.OCRText, error) {
	data, err := util.EncodeJPEG(cropped)
	if err != nil {
		return nil, fmt.Errorf("ocr rec: encode: %w", err)
	}
	var result ocrRecognizeResponse
	if err := c.post(ctx, "/predict/ocr", data, "ocr_rec.jpeg", &result, "operator", "rec"); err != nil {
		return nil, fmt.Errorf("ocr rec: %w", err)
	}
	var texts []pdf.OCRText
	for _, page := range result.Output {
		for _, item := range page {
			for _, pair := range item {
				if len(pair) >= 2 {
					text, _ := pair[0].(string)
					conf, _ := pair[1].(float64)
					texts = append(texts, pdf.OCRText{Text: text, Confidence: conf})
				}
			}
		}
	}
	return texts, nil
}

// OCRRecognizeBatch recognizes text in multiple cropped image regions.
// Returns a slice of results and a parallel slice of errors (nil on success).
// A nil cropped image in the input produces nil results and a non-nil error.
func (c *Client) OCRRecognizeBatch(ctx context.Context, cropped []image.Image) ([][]pdf.OCRText, []error) {
	results := make([][]pdf.OCRText, len(cropped))
	errs := make([]error, len(cropped))

	// Process images concurrently with a bounded worker pool to avoid
	// overwhelming the DeepDoc service.
	const maxConcurrent = 4
	sem := make(chan struct{}, maxConcurrent)
	var wg sync.WaitGroup

	for i, img := range cropped {
		if img == nil {
			errs[i] = fmt.Errorf("ocr rec batch: image[%d] is nil", i)
			continue
		}
		wg.Add(1)
		go func(idx int, im image.Image) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			texts, err := c.OCRRecognize(ctx, im)
			results[idx] = texts
			errs[idx] = err
		}(i, img)
	}
	wg.Wait()
	return results, errs
}

// Health checks whether the DeepDoc service is reachable.
func (c *Client) Health() bool {
	resp, err := c.httpClient.Get(c.baseURL + "/health")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == 200
}

func (c *Client) post(ctx context.Context, endpoint string, imgData []byte, filename string, result interface{}, extraFields ...string) error {
	// Build multipart body once — the image data is idempotent.
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	fw, err := w.CreateFormFile("request", filename)
	if err != nil {
		return err
	}
	if _, err := fw.Write(imgData); err != nil {
		return err
	}
	for i := 0; i+1 < len(extraFields); i += 2 {
		w.WriteField(extraFields[i], extraFields[i+1])
	}
	w.Close()
	contentType := w.FormDataContentType()
	bodyBytes := body.Bytes()

	_, err = backoff.Retry(ctx, func() (struct{}, error) {
		req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+endpoint, bytes.NewReader(bodyBytes))
		if err != nil {
			return struct{}{}, backoff.Permanent(err)
		}
		req.Header.Set("Content-Type", contentType)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return struct{}{}, backoff.Permanent(err)
			}
			var netErr net.Error
			if errors.As(err, &netErr) {
				slog.Warn("deepdoc: network error, will retry", "endpoint", endpoint, "err", err)
				return struct{}{}, err
			}
			return struct{}{}, backoff.Permanent(err)
		}

		if resp.StatusCode == 200 {
			defer resp.Body.Close()
			return struct{}{}, json.NewDecoder(io.LimitReader(resp.Body, 64<<20)).Decode(result)
		}

		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		respErr := fmt.Errorf("http %d: %s", resp.StatusCode, string(errBody[:min(200, len(errBody))]))

		if resp.StatusCode >= 500 {
			slog.Warn("deepdoc: server error, will retry", "endpoint", endpoint, "status", resp.StatusCode)
			return struct{}{}, respErr
		}
		// 4xx and other codes are not retryable.
		return struct{}{}, backoff.Permanent(respErr)
	}, backoff.WithMaxTries(4), backoff.WithNotify(func(err error, d time.Duration) {
		slog.Info("deepdoc: retrying", "endpoint", endpoint, "backoff", d.Round(time.Millisecond), "err", err)
	}))
	return err
}
