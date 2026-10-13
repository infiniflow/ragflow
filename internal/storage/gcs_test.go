package storage

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	gcs "cloud.google.com/go/storage"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

type gcsRoundTripper func(*http.Request) (*http.Response, error)

func (f gcsRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestGCSStoragePutCancelsUpload(t *testing.T) {
	for _, tt := range []struct {
		name string
		size int
		fail bool
	}{
		{name: "success", size: 1},
		// Exceed the chunk size so the request fails before Write can finish.
		{name: "write failure", size: googleapi.DefaultUploadChunkSize + 1, fail: true},
		{name: "close failure", size: 1, fail: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			wantErr := errors.New("upload failed")
			requests := make(chan context.Context, 1)
			client, err := gcs.NewClient(t.Context(), option.WithoutAuthentication(), option.WithHTTPClient(&http.Client{
				Transport: gcsRoundTripper(func(req *http.Request) (*http.Response, error) {
					defer req.Body.Close()
					requests <- req.Context()
					if tt.fail {
						return nil, wantErr
					}
					if _, err := io.Copy(io.Discard, req.Body); err != nil {
						return nil, err
					}
					return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{}`))}, nil
				}),
			}))
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			ctx := t.Context()
			err = (&GCSStorage{client: client}).Put(ctx, "bucket", "object", make([]byte, tt.size))
			if tt.fail && !errors.Is(err, wantErr) || !tt.fail && err != nil {
				t.Fatalf("Put error = %v, want upload failure: %t", err, tt.fail)
			}
			select {
			case uploadCtx := <-requests:
				if !errors.Is(uploadCtx.Err(), context.Canceled) {
					t.Errorf("upload context error = %v, want context.Canceled", uploadCtx.Err())
				}
			default:
				t.Fatal("upload did not reach the transport")
			}
			if err := ctx.Err(); err != nil {
				t.Errorf("caller context was canceled: %v", err)
			}
		})
	}
}
