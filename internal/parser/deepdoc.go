package parser

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"sync"
	"time"
)

// DeepDocClient wraps the DeepDoc HTTP API.
type DeepDocClient struct {
	baseURL    string
	httpClient *http.Client
	modelOnce  sync.Once
	model      ModelType

	// Label tables for class_id → label string mapping.
	// Set by the service layer (Oss/Saas) to reflect the model's taxonomy.
	DLALabels []string
	TSRLabels []string
}

// NewDeepDocClient creates a client.  baseURL must be provided by the caller
// (e.g. from the DEEPDOC_URL environment variable).  Returns an error if empty.
func NewDeepDocClient(baseURL string) (*DeepDocClient, error) {
	if baseURL == "" {
		return nil, fmt.Errorf("deepdoc client: baseURL is required (set DEEPDOC_URL)")
	}
	return &DeepDocClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}, nil
}

// Default DLA/TSR label tables.  Service constructors replace these with
// model-specific labels (OSS 6-class TSR, SaaS 2-class, etc.).
var defaultDLALabels = []string{
	"title", "text", "reference", "figure", "figure caption",
	"table", "table caption", "table caption", "equation", "figure caption",
}
var defaultTSRLabels = []string{
	"table", "table column", "table row",
	"table column header", "table projected row header",
	"table spanning cell",
}

type bboxesResponse struct {
	BBoxes [][]float64 `json:"bboxes"`
}

// DLA analyses a full page image and returns labelled regions.
func (c *DeepDocClient) DLA(ctx context.Context, pageImage image.Image) ([]DLARegion, error) {
	data, err := encodeJPEG(pageImage)
	if err != nil {
		return nil, fmt.Errorf("dla: encode: %w", err)
	}
	var resp bboxesResponse
	if err := c.post(ctx, "/predict/dla", data, "dla.jpeg", &resp); err != nil {
		return nil, fmt.Errorf("dla: %w", err)
	}
	regions := make([]DLARegion, 0, len(resp.BBoxes))
	for _, b := range resp.BBoxes {
		if len(b) < 6 {
			continue
		}
		labels := c.DLALabels
		if labels == nil {
			labels = defaultDLALabels
		}
		label := ""
		if clsID := int(b[5]); clsID >= 0 && clsID < len(labels) {
			label = labels[clsID]
		}
		regions = append(regions, DLARegion{
			X0: b[0], Y0: b[1], X1: b[2], Y1: b[3],
			Confidence: b[4],
			Label:      label,
		})
	}
	return regions, nil
}

// TSR recognises table structure from a cropped image.
func (c *DeepDocClient) TSR(ctx context.Context, cropped image.Image) ([]TSRCell, error) {
	data, err := encodeJPEG(cropped)
	if err != nil {
		return nil, fmt.Errorf("tsr: encode: %w", err)
	}
	var resp bboxesResponse
	if err := c.post(ctx, "/predict/tsr", data, "tsr.jpeg", &resp); err != nil {
		return nil, fmt.Errorf("tsr: %w", err)
	}
	cells := make([]TSRCell, 0, len(resp.BBoxes))
	for _, b := range resp.BBoxes {
		if len(b) < 5 {
			continue
		}
		tlabels := c.TSRLabels
		if tlabels == nil {
			tlabels = defaultTSRLabels
		}
		label := ""
		if len(b) >= 6 {
			if cls := int(b[5]); cls >= 0 && cls < len(tlabels) {
				label = tlabels[cls]
			}
		}
		cells = append(cells, TSRCell{
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
func (c *DeepDocClient) OCRDetect(ctx context.Context, cropped image.Image) ([]OCRBox, error) {
	data, err := encodeJPEG(cropped)
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

	var boxes []OCRBox
	for _, outer := range result.Output {
		for _, page := range outer {
			for _, box := range page {
				if len(box) < 4 {
					continue
				}
				boxes = append(boxes, OCRBox{
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
func (c *DeepDocClient) OCRRecognize(ctx context.Context, cropped image.Image) ([]OCRText, error) {
	data, err := encodeJPEG(cropped)
	if err != nil {
		return nil, fmt.Errorf("ocr rec: encode: %w", err)
	}
	var result ocrRecognizeResponse
	if err := c.post(ctx, "/predict/ocr", data, "ocr_rec.jpeg", &result, "operator", "rec"); err != nil {
		return nil, fmt.Errorf("ocr rec: %w", err)
	}
	var texts []OCRText
	for _, page := range result.Output {
		for _, item := range page {
			for _, pair := range item {
				if len(pair) >= 2 {
					text, _ := pair[0].(string)
					conf, _ := pair[1].(float64)
					texts = append(texts, OCRText{Text: text, Confidence: conf})
				}
			}
		}
	}
	return texts, nil
}

// OCRRecognizeBatch recognizes text in multiple cropped image regions.
// Returns a slice of results and a parallel slice of errors (nil on success).
// A nil cropped image in the input produces nil results and a non-nil error.
func (c *DeepDocClient) OCRRecognizeBatch(ctx context.Context, cropped []image.Image) ([][]OCRText, []error) {
	results := make([][]OCRText, len(cropped))
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
func (c *DeepDocClient) Health() bool {
	resp, err := c.httpClient.Get(c.baseURL + "/health")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == 200
}

// ModelType probes the DeepDoc /model endpoint once and caches the model flavour.
// The /model endpoint is expected to return JSON like {"model":"oss","version":"1.0"}.
// When the endpoint is unreachable or model is not "oss", ModelSaas is returned.
// Uses sync.Once so the call is safe for concurrent use.
func (c *DeepDocClient) ModelType() ModelType {
	c.modelOnce.Do(func() {
		c.model = ModelSaas
		resp, err := c.httpClient.Get(c.baseURL + "/model")
		if err != nil {
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return
		}
		var h struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&h); err != nil {
			slog.Warn("deepdoc /model: failed to decode response, falling back to SaaS",
				"err", err)
			return
		}
		if h.Model == "oss" {
			c.model = ModelOSS
		}
	})
	return c.model
}

// NewTableBuilderFor creates the right TableBuilder for the given
// DocAnalyzer, chosen by ModelType().
func NewTableBuilderFor(doc DocAnalyzer) TableBuilder {
	switch doc.ModelType() {
	case ModelOSS:
		return NewOssDeepDocService(doc)
	default:
		return NewSaasDeepDocService(doc)
	}
}

func (c *DeepDocClient) post(ctx context.Context, endpoint string, imgData []byte, filename string, result interface{}, extraFields ...string) error {
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

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+endpoint, &body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		s := string(b)
		if len(s) > 200 {
			s = s[:200]
		}
		return fmt.Errorf("http %d: %s", resp.StatusCode, s)
	}
	// Guard against unbounded response bodies (malicious / misconfigured
	// server).  64 MiB is far beyond any legitimate DLA / TSR / OCR payload.
	return json.NewDecoder(io.LimitReader(resp.Body, 64<<20)).Decode(result)
}
