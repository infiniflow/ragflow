package storage

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type copySourceStorage interface {
	Copy(context.Context, string, string, string, string) bool
	Move(context.Context, string, string, string, string) bool
}

type copySourceRequest struct {
	source string
	target string
}

func newCopySourceTestStorage(backend, endpoint, bucket, prefix string) copySourceStorage {
	client := s3.NewFromConfig(aws.Config{
		Region:      "us-east-1",
		Credentials: aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider("access", "secret", "")),
	}, func(options *s3.Options) {
		options.BaseEndpoint = aws.String(endpoint)
		options.UsePathStyle = true
		options.RetryMaxAttempts = 1
	})
	if backend == "oss" {
		return &OSSStorage{client: client, bucket: bucket, prefixPath: prefix}
	}
	return &S3Storage{client: client, bucket: bucket, prefixPath: prefix}
}

func TestStorageCopyEncodesSource(t *testing.T) {
	for _, backend := range []string{"s3", "oss"} {
		for _, test := range []struct {
			name       string
			key        string
			wantSource string
		}{
			{name: "ordinary", key: "report.txt", wantSource: "source/report.txt"},
			{name: "nested", key: "folder/report.txt", wantSource: "source/folder/report.txt"},
			{name: "escaped slash", key: "report%2Ffinal.txt", wantSource: "source/report%252Ffinal.txt"},
			{name: "escaped percent", key: "report%25final.txt", wantSource: "source/report%2525final.txt"},
			{name: "literal percent", key: "50%.txt", wantSource: "source/50%25.txt"},
			{name: "unicode and spaces", key: "报告  2026.txt", wantSource: "source/%E6%8A%A5%E5%91%8A%20%202026.txt"},
			{name: "reserved characters", key: "a+b?#.txt", wantSource: "source/a%2Bb%3F%23.txt"},
		} {
			t.Run(backend+"/"+test.name, func(t *testing.T) {
				requests := make(chan copySourceRequest, 1)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Method != http.MethodPut || r.URL.Path != "/target/result.txt" {
						t.Errorf("unexpected request: %s %s", r.Method, r.URL)
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					requests <- copySourceRequest{source: r.Header.Get("X-Amz-Copy-Source")}
					w.Header().Set("Content-Type", "application/xml")
					fmt.Fprint(w, `<CopyObjectResult><ETag>"copied"</ETag></CopyObjectResult>`)
				}))
				defer server.Close()
				store := newCopySourceTestStorage(backend, server.URL, "", "")
				if !store.Copy(t.Context(), "source", test.key, "target", "result.txt") {
					t.Fatal("Copy failed")
				}
				source := (<-requests).source
				if source != test.wantSource {
					t.Fatalf("CopySource = %q, want %q", source, test.wantSource)
				}
			})
		}
	}
}

func TestStorageCopyEncodesResolvedSource(t *testing.T) {
	for _, test := range []struct {
		backend    string
		bucket     string
		prefix     string
		wantSource string
		wantTarget string
	}{
		{backend: "s3", prefix: "data+%2F", wantSource: "source/data%2B%252F/source/report.txt", wantTarget: "/target/data+%2F/target/result.txt"},
		{backend: "oss", prefix: "data+%2F", wantSource: "source/data%2B%252F/report.txt", wantTarget: "/target/data+%2F/result.txt"},
		{backend: "s3", bucket: "physical", prefix: "data+%2F", wantSource: "physical/data%2B%252F/source/report.txt", wantTarget: "/physical/data+%2F/target/result.txt"},
		{backend: "oss", bucket: "physical", prefix: "data+%2F", wantSource: "physical/data%2B%252F/report.txt", wantTarget: "/physical/data+%2F/result.txt"},
	} {
		t.Run(test.backend+"/"+test.bucket, func(t *testing.T) {
			requests := make(chan copySourceRequest, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests <- copySourceRequest{source: r.Header.Get("X-Amz-Copy-Source"), target: r.URL.Path}
				w.Header().Set("Content-Type", "application/xml")
				fmt.Fprint(w, `<CopyObjectResult><ETag>"copied"</ETag></CopyObjectResult>`)
			}))
			defer server.Close()
			store := newCopySourceTestStorage(test.backend, server.URL, test.bucket, test.prefix)
			if !store.Copy(t.Context(), "source", "report.txt", "target", "result.txt") {
				t.Fatal("Copy failed")
			}
			request := <-requests
			if request.source != test.wantSource || request.target != test.wantTarget {
				t.Fatalf("source = %q, target = %q; want source = %q, target = %q", request.source, request.target, test.wantSource, test.wantTarget)
			}
		})
	}
}

func TestStorageMovePreservesLiteralPercentKey(t *testing.T) {
	for _, backend := range []string{"s3", "oss"} {
		t.Run(backend, func(t *testing.T) {
			var mu sync.Mutex
			objects := map[string]string{
				"source/report%2Ffinal.txt": "original bytes",
				"source/report/final.txt":   "different object",
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				key := strings.TrimPrefix(r.URL.Path, "/")
				switch r.Method {
				case http.MethodPut:
					// CopySource is URL-encoded, whereas URL.Path is already decoded.
					source, err := url.PathUnescape(r.Header.Get("X-Amz-Copy-Source"))
					if err != nil {
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					data, exists := objects[source]
					if !exists {
						w.WriteHeader(http.StatusNotFound)
						fmt.Fprint(w, `<Error><Code>NoSuchKey</Code></Error>`)
						return
					}
					objects[key] = data
					w.Header().Set("Content-Type", "application/xml")
					fmt.Fprint(w, `<CopyObjectResult><ETag>"copied"</ETag></CopyObjectResult>`)
				case http.MethodDelete:
					delete(objects, key)
					w.WriteHeader(http.StatusNoContent)
				default:
					w.WriteHeader(http.StatusBadRequest)
				}
			}))
			defer server.Close()
			store := newCopySourceTestStorage(backend, server.URL, "", "")
			if !store.Move(t.Context(), "source", "report%2Ffinal.txt", "target", "result.txt") {
				t.Fatal("Move failed")
			}
			mu.Lock()
			defer mu.Unlock()
			if got := objects["target/result.txt"]; got != "original bytes" {
				t.Fatalf("destination = %q, want original bytes", got)
			}
			if _, exists := objects["source/report%2Ffinal.txt"]; exists {
				t.Fatal("Move did not remove the original source")
			}
			if got := objects["source/report/final.txt"]; got != "different object" {
				t.Fatalf("unrelated source was changed: %q", got)
			}
		})
	}
}

func TestStorageMoveKeepsSourceAfterCopyFailure(t *testing.T) {
	for _, backend := range []string{"s3", "oss"} {
		t.Run(backend, func(t *testing.T) {
			var copies, deletes atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodDelete {
					deletes.Add(1)
				}
				if r.Method == http.MethodPut {
					copies.Add(1)
				}
				w.WriteHeader(http.StatusForbidden)
				fmt.Fprint(w, `<Error><Code>AccessDenied</Code></Error>`)
			}))
			defer server.Close()
			store := newCopySourceTestStorage(backend, server.URL, "", "")
			if store.Move(t.Context(), "source", "report%2Ffinal.txt", "target", "result.txt") {
				t.Fatal("Move succeeded after Copy failed")
			}
			if copies.Load() != 1 || deletes.Load() != 0 {
				t.Fatalf("Move issued %d copies and %d deletes after Copy failed", copies.Load(), deletes.Load())
			}
		})
	}
}
