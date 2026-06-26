//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package deepdoc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/url"
	"strings"
)

// BBox is a 4-tuple [left, top, right, bottom] in the image's
// native pixel coordinates. Float to preserve sub-pixel accuracy
// from the upstream server (the Python client lowercases+indexes
// without rounding).
type BBox [4]float64

// DLAResult is one detected layout region. Type is the normalized
// class name (lowercased, per Python `dla_cli.py:43`); TypeIdx is
// the raw class index into DLAClasses (preserved so callers can
// disambiguate the documented duplicate class slots).
type DLAResult struct {
	Type    string  `json:"type"`
	Score   float64 `json:"score"`
	BBox    BBox    `json:"bbox"`
	TypeIdx int     `json:"type_idx"`
}

// DLAClasses is the 10-entry class taxonomy from
// deepdoc/vision/dla_cli.py:10-21. Order is significant — TypeIdx
// in the wire payload is an index into this slice. The duplicates
// at indices 4/6/7/9 are kept verbatim for backward compatibility
// with existing inference servers.
var DLAClasses = []string{
	"title",          // 0
	"text",           // 1
	"reference",      // 2
	"figure",         // 3
	"figure caption", // 4
	"table",          // 5
	"table caption",  // 6
	"table caption",  // 7  duplicate
	"equation",       // 8
	"figure caption", // 9  duplicate
}

// rawDLA is the wire format the DLA server returns
// (docs/agent-port/deepdoc-endpoints.md §2.3).
type rawDLA struct {
	BBoxes [][]float64 `json:"bboxes"`
}

// DLA calls the remote DLA service for layout analysis of one or
// more JPEG-encoded images. The Python contract
// (dla_cli.py:25-50) is replicated:
//
//   - one HTTP POST per image
//   - 3 attempts per image, 18s per attempt, 200ms initial backoff
//   - failed images return an empty DLAResult (caller does not
//     have to handle per-image errors — the Python
//     `layout_recognizer.py:74-76` is happy with empty results)
//
// When no DEEPDOC_URL is set, returns ErrNoURL without any network
// call. When the base URL is set but the service is unreachable
// after 3 attempts, the failed image's slot is an empty DLAResult
// and the rest still process (matches Python's "len(res) == i"
// append-empty pattern).
func (c *Client) DLA(ctx context.Context, images [][]byte) ([]DLAResult, error) {
	if !c.Enabled() {
		return nil, ErrNoURL
	}
	if len(images) == 0 {
		return []DLAResult{}, nil
	}
	predictURL, err := c.predictURL()
	if err != nil {
		return nil, err
	}
	out := make([]DLAResult, 0, len(images))
	for _, img := range images {
		res := c.predictOne(ctx, predictURL, img)
		// Per Python: a failed image yields an empty slot rather
		// than aborting the whole batch. Surface the first hard
		// error at the end if the user wants it.
		if len(res) == 0 {
			out = append(out, DLAResult{})
		} else {
			out = append(out, res...)
		}
	}
	return out, nil
}

// predictURL resolves the DLA endpoint URL from the configured base.
// Trims trailing slash to avoid `//predict` on `http://host/`.
func (c *Client) predictURL() (string, error) {
	base := strings.TrimRight(c.baseURL, "/")
	u, err := url.Parse(base + predictPath)
	if err != nil {
		return "", fmt.Errorf("deepdoc: parse predict url: %w", err)
	}
	return u.String(), nil
}

// predictOne runs the retry loop for a single image. Returns the
// list of bboxes the server returned, or an empty slice if all
// attempts failed. Errors are NOT returned for retry exhaustion —
// the caller maps "empty slice" to "no detections" per the Python
// contract; a hard error (4xx, bad URL) is returned immediately.
func (c *Client) predictOne(ctx context.Context, predictURL string, image []byte) []DLAResult {
	buildBody := func() (io.Reader, string) {
		// Each retry needs a fresh multipart body — multipart.Writer
		// consumes its underlying buffer on Close. CreatePart lets
		// us set both a filename (so Go's net/http server-side
		// parser routes the part to MultipartForm.File) and the
		// image/jpeg Content-Type the DLA server expects (matches
		// the Python `files={'request': ('image.jpg', ...)}`
		// contract from dla_cli.py:35).
		buf := &bytes.Buffer{}
		w := multipart.NewWriter(buf)
		fw, _ := w.CreatePart(map[string][]string{
			"Content-Disposition": {`form-data; name="request"; filename="image.jpg"`},
			"Content-Type":        {"image/jpeg"},
		})
		_, _ = fw.Write(image)
		_ = w.Close()
		return buf, w.FormDataContentType()
	}
	validate := func(data []byte) error {
		var r rawDLA
		if err := json.Unmarshal(data, &r); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidResponse, err)
		}
		if r.BBoxes == nil {
			return fmt.Errorf("%w: missing bboxes key", ErrInvalidResponse)
		}
		return nil
	}
	data, err := c.doPost(ctx, predictURL, buildBody, validate)
	if err != nil {
		return nil
	}
	var r rawDLA
	_ = json.Unmarshal(data, &r) // already validated above
	results := make([]DLAResult, 0, len(r.BBoxes))
	for _, b := range r.BBoxes {
		if len(b) < 6 {
			continue
		}
		// [l, t, r, b, score, type_idx] per docs/agent-port/deepdoc-endpoints.md §2.3.
		bbox := BBox{b[0], b[1], b[2], b[3]}
		idx := int(b[5])
		cls := ""
		if idx >= 0 && idx < len(DLAClasses) {
			cls = DLAClasses[idx]
		}
		results = append(results, DLAResult{
			Type:    cls,
			Score:   b[4],
			BBox:    bbox,
			TypeIdx: idx,
		})
	}
	return results
}
