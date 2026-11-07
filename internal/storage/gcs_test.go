package storage

import (
	"context"
	"errors"
	"fmt"
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

// fakeGCS records each request and answers listings with the given object names.
func fakeGCS(t *testing.T, listed []string) (*gcs.Client, *[]string) {
	t.Helper()
	var seen []string
	client, err := gcs.NewClient(t.Context(), option.WithoutAuthentication(), option.WithHTTPClient(&http.Client{
		Transport: gcsRoundTripper(func(req *http.Request) (*http.Response, error) {
			if req.Body != nil {
				_, _ = io.Copy(io.Discard, req.Body)
				_ = req.Body.Close()
			}
			line := req.Method + " " + req.URL.Path
			if name := req.URL.Query().Get("name"); name != "" {
				line += " name=" + name
			}
			if prefix := req.URL.Query().Get("prefix"); prefix != "" {
				line += " prefix=" + prefix
			}
			if req.URL.Query().Get("versions") == "true" {
				line += " versions"
			}
			if generation := req.URL.Query().Get("generation"); generation != "" {
				line += " generation=" + generation
			}
			seen = append(seen, line)
			body := `{}`
			if req.Method == http.MethodGet && strings.HasSuffix(req.URL.Path, "/o") {
				items := make([]string, 0, len(listed))
				for i, name := range listed {
					items = append(items, fmt.Sprintf(`{"name":%q,"bucket":"shared","generation":"%d"}`, name, i+1))
				}
				body = `{"items":[` + strings.Join(items, ",") + `]}`
			}
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
		}),
	}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client, &seen
}

func TestGCSStorageResolve(t *testing.T) {
	for _, tt := range []struct {
		name               string
		bucket, prefix     string
		wantBucket, wantFn string
	}{
		{name: "bucket per dataset", wantBucket: "kb1", wantFn: "a.txt"},
		{name: "single bucket", bucket: "shared", wantBucket: "shared", wantFn: "kb1/a.txt"},
		{name: "single bucket with prefix", bucket: "shared", prefix: "rf", wantBucket: "shared", wantFn: "rf/kb1/a.txt"},
		{name: "single bucket with slashes around the prefix", bucket: "shared", prefix: "/rf/", wantBucket: "shared", wantFn: "rf/kb1/a.txt"},
		{name: "prefix without a bucket is not used", prefix: "rf", wantBucket: "kb1", wantFn: "a.txt"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := &GCSStorage{}
			s.config.Bucket, s.config.PrefixPath = tt.bucket, tt.prefix
			if b, fn := s.resolve("kb1", "a.txt"); b != tt.wantBucket || fn != tt.wantFn {
				t.Fatalf("resolve = %s, %s; want %s, %s", b, fn, tt.wantBucket, tt.wantFn)
			}
		})
	}
}

func TestGCSStorageSingleBucketPutWritesUnderDatasetPrefix(t *testing.T) {
	client, seen := fakeGCS(t, nil)
	s := &GCSStorage{client: client}
	s.config.Bucket = "shared"
	if err := s.Put(t.Context(), "kb1", "a.txt", []byte("x")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if len(*seen) != 1 || !strings.Contains((*seen)[0], "/b/shared/o") || !strings.HasSuffix((*seen)[0], "name=kb1/a.txt") {
		t.Fatalf("requests = %q, want one upload of kb1/a.txt into bucket shared", *seen)
	}
}

func TestGCSStorageSingleBucketRemoveBucketDeletesOnlyThePrefix(t *testing.T) {
	// kb1/a.txt is listed twice: a live and a noncurrent generation.
	client, seen := fakeGCS(t, []string{"kb1/a.txt", "kb1/a.txt", "kb1/b.txt"})
	s := &GCSStorage{client: client}
	s.config.Bucket = "shared"
	if err := s.RemoveBucket(t.Context(), "kb1"); err != nil {
		t.Fatalf("RemoveBucket: %v", err)
	}
	want := []string{
		"GET /storage/v1/b/shared/o prefix=kb1/ versions",
		"DELETE /storage/v1/b/shared/o/kb1/a.txt generation=1",
		"DELETE /storage/v1/b/shared/o/kb1/a.txt generation=2",
		"DELETE /storage/v1/b/shared/o/kb1/b.txt generation=3",
	}
	if strings.Join(*seen, "\n") != strings.Join(want, "\n") {
		t.Fatalf("requests:\n%s\nwant:\n%s", strings.Join(*seen, "\n"), strings.Join(want, "\n"))
	}
}

func TestGCSStorageSingleBucketRemoveEmptyBucket(t *testing.T) {
	client, seen := fakeGCS(t, nil)
	s := &GCSStorage{client: client}
	s.config.Bucket = "shared"
	if err := s.RemoveEmptyBucket(t.Context(), "folder1"); err != nil {
		t.Fatalf("RemoveEmptyBucket of an empty prefix: %v", err)
	}
	for _, line := range *seen {
		if strings.HasPrefix(line, "DELETE") {
			t.Fatalf("deleted %q; the shared bucket must stay", line)
		}
	}

	// The listing is version-aware, so a prefix holding only noncurrent
	// generations counts as non-empty.
	client, seen = fakeGCS(t, []string{"folder1/x"})
	s.client = client
	if err := s.RemoveEmptyBucket(t.Context(), "folder1"); err == nil {
		t.Fatal("RemoveEmptyBucket of a non-empty prefix succeeded")
	}
	if len(*seen) != 1 || (*seen)[0] != "GET /storage/v1/b/shared/o prefix=folder1/ versions" {
		t.Fatalf("requests = %q, want one version-aware listing of prefix folder1/", *seen)
	}
}

func TestGCSStorageSingleBucketListObjectsIsRelative(t *testing.T) {
	client, seen := fakeGCS(t, []string{"kb1/a.txt", "kb1/sub/b.txt"})
	s := &GCSStorage{client: client}
	s.config.Bucket = "shared"
	got, err := s.ListObjects(t.Context(), "kb1")
	if err != nil {
		t.Fatalf("ListObjects: %v", err)
	}
	if len(*seen) != 1 || (*seen)[0] != "GET /storage/v1/b/shared/o prefix=kb1/" {
		t.Fatalf("requests = %q, want one listing of prefix kb1/ in bucket shared", *seen)
	}
	if strings.Join(got, ",") != "a.txt,sub/b.txt" {
		t.Fatalf("ListObjects = %q", got)
	}
}

func TestGCSStorageRemoveBucketDeletesEveryGeneration(t *testing.T) {
	client, seen := fakeGCS(t, []string{"a.txt", "a.txt"})
	s := &GCSStorage{client: client}
	if err := s.RemoveBucket(t.Context(), "kb1"); err != nil {
		t.Fatalf("RemoveBucket: %v", err)
	}
	want := []string{
		"GET /storage/v1/b/kb1/o versions",
		"DELETE /storage/v1/b/kb1/o/a.txt generation=1",
		"DELETE /storage/v1/b/kb1/o/a.txt generation=2",
		"DELETE /storage/v1/b/kb1",
	}
	if strings.Join(*seen, "\n") != strings.Join(want, "\n") {
		t.Fatalf("requests:\n%s\nwant:\n%s", strings.Join(*seen, "\n"), strings.Join(want, "\n"))
	}
}
