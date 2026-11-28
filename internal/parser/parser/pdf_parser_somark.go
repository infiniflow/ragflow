package parser

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"time"

	pdfreader "github.com/ledongthuc/pdf"

	"ragflow/internal/common"
	"ragflow/internal/parser/media"
	"ragflow/internal/utility"
)

const (
	// defaultSoMarkPollTimeout bounds how long soMarkPoll waits for an async
	// task. SoMark parses on a remote service whose own task timeout
	// (EXTRACT_MAX_TIMEOUT) defaults to 24h, so a short hard-coded client
	// window would fail valid large PDFs while the service still considers
	// them in flight. SOMARK_POLL_TIMEOUT (Go duration, e.g. "30m") overrides
	// the default, and a tighter caller context deadline still wins over both.
	defaultSoMarkPollTimeout = 30 * time.Minute
	// maxSoMarkPollFailures caps consecutive transient poll failures. A single
	// blip must not abort the parse — absorbing exactly that flakiness is why
	// the poll loop exists — but a persistently broken endpoint should fail
	// instead of retrying until the deadline.
	maxSoMarkPollFailures = 5
)

// soMarkPollInterval is the delay between /parse/async_check polls. It is a
// variable only so tests can shorten it; production code never mutates it.
var soMarkPollInterval = 2 * time.Second

// parsePDFWithSoMark submits a PDF and converts the task result into parser
// output. Explicit tenant configuration never borrows a global endpoint or key.
func parsePDFWithSoMark(ctx context.Context, filename string, data []byte, parser *PDFParser) ParseResult {
	if len(data) == 0 {
		return emptyPDFResult(filename)
	}
	baseURL := strings.TrimSpace(parser.SoMarkBaseURL)
	if baseURL == "" && !parser.UseTenantModelConfig {
		baseURL = strings.TrimSpace(common.GetEnv(common.EnvSOMarkBaseUrl))
	}
	if baseURL == "" {
		return ParseResult{Err: fmt.Errorf("parser: SoMark requires somark_base_url or SOMARK_BASE_URL")}
	}
	apiKey := parser.SoMarkAPIKey
	if strings.TrimSpace(apiKey) == "" && !parser.UseTenantModelConfig {
		apiKey = strings.TrimSpace(common.GetEnv(common.EnvSOMarkAPIKey))
	}
	taskID, err := soMarkSubmit(ctx, strings.TrimRight(baseURL, "/"), filename, data, parser, apiKey)
	if err != nil {
		return ParseResult{Err: err}
	}
	result, err := soMarkPoll(ctx, strings.TrimRight(baseURL, "/"), taskID, apiKey)
	if err != nil {
		return ParseResult{Err: err}
	}
	items, pageCount, warnings := soMarkItems(ctx, result, parser.SoMarkKeepHeaderFooter, soMarkPagePointSizes(data))
	if err := ctx.Err(); err != nil {
		return ParseResult{Err: err}
	}
	if len(items) == 0 {
		return ParseResult{Err: fmt.Errorf("parser: SoMark returned no usable blocks")}
	}
	parsed := pdfItemsToResult(filename, items, pageCount)
	parsed.Warnings = warnings
	return parsed
}

// soMarkSubmit uploads a PDF with the supplied credentials and parser options,
// applying environment defaults only when no tenant instance was selected.
func soMarkSubmit(ctx context.Context, baseURL, filename string, data []byte, parser *PDFParser, apiKey string) (string, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return "", fmt.Errorf("parser: SoMark create file: %w", err)
	}
	if _, err := part.Write(data); err != nil {
		return "", fmt.Errorf("parser: SoMark write file: %w", err)
	}
	_ = writer.WriteField("output_formats", "json")
	elementFormats := utility.MustJSON(map[string]any{
		"image":   parser.envOrDefault(common.EnvSOMarkImageFormat, parser.SoMarkImageFormat, "url"),
		"formula": parser.envOrDefault(common.EnvSOMarkFormulaFormat, parser.SoMarkFormulaFormat, "latex"),
		"table":   parser.envOrDefault(common.EnvSOMarkTableFormat, parser.SoMarkTableFormat, "html"),
		"cs":      parser.envOrDefault(common.EnvSOMarkCSFormat, parser.SoMarkCSFormat, "image"),
	})
	featureConfig := utility.MustJSON(map[string]any{
		"enable_text_cross_page":         parser.envOrBool(common.EnvSOMarkEnableTextCrossPage, parser.SoMarkEnableTextCrossPage),
		"enable_table_cross_page":        parser.envOrBool(common.EnvSOMarkEnableTableCrossPage, parser.SoMarkEnableTableCrossPage),
		"enable_title_level_recognition": parser.envOrBool(common.EnvSOMarkEnableTitleLevelRecognition, parser.SoMarkEnableTitleLevelRecognition),
		"enable_inline_image":            parser.envOrBool(common.EnvSOMarkEnableInlineImage, parser.SoMarkEnableInlineImage),
		"enable_table_image":             parser.envOrBool(common.EnvSOMarkEnableTableImage, parser.SoMarkEnableTableImage),
		"enable_image_understanding":     parser.envOrBool(common.EnvSOMarkEnableImageUnderstanding, parser.SoMarkEnableImageUnderstanding),
		"keep_header_footer":             parser.envOrBool(common.EnvSOMarkKeepHeaderFooter, parser.SoMarkKeepHeaderFooter),
	})
	_ = writer.WriteField("element_formats", elementFormats)
	_ = writer.WriteField("feature_config", featureConfig)
	if apiKey != "" {
		_ = writer.WriteField("api_key", apiKey)
	}
	if err = writer.Close(); err != nil {
		return "", fmt.Errorf("parser: SoMark finalize form: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/parse/async", &body)
	if err != nil {
		return "", fmt.Errorf("parser: SoMark request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := common.GetSSRFHTTPClient().Do(req)
	if err != nil {
		return "", fmt.Errorf("parser: SoMark submit: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("parser: SoMark read submit: %w", err)
	}
	var payload struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			TaskID string `json:"task_id"`
		} `json:"data"`
	}
	if err = json.Unmarshal(raw, &payload); err != nil {
		return "", fmt.Errorf("parser: SoMark decode submit: %w", err)
	}
	if payload.Code != 0 {
		return "", fmt.Errorf("parser: SoMark submit business error code=%d message=%s", payload.Code, payload.Message)
	}
	if payload.Data.TaskID == "" {
		return "", fmt.Errorf("parser: SoMark submit returned no task_id")
	}
	return payload.Data.TaskID, nil
}

// soMarkPollTimeout reads the poll budget. An unset or malformed
// SOMARK_POLL_TIMEOUT degrades to the default: a typo must not disable the
// timeout or silently turn it into zero.
func soMarkPollTimeout() time.Duration {
	raw := strings.TrimSpace(common.GetEnv(common.EnvSOMarkPollTimeout))
	if raw == "" {
		return defaultSoMarkPollTimeout
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return defaultSoMarkPollTimeout
	}
	return d
}

// soMarkStatus is the classified state of an async task under SoMark's
// explicit status model.
type soMarkStatus int

const (
	soMarkStatusInProgress soMarkStatus = iota
	soMarkStatusSuccess
	soMarkStatusFailed
	soMarkStatusUnknown
)

// classifySoMarkStatus maps a raw status onto SoMark's explicit state model:
// only the documented in-progress states keep polling, SUCCESS/FAILED are
// terminal, and anything else is a protocol error rather than an implicit
// "keep polling until the deadline". An empty status lands here too.
//
// PENDING and QUEUEING (double E) are kept as defensive aliases even though
// the documented set is WAITING/QUEUING/CONVERTING/PROCESSING: PENDING is what
// the implementation merged in #21710 reports, and QUEUEING is a plausible
// spelling from a proxy that rewrites the payload. Treating either as unknown
// would turn a live task into an immediate protocol error; the deadline still
// bounds how long an alias can keep the loop running.
func classifySoMarkStatus(status string) soMarkStatus {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "SUCCESS":
		return soMarkStatusSuccess
	case "WAITING", "QUEUING", "QUEUEING", "PENDING", "CONVERTING", "PROCESSING", "RUNNING":
		return soMarkStatusInProgress
	case "FAILED", "FAILURE", "CANCELED", "CANCELLED", "ERROR":
		return soMarkStatusFailed
	}
	return soMarkStatusUnknown
}

// soMarkPollError marks whether a poll failure is worth retrying. Transport
// errors, 429/5xx, non-JSON bodies (e.g. an HTML 502 from a proxy), and
// business errors are transient. Business errors are deliberately retryable:
// SoMark returns the same code (e.g. 1133) both for a task that is not visible
// yet ("task not found") and for a genuinely bad request ("invalid PDF"), so
// trusting them as terminal would abort the parse on a task that is still
// being registered.
type soMarkPollError struct {
	err       error
	transient bool
}

func (e *soMarkPollError) Error() string { return e.err.Error() }
func (e *soMarkPollError) Unwrap() error { return e.err }

func soMarkTransientError(err error) error { return &soMarkPollError{err: err, transient: true} }
func soMarkTerminalError(err error) error  { return &soMarkPollError{err: err} }

// soMarkPoll polls /parse/async_check until the task reaches a terminal state,
// the deadline expires, or ctx is cancelled. SoMark submission is asynchronous
// (POST /parse/async), so a single check almost never sees SUCCESS.
func soMarkPoll(ctx context.Context, baseURL, taskID, apiKey string) (map[string]any, error) {
	// context.WithTimeout yields the earlier of this budget and a caller
	// deadline, so a tighter caller context is honoured as-is.
	pollCtx, cancel := context.WithTimeout(ctx, soMarkPollTimeout())
	defer cancel()

	wait := func() error {
		select {
		case <-pollCtx.Done():
			return pollCtx.Err()
		case <-time.After(soMarkPollInterval):
			return nil
		}
	}

	// The task is queued by definition right after submit, so an immediate
	// check would spend a request on a status that cannot have moved yet.
	if err := wait(); err != nil {
		return nil, soMarkPollDeadlineError(taskID, "", err)
	}

	failures := 0
	lastStatus := ""
	for {
		result, status, err := soMarkPollOnce(pollCtx, baseURL, taskID, apiKey)
		if err != nil {
			var pollErr *soMarkPollError
			if !errors.As(err, &pollErr) || !pollErr.transient {
				// When the deadline or a cancellation ended the in-flight
				// request, report the standardized timeout/cancel error (with
				// the last status) instead of the raw transport error, so
				// callers see the same wording as the wait-path timeout.
				if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
					return nil, soMarkPollDeadlineError(taskID, lastStatus, pollCtx.Err())
				}
				return nil, err
			}
			failures++
			if failures >= maxSoMarkPollFailures {
				return nil, fmt.Errorf("parser: SoMark task %s: %w (after %d consecutive transient failures)", taskID, err, failures)
			}
		} else {
			failures = 0
			lastStatus = status
			if result != nil {
				return result, nil
			}
		}
		if err := wait(); err != nil {
			return nil, soMarkPollDeadlineError(taskID, lastStatus, err)
		}
	}
}

// soMarkPollDeadlineError distinguishes a cancelled poll from one that ran out
// of budget, so the caller can tell "user went away" from "SoMark too slow".
func soMarkPollDeadlineError(taskID, lastStatus string, err error) error {
	if errors.Is(err, context.Canceled) {
		return fmt.Errorf("parser: SoMark task %s cancelled (last status=%q): %w", taskID, lastStatus, err)
	}
	return fmt.Errorf("parser: SoMark task %s timed out (last status=%q): %w", taskID, lastStatus, err)
}

// soMarkPollOnce performs a single /parse/async_check. On success it returns
// the result map; while the task is in progress it returns the raw status and
// a nil result. Errors say whether they are transient (retry) or terminal.
func soMarkPollOnce(ctx context.Context, baseURL, taskID, apiKey string) (result map[string]any, status string, err error) {
	form := url.Values{"task_id": {taskID}}
	if apiKey != "" {
		form.Set("api_key", apiKey)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/parse/async_check", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, "", soMarkTerminalError(fmt.Errorf("parser: SoMark poll request: %w", err))
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := common.GetSSRFHTTPClient().Do(req)
	if err != nil {
		if ctx.Err() != nil {
			// The caller's context, not the network, ended the request.
			return nil, "", soMarkTerminalError(fmt.Errorf("parser: SoMark poll: %w", err))
		}
		return nil, "", soMarkTransientError(fmt.Errorf("parser: SoMark poll: %w", err))
	}
	defer resp.Body.Close()
	// Classify the status before touching the body: a permanent 4xx must fail
	// fast even when the server stalls or truncates the body, and a read error
	// on a known 4xx must not be retried as a transport blip.
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= http.StatusInternalServerError {
		return nil, "", soMarkTransientError(fmt.Errorf("parser: SoMark poll: HTTP %d", resp.StatusCode))
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, "", soMarkTerminalError(fmt.Errorf("parser: SoMark poll: HTTP %d", resp.StatusCode))
	}
	raw, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		if ctx.Err() != nil {
			return nil, "", soMarkTerminalError(fmt.Errorf("parser: SoMark read poll: %w", readErr))
		}
		return nil, "", soMarkTransientError(fmt.Errorf("parser: SoMark read poll: %w", readErr))
	}
	var payload struct {
		Code    int            `json:"code"`
		Message string         `json:"message"`
		Data    map[string]any `json:"data"`
	}
	if err = json.Unmarshal(raw, &payload); err != nil {
		return nil, "", soMarkTransientError(fmt.Errorf("parser: SoMark decode poll: %w", err))
	}
	if payload.Code != 0 {
		return nil, "", soMarkTransientError(fmt.Errorf("parser: SoMark poll business error code=%d message=%s", payload.Code, payload.Message))
	}
	status, _ = payload.Data["status"].(string)
	switch classifySoMarkStatus(status) {
	case soMarkStatusSuccess:
		result, _ = payload.Data["result"].(map[string]any)
		if result == nil {
			// A SUCCESS without result is a service hiccup worth another poll.
			return nil, status, soMarkTransientError(fmt.Errorf("parser: SoMark SUCCESS without result"))
		}
		return result, status, nil
	case soMarkStatusInProgress:
		return nil, status, nil
	case soMarkStatusFailed:
		return nil, status, soMarkTerminalError(fmt.Errorf("parser: SoMark task %s failed status=%s", taskID, status))
	default:
		return nil, status, soMarkTerminalError(fmt.Errorf("parser: SoMark task %s returned unknown status %q", taskID, status))
	}
}

// soMarkItems converts the task result into parse items. SoMark reports boxes
// as [x0, top, x1, bottom] in the pixel space of its own page render
// (page["page_size"]), with the corners in no fixed order; pageSizes holds the
// source PDF's page sizes in points, indexed by zero-based page.
func soMarkItems(ctx context.Context, result map[string]any, keepHeaderFooter bool, pageSizes [][2]float64) ([]map[string]any, int, []string) {
	outputs, _ := result["outputs"].(map[string]any)
	jsonPayload, _ := outputs["json"].(map[string]any)
	pages, _ := jsonPayload["pages"].([]any)
	state := &htmlWalkState{budget: media.NewBudget()}
	var warnings []string

	// Positions reference real document pages: prefer the source PDF's page
	// count when it is known, falling back to the number of pages the
	// service returned.
	pageCount := len(pages)
	if len(pageSizes) > 0 {
		pageCount = len(pageSizes)
	}

	pointsByPage := make(map[int][2]float64, len(pageSizes))
	for index, size := range pageSizes {
		if size[0] > 0 && size[1] > 0 {
			pointsByPage[index+1] = size
		}
	}

	baseOpts := ExternalBlockOptions{
		Provider:          "SoMark",
		PageSizes:         pointsByPage,
		BleedTolerance:    0.1,
		ReorderEdges:      true,
		FigurePlaceholder: "[Image]",
	}

	var items []map[string]any
	seenWarnings := map[string]bool{}
	for pageIndex, pageRaw := range pages {
		page, ok := pageRaw.(map[string]any)
		if !ok {
			continue
		}
		pageNumber := soMarkPageNumber(page, pageIndex)
		var pageBlocks []ExternalBlock
		rawBlocks, _ := page["blocks"].([]any)
		for _, blockRaw := range rawBlocks {
			raw, ok := blockRaw.(map[string]any)
			if !ok {
				continue
			}
			block, ok, blockWarnings := soMarkBlock(ctx, raw, keepHeaderFooter, state)
			warnings = append(warnings, blockWarnings...)
			if !ok {
				continue
			}
			block.Page = pageNumber
			if box, ok := soMarkBox(raw); ok {
				block.Regions = []ExternalRegion{{Page: pageNumber, Box: box}}
			}
			pageBlocks = append(pageBlocks, block)
		}
		if len(pageBlocks) == 0 {
			continue
		}
		// Convert each page entry with its own pixel dimensions so two entries
		// resolving to the same PDF page cannot overwrite each other's scale.
		pageOpts := baseOpts
		pageOpts.SourceSizes = map[int][2]float64{}
		if w, h := soMarkPagePixelSize(page["page_size"]); w > 0 && h > 0 {
			pageOpts.SourceSizes[pageNumber] = [2]float64{w, h}
		}
		pageItems, placementWarnings := externalBlockMaps(pageBlocks, pageOpts)
		items = append(items, pageItems...)
		for _, w := range placementWarnings {
			if !seenWarnings[w] {
				seenWarnings[w] = true
				warnings = append(warnings, w)
			}
		}
	}
	return items, pageCount, append(state.warnings(), warnings...)
}

// soMarkPageNumber returns the 1-based number of a page entry. SoMark pages
// carry a 0-based page_num; the entry's position is the fallback.
func soMarkPageNumber(page map[string]any, pageIndex int) int {
	if n, ok := soMarkPageIndex(page["page_num"]); ok {
		return n + 1
	}
	return pageIndex + 1
}

// soMarkPageIndex validates a SoMark page index before use: JSON numbers
// arrive as float64, so reject fractional, negative, non-finite, and
// out-of-int-range values instead of truncating them into a wrong page.
func soMarkPageIndex(raw any) (int, bool) {
	f, ok := numericAny(raw)
	if !ok || math.IsNaN(f) || math.IsInf(f, 0) || f < 0 || f != math.Trunc(f) || f > float64(math.MaxInt32) {
		return 0, false
	}
	return int(f), true
}

// soMarkBox reads a block's bbox ([x0, top, x1, bottom]).
func soMarkBox(block map[string]any) ([4]float64, bool) {
	var box [4]float64
	bbox, _ := block["bbox"].([]any)
	if len(bbox) != 4 {
		return box, false
	}
	for i, raw := range bbox {
		n, ok := numericAny(raw)
		if !ok || math.IsNaN(n) || math.IsInf(n, 0) {
			return box, false
		}
		box[i] = n
	}
	return box, true
}

// soMarkPagePixelSize reads a page's SoMark pixel dimensions ({w, h}); a zero
// value means unknown.
func soMarkPagePixelSize(raw any) (float64, float64) {
	m, _ := raw.(map[string]any)
	w, _ := numericAny(m["w"])
	h, _ := numericAny(m["h"])
	return w, h
}

// soMarkPagePointSizes reads the visible [width, height] of each page in PDF points.
// A nil result or a zero entry means positions for that page are omitted.
func soMarkPagePointSizes(data []byte) [][2]float64 {
	r, err := pdfreader.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil
	}
	numPage := r.NumPage()
	// A corrupt or malicious catalog /Count can be negative (panics make) or
	// enormous (allocates/iterates without bound). Treat it as unreadable.
	if numPage < 0 || numPage > maxSoMarkPageCount {
		return nil
	}
	sizes := make([][2]float64, numPage)
	for p := 1; p <= numPage; p++ {
		page := r.Page(p)
		media, hasMedia := soMarkPageBox(page, "MediaBox")
		crop, hasCrop := soMarkPageBox(page, "CropBox")
		switch {
		case !hasCrop:
			crop = media
		case hasMedia:
			crop.Min.X = max(crop.Min.X, media.Min.X)
			crop.Min.Y = max(crop.Min.Y, media.Min.Y)
			crop.Max.X = min(crop.Max.X, media.Max.X)
			crop.Max.Y = min(crop.Max.Y, media.Max.Y)
		}
		w, h := crop.Max.X-crop.Min.X, crop.Max.Y-crop.Min.Y
		if w <= 0 || h <= 0 {
			continue
		}
		// Page boxes are in default user-space units (one point unless UserUnit is set).
		if unit := page.V.Key("UserUnit"); !unit.IsNull() {
			w, h = w*unit.Float64(), h*unit.Float64()
		}
		// A quarter-turn /Rotate (also inheritable) swaps the dimensions of
		// the rendered page SoMark sees; scaling and clamping must use the
		// rotated size.
		if rot := soMarkInheritedPageValue(page, "Rotate"); !rot.IsNull() {
			switch deg := ((int(rot.Float64()) % 360) + 360) % 360; deg {
			case 90, 270:
				w, h = h, w
			}
		}
		sizes[p-1] = [2]float64{w, h}
	}
	return sizes
}

// PDF rectangles may specify either pair of opposite corners in any order.
func soMarkPageBox(page pdfreader.Page, key string) (pdfreader.Rect, bool) {
	box := soMarkInheritedPageValue(page, key)
	if box.Len() != 4 {
		return pdfreader.Rect{}, false
	}
	var coords [4]float64
	for i := range coords {
		v := box.Index(i)
		if v.Kind() != pdfreader.Integer && v.Kind() != pdfreader.Real {
			return pdfreader.Rect{}, false
		}
		coords[i] = v.Float64()
	}
	x0, y0, x1, y1 := coords[0], coords[1], coords[2], coords[3]
	return pdfreader.Rect{
		Min: pdfreader.Point{X: min(x0, x1), Y: min(y0, y1)},
		Max: pdfreader.Point{X: max(x0, x1), Y: max(y0, y1)},
	}, true
}

// maxSoMarkPageCount mirrors deepdoc/pdf/plaintext_nocgo.go: one million
// pages is far beyond any real document, so larger claimed counts are
// malformed.
const maxSoMarkPageCount = 1_000_000

// maxSoMarkParentDepth bounds the /Parent walk: ledongthuc/pdf detects no
// reference cycles, so a self-referencing /Parent would loop forever.
const maxSoMarkParentDepth = 32

// soMarkInheritedPageValue resolves a page dictionary key through the Parent
// chain, the inheritance the PDF spec defines for page boxes.
func soMarkInheritedPageValue(page pdfreader.Page, key string) pdfreader.Value {
	depth := 0
	for v := page.V; !v.IsNull() && depth < maxSoMarkParentDepth; v = v.Key("Parent") {
		if r := v.Key(key); !r.IsNull() {
			return r
		}
		depth++
	}
	return pdfreader.Value{}
}

// soMarkBlock reads one block. Catalog and blank blocks are dropped, as are
// running headers and footers unless keepHeaderFooter is set. A table's
// content is an HTML or Markdown fragment, so the Markdown parser splits it
// into items here.
func soMarkBlock(ctx context.Context, raw map[string]any, keepHeaderFooter bool, state *htmlWalkState) (ExternalBlock, bool, []string) {
	blockType := strings.ToLower(strings.TrimSpace(stringValue(raw["type"])))
	switch blockType {
	case "cate", "cate_item", "blank":
		return ExternalBlock{}, false, nil
	case "header", "footer":
		if !keepHeaderFooter {
			return ExternalBlock{}, false, nil
		}
	}
	block := ExternalBlock{Type: ExternalBlockText, Text: stringValue(raw["content"])}
	switch blockType {
	case "figure", "cs", "qrcode", "stamp":
		block.Type = ExternalBlockFigure
	case "table":
		if strings.TrimSpace(block.Text) == "" {
			return ExternalBlock{}, false, nil
		}
		markdown := &MarkdownParser{libType: GoMarkdown}
		parsed := markdown.parseWithState(ctx, "", []byte(strings.TrimSpace(block.Text)), state)
		items := ItemsToMaps(parsed.JSON)
		for _, item := range items {
			if item["doc_type_kwd"] == "table" {
				item["layout"] = "table"
			}
		}
		if len(items) == 0 {
			return ExternalBlock{}, false, parsed.Warnings
		}
		block.Items = items
		return block, true, parsed.Warnings
	case "equation":
		block.Type = ExternalBlockEquation
	case "title":
		block.Type = ExternalBlockTitle
		block.HeadingLevel = int(numberValue(raw["title_level"]))
		if block.HeadingLevel < 1 || block.HeadingLevel > 6 {
			block.HeadingLevel = 1
		}
	}
	return block, true, nil
}

// envOrDefault prefers configured values, then the environment for named
// engines only, then the built-in fallback.
func (p *PDFParser) envOrDefault(envKey, configured, fallback string) string {
	if configured != "" {
		return configured
	}
	if !p.UseTenantModelConfig {
		if raw := strings.TrimSpace(common.GetEnv(envKey)); raw != "" {
			return raw
		}
	}
	return fallback
}

// envOrBool preserves tenant instance values, including explicit false, while
// named engines retain their environment override.
func (p *PDFParser) envOrBool(envKey string, configured bool) bool {
	if p.UseTenantModelConfig {
		return configured
	}
	if raw := strings.TrimSpace(common.GetEnv(envKey)); raw != "" {
		switch strings.ToLower(raw) {
		case "1", "true", "yes", "on":
			return true
		case "0", "false", "no", "off":
			return false
		}
	}
	return configured
}

func urlEncoded(values map[string]string) string {
	parts := make([]string, 0, len(values))
	for k, v := range values {
		if strings.TrimSpace(v) == "" {
			if k == "api_key" {
				continue
			}
		}
		parts = append(parts, fmt.Sprintf("%s=%s", k, v))
	}
	return strings.Join(parts, "&")
}
