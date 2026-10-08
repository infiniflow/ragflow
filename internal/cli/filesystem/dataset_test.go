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

package filesystem

import (
	stdcontext "context"
	"errors"
	"io"
	"net/url"
	"strings"
	"sync"
	"testing"
)

// recordingClient is a fake HTTPClientInterface that captures the path,
// headers, and body passed to each Request call. It also branches the
// canned response by path so getDataset (which lists /datasets) and the
// documents list (which hits /datasets/{id}/documents) can both return
// shapes the production code expects.
type recordingClient struct {
	mu    sync.Mutex
	calls []recordedCall
}

type recordedCall struct {
	path    string
	headers map[string]string
	body    map[string]interface{}
}

func (r *recordingClient) Request(_, path, _ string, headers map[string]string, body map[string]interface{}) (*HTTPResponse, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, recordedCall{path: path, headers: headers, body: body})

	bare := path
	if i := strings.IndexByte(path, '?'); i >= 0 {
		bare = path[:i]
	}

	switch {
	case bare == "/datasets":
		// getDataset: a list of datasets with one entry whose name matches.
		return &HTTPResponse{
			StatusCode: 200,
			Body:       []byte(`{"code":0,"data":[{"name":"kb","id":"ds-1"}],"message":"success"}`),
		}, nil
	case strings.HasPrefix(bare, "/datasets/") && strings.HasSuffix(bare, "/documents"):
		// listDocuments: an empty docs list with the right shape.
		return &HTTPResponse{
			StatusCode: 200,
			Body:       []byte(`{"code":0,"data":{"docs":[]},"message":"success"}`),
		}, nil
	default:
		return &HTTPResponse{StatusCode: 404, Body: []byte(`{"code":404,"message":"unknown"}`)}, nil
	}
}

func (r *recordingClient) UploadMultipart(_ string, _ string, _ io.Reader) error {
	return errors.New("upload not used in tests")
}

// TestListDocumentsForwardsPageAndPageSizeAsQueryParams drives the full
// listDocuments flow with a recording client and verifies that the path
// passed to Request carries page and page_size on the URL (not in the
// headers argument) and that the page math reflects the offset. Also
// pins that the page math does not panic when Limit is zero with a
// positive Offset.
func TestListDocumentsForwardsPageAndPageSizeAsQueryParams(t *testing.T) {
	cases := []struct {
		name     string
		opts     *ListOptions
		wantPage string
		wantSize string
	}{
		{"limit+offset", &ListOptions{Limit: 10, Offset: 20}, "3", "10"},
		{"limit only", &ListOptions{Limit: 25, Offset: 0}, "1", "25"},
		{"offset with zero limit does not panic", &ListOptions{Limit: 0, Offset: 30}, "1", "10"},
		{"zero opts defaults page_size=10 page=1", &ListOptions{}, "1", "10"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := &recordingClient{}
			p := &DatasetProvider{httpClient: rec}

			if _, err := p.listDocuments(stdcontext.Background(), "kb", tc.opts); err != nil {
				t.Fatalf("listDocuments: %v", err)
			}

			// The listDocuments call is the last recorded call. The first
			// call is the getDataset lookup against /datasets. Strip the
			// query string before matching the suffix, because the fix
			// appends ?page=...&page_size=... to the path.
			var listCall recordedCall
			for _, c := range rec.calls {
				bare := c.path
				if i := strings.IndexByte(bare, '?'); i >= 0 {
					bare = bare[:i]
				}
				if strings.HasPrefix(bare, "/datasets/") && strings.HasSuffix(bare, "/documents") {
					listCall = c
					break
				}
			}
			if listCall.path == "" {
				t.Fatalf("no documents list call recorded; calls = %+v", rec.calls)
			}
			if listCall.headers != nil {
				t.Errorf("headers = %v, want nil (page params must not go in headers)", listCall.headers)
			}
			values, err := url.ParseQuery(strings.TrimPrefix(listCall.path, "/datasets/ds-1/documents?"))
			if err != nil {
				t.Fatalf("ParseQuery(%q): %v", listCall.path, err)
			}
			if page := values.Get("page"); page != tc.wantPage {
				t.Errorf("page = %q, want %q", page, tc.wantPage)
			}
			if size := values.Get("page_size"); size != tc.wantSize {
				t.Errorf("page_size = %q, want %q", size, tc.wantSize)
			}
		})
	}
}

// TestBuildDocumentsListPath pins the URL→page/page_size mapping. The
// server reads page and page_size from c.Query, so they must go on the
// URL, not in the headers argument of HTTPClientInterface.Request. The
// pre-fix code passed them as headers (silent no-op on the server side)
// and divided Offset by opts.Limit without checking that Limit was
// non-zero (panic for Offset>0, Limit=0).
func TestBuildDocumentsListPath(t *testing.T) {
	cases := []struct {
		name     string
		opts     *ListOptions
		wantPage string
		wantSize string
	}{
		{
			name:     "nil opts leaves the path unchanged",
			opts:     nil,
			wantPage: "",
			wantSize: "",
		},
		{
			name:     "limit and offset both positive: page = offset/limit + 1",
			opts:     &ListOptions{Limit: 10, Offset: 20},
			wantPage: "3",
			wantSize: "10",
		},
		{
			name:     "offset only is ignored when limit is zero (no div-by-zero panic)",
			opts:     &ListOptions{Limit: 0, Offset: 20},
			wantPage: "1",
			wantSize: "10",
		},
		{
			name:     "limit only, no offset: page 1",
			opts:     &ListOptions{Limit: 25, Offset: 0},
			wantPage: "1",
			wantSize: "25",
		},
		{
			name:     "no limit and no offset: default page_size=10, page=1",
			opts:     &ListOptions{Limit: 0, Offset: 0},
			wantPage: "1",
			wantSize: "10",
		},
		{
			name:     "negative limit treated as missing",
			opts:     &ListOptions{Limit: -5, Offset: 30},
			wantPage: "1",
			wantSize: "10",
		},
		{
			name:     "offset at the boundary of one full page",
			opts:     &ListOptions{Limit: 10, Offset: 10},
			wantPage: "2",
			wantSize: "10",
		},
	}

	base := "/datasets/00000000-0000-0000-0000-000000000001/documents"
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildDocumentsListPath(base, tc.opts)
			if tc.wantPage == "" {
				if got != base {
					t.Fatalf("got %q; want unchanged %q", got, base)
				}
				return
			}
			if !strings.HasPrefix(got, base+"?") {
				t.Fatalf("got %q; want prefix %q?", got, base)
			}
			values, err := url.ParseQuery(strings.TrimPrefix(got, base+"?"))
			if err != nil {
				t.Fatalf("ParseQuery: %v", err)
			}
			if page := values.Get("page"); page != tc.wantPage {
				t.Errorf("page = %q, want %q", page, tc.wantPage)
			}
			if size := values.Get("page_size"); size != tc.wantSize {
				t.Errorf("page_size = %q, want %q", size, tc.wantSize)
			}
			// The page param must come through URL-decoded as a positive
			// integer; the values map decodes automatically.
			if tc.wantPage != "" {
				if _, err := url.QueryUnescape(tc.wantPage); err != nil {
					t.Errorf("page %q does not URL-decode cleanly: %v", tc.wantPage, err)
				}
			}
		})
	}
}
