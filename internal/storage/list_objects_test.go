package storage

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"ragflow/internal/server/config"
)

func newListObjectsTestStorage(t *testing.T, backend, endpoint, bucket, prefix string) Storage {
	t.Helper()
	if backend != "minio" {
		return newCopySourceTestStorage(backend, endpoint, bucket, prefix).(Storage)
	}
	store, err := NewMinioStorage(config.MinioConfig{
		Host: strings.TrimPrefix(endpoint, "http://"), Region: "us-east-1",
		User: "access", Password: "secret", Bucket: bucket, PrefixPath: prefix,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

// Exercise the real SDKs against an S3 HTTP boundary. A returned key must
// address the same object through Put, Get and Remove without exposing the
// storage prefix or accidentally selecting a neighboring logical bucket.
func TestListObjectsUsesLogicalBucketPaths(t *testing.T) {
	for _, backend := range []string{"s3", "minio", "oss"} {
		for _, tc := range []struct {
			name, bucket, prefix, logical, physical, keyPrefix string
		}{
			{name: "ordinary", logical: "dataset", physical: "dataset"},
			{name: "prefix", prefix: "root", logical: "dataset", physical: "dataset", keyPrefix: "root/"},
			{name: "shared", bucket: "shared", logical: "dataset", physical: "shared", keyPrefix: "dataset/"},
			{name: "shared prefix", bucket: "shared", prefix: "root", logical: "dataset", physical: "shared", keyPrefix: "root/dataset/"},
			{name: "logical equals physical", bucket: "shared", prefix: "root", logical: "shared", physical: "shared", keyPrefix: "root/shared/"},
			{name: "literal trailing slash", bucket: "shared", prefix: "root/", logical: "dataset", physical: "shared", keyPrefix: "root//dataset/"},
		} {
			if backend == "oss" && tc.bucket != "" {
				continue
			}
			if backend == "s3" && tc.bucket == "" && tc.prefix != "" {
				tc.keyPrefix = "root/dataset/"
			}
			t.Run(backend+"/"+tc.name, func(t *testing.T) {
				key := "folder/报告 & 50%.txt"
				var listCalls, putCalls, getCalls, deleteCalls atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if strings.TrimSuffix(r.URL.Path, "/") == "/"+tc.physical && r.Method == http.MethodHead {
						return
					}
					if r.URL.Query().Get("list-type") == "2" {
						listCalls.Add(1)
						if strings.TrimSuffix(r.URL.Path, "/") != "/"+tc.physical || r.URL.Query().Get("prefix") != tc.keyPrefix {
							t.Errorf("list request = %s, want bucket %q prefix %q", r.URL, tc.physical, tc.keyPrefix)
						}
						w.Header().Set("Content-Type", "application/xml")
						// XML entities, not URL escapes: both SDKs must return the literal key.
						fmt.Fprintf(w, `<ListBucketResult><Name>%s</Name><IsTruncated>false</IsTruncated><Contents><Key>%sfolder/报告 &amp; 50%%.txt</Key><Size>4</Size><ETag>"etag"</ETag></Contents></ListBucketResult>`, tc.physical, tc.keyPrefix)
						return
					}
					if r.URL.Path != "/"+tc.physical+"/"+tc.keyPrefix+key {
						t.Errorf("object request = %s %s, want key %q", r.Method, r.URL, tc.keyPrefix+key)
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					switch r.Method {
					case http.MethodPut:
						putCalls.Add(1)
						_, _ = io.Copy(io.Discard, r.Body)
						w.Header().Set("ETag", `"etag"`)
					case http.MethodGet:
						getCalls.Add(1)
						w.Header().Set("Last-Modified", "Wed, 18 Nov 2026 00:00:00 GMT")
						w.Header().Set("Content-Length", "4")
						fmt.Fprint(w, "data")
					case http.MethodDelete:
						deleteCalls.Add(1)
						w.WriteHeader(http.StatusNoContent)
					default:
						t.Errorf("unexpected method %s", r.Method)
						w.WriteHeader(http.StatusBadRequest)
					}
				}))
				defer server.Close()
				store := newListObjectsTestStorage(t, backend, server.URL, tc.bucket, tc.prefix)
				if err := store.Put(t.Context(), tc.logical, key, []byte("data")); err != nil {
					t.Fatal(err)
				}
				keys, err := store.ListObjects(t.Context(), tc.logical)
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(keys, []string{key}) {
					t.Fatalf("ListObjects = %q, want relative key %q", keys, key)
				}
				got, err := store.Get(t.Context(), tc.logical, keys[0])
				if err != nil || string(got) != "data" {
					t.Fatalf("Get listed key = %q, %v", got, err)
				}
				if err := store.Remove(t.Context(), tc.logical, keys[0]); err != nil {
					t.Fatal(err)
				}
				if listCalls.Load() != 1 || putCalls.Load() != 1 || getCalls.Load() != 1 || deleteCalls.Load() != 1 {
					t.Fatal("expected one request for each operation")
				}
			})
		}
	}
}

func TestListObjectsOSSRejectsSharedBucket(t *testing.T) {
	for _, prefix := range []string{"", "root"} {
		t.Run(prefix, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); fmt.Fprint(w, `<ListBucketResult/>`) }))
			defer server.Close()
			store := newListObjectsTestStorage(t, "oss", server.URL, "shared", prefix)
			keys, err := store.ListObjects(context.Background(), "dataset")
			if err == nil || !strings.Contains(err.Error(), "shared bucket mode does not isolate objects by logical bucket") || keys != nil {
				t.Fatalf("ListObjects = %q, %v; want shared-mode refusal", keys, err)
			}
			if calls.Load() != 0 {
				t.Fatal("shared OSS listing must not send a request")
			}
		})
	}
}

func TestListObjectsEmptyAndFailedResponses(t *testing.T) {
	for _, backend := range []string{"s3", "minio", "oss"} {
		for _, fail := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/error=%v", backend, fail), func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/xml")
					if fail {
						w.WriteHeader(http.StatusForbidden)
						fmt.Fprint(w, `<Error><Code>AccessDenied</Code><Message>denied</Message></Error>`)
						return
					}
					fmt.Fprint(w, `<ListBucketResult><Name>dataset</Name><IsTruncated>false</IsTruncated></ListBucketResult>`)
				}))
				defer server.Close()
				store := newListObjectsTestStorage(t, backend, server.URL, "", "root")
				keys, err := store.ListObjects(t.Context(), "dataset")
				if fail {
					if err == nil || keys != nil {
						t.Fatalf("denied listing = %q, %v", keys, err)
					}
				} else if err != nil || len(keys) != 0 {
					t.Fatalf("empty listing = %q, %v", keys, err)
				}
			})
		}
	}
}

// A successful first page must not hide a failure or cancellation on a later
// page. Keys on every page use the same logical bucket and prefix mapping.
func TestListObjectsReadsAllPages(t *testing.T) {
	for _, tc := range []struct{ name, backend, bucket, prefix, wantBucket, wantPrefix string }{
		{"s3 shared", "s3", "physical", "root", "physical", "root/dataset/"},
		{"s3 dedicated", "s3", "", "root", "dataset", "root/dataset/"},
		{"oss dedicated", "oss", "", "root", "dataset", "root/"},
	} {
		for _, outcome := range []string{"success", "second page failure", "cancel second page"} {
			t.Run(tc.name+"/"+outcome, func(t *testing.T) {
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				var requests atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					page := requests.Add(1)
					if r.URL.Path != "/"+tc.wantBucket || r.URL.Query().Get("prefix") != tc.wantPrefix || r.URL.Query().Get("list-type") != "2" {
						t.Errorf("unexpected list scope: %s", r.URL)
					}
					w.Header().Set("Content-Type", "application/xml")
					switch page {
					case 1:
						if r.URL.Query().Get("continuation-token") != "" {
							t.Errorf("unexpected first-page token: %s", r.URL)
						}
						fmt.Fprintf(w, `<ListBucketResult><IsTruncated>true</IsTruncated><NextContinuationToken>next/+ &amp; token</NextContinuationToken><Contents><Key>%sfirst.txt</Key></Contents></ListBucketResult>`, tc.wantPrefix)
					case 2:
						if r.URL.Query().Get("continuation-token") != "next/+ & token" {
							t.Errorf("unexpected continuation token: %s", r.URL)
						}
						if outcome == "second page failure" {
							w.WriteHeader(http.StatusForbidden)
							fmt.Fprint(w, `<Error><Code>AccessDenied</Code></Error>`)
							return
						}
						if outcome == "cancel second page" {
							cancel()
							<-r.Context().Done()
							return
						}
						fmt.Fprintf(w, `<ListBucketResult><IsTruncated>false</IsTruncated><Contents><Key>%ssecond.txt</Key></Contents></ListBucketResult>`, tc.wantPrefix)
					default:
						t.Errorf("unexpected extra request: %s", r.URL)
						w.WriteHeader(http.StatusBadRequest)
					}
				}))
				defer server.Close()
				store := newListObjectsTestStorage(t, tc.backend, server.URL, tc.bucket, tc.prefix)
				keys, err := store.ListObjects(ctx, "dataset")
				if outcome == "success" {
					if err != nil || !reflect.DeepEqual(keys, []string{"first.txt", "second.txt"}) {
						t.Fatalf("ListObjects = %q, %v; want both relative keys", keys, err)
					}
				} else if err == nil || keys != nil {
					t.Fatalf("ListObjects = %q, %v; must not return partial success", keys, err)
				}
				if requests.Load() != 2 {
					t.Fatalf("requests = %d, want 2", requests.Load())
				}
			})
		}
	}
}

func TestListObjectsRejectsInvalidContinuation(t *testing.T) {
	for _, repeated := range []bool{false, true} {
		t.Run(fmt.Sprintf("repeated=%v", repeated), func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				page := requests.Add(1)
				if page > 2 {
					t.Errorf("repeated continuation made an extra request")
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				w.Header().Set("Content-Type", "application/xml")
				if repeated {
					fmt.Fprint(w, `<ListBucketResult><IsTruncated>true</IsTruncated><NextContinuationToken>same</NextContinuationToken></ListBucketResult>`)
				} else {
					fmt.Fprint(w, `<ListBucketResult><IsTruncated>true</IsTruncated></ListBucketResult>`)
				}
			}))
			defer server.Close()
			store := newListObjectsTestStorage(t, "s3", server.URL, "physical", "root")
			keys, err := store.ListObjects(t.Context(), "dataset")
			if err == nil || keys != nil {
				t.Fatalf("invalid continuation = %q, %v; want error", keys, err)
			}
		})
	}
}
