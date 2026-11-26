package cloud

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
		name      string
		size      int
		fail      bool
		resumable bool
	}{
		{name: "success", size: 1},
		// Exceed the chunk size so the request fails before Write can finish.
		{name: "write failure", size: googleapi.DefaultUploadChunkSize + 1, fail: true},
		{name: "close failure", size: 1, fail: true},
		{name: "resumable upload failure", size: googleapi.DefaultUploadChunkSize + 1, fail: true, resumable: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			wantErr := errors.New("upload failed")
			requests := make(chan context.Context, 2)
			client, err := gcs.NewClient(t.Context(), option.WithoutAuthentication(), option.WithHTTPClient(&http.Client{
				Transport: gcsRoundTripper(func(req *http.Request) (*http.Response, error) {
					defer req.Body.Close()
					requests <- req.Context()
					if tt.resumable && req.URL.Query().Get("uploadType") == "resumable" {
						return &http.Response{
							StatusCode: http.StatusOK,
							Header:     http.Header{"Location": {"https://storage.googleapis.com/upload/session?upload_id=private-upload-session"}},
							Body:       io.NopCloser(strings.NewReader(`{}`)),
						}, nil
					}
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
			if tt.fail && strings.Contains(err.Error(), "https://") {
				t.Fatalf("Put error exposes an upload URL: %v", err)
			}
			if tt.resumable && len(requests) != 2 {
				t.Fatal("upload did not reach the resumable session")
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

// TestGCSStorageRemoveBucketMissingIsNoop pins that removing an already
// absent bucket succeeds, matching every other storage backend. Callers drop
// their pre-delete BucketExists probe and rely on this idempotency.
func TestGCSStorageRemoveBucketMissingIsNoop(t *testing.T) {
	client, err := gcs.NewClient(t.Context(), option.WithoutAuthentication(), option.WithHTTPClient(&http.Client{
		Transport: gcsRoundTripper(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusNotFound,
				Header:     http.Header{"Content-Type": {"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"error":{"code":404,"message":"Not Found","status":"NOT_FOUND"}}`)),
			}, nil
		}),
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	if err := (&GCSStorage{client: client}).RemoveBucket(t.Context(), "missing-bucket"); err != nil {
		t.Fatalf("RemoveBucket on missing bucket = %v, want nil", err)
	}
}

// TestGCSStorageRemoveBucketPropagatesBackendError guards that the missing-
// bucket tolerance does not swallow an unrelated backend failure.
func TestGCSStorageRemoveBucketPropagatesBackendError(t *testing.T) {
	client, err := gcs.NewClient(t.Context(), option.WithoutAuthentication(), option.WithHTTPClient(&http.Client{
		Transport: gcsRoundTripper(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusForbidden,
				Header:     http.Header{"Content-Type": {"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"error":{"code":403,"message":"permission denied","status":"PERMISSION_DENIED"}}`)),
			}, nil
		}),
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	if err := (&GCSStorage{client: client}).RemoveBucket(t.Context(), "broken-bucket"); err == nil {
		t.Fatal("RemoveBucket swallowed a 403 response")
	}
}
