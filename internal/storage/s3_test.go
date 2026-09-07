package storage

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type s3RoundTripper func(*http.Request) (*http.Response, error)

func (f s3RoundTripper) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func newS3TestStorage(handler s3RoundTripper) *S3Storage {
	client := s3.NewFromConfig(aws.Config{
		Region:      "us-east-1",
		Credentials: aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider("access", "secret", "")),
		HTTPClient:  &http.Client{Transport: handler},
	}, func(options *s3.Options) {
		options.BaseEndpoint = aws.String("https://s3.test")
		options.UsePathStyle = true
		options.RetryMaxAttempts = 1
	})
	return &S3Storage{client: client}
}

func s3Response(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}

func TestS3StorageRemoveBucketDeletesEmptyPhysicalBucket(t *testing.T) {
	bucketDeleted := false
	storage := newS3TestStorage(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.Method == http.MethodHead:
			return s3Response(http.StatusOK, ""), nil
		case req.URL.Query().Has("versions"):
			return s3Response(http.StatusOK, `<ListVersionsResult><IsTruncated>false</IsTruncated></ListVersionsResult>`), nil
		case req.Method == http.MethodDelete:
			if req.URL.Path != "/kb01" {
				t.Fatalf("DeleteBucket path = %q, want /kb01", req.URL.Path)
			}
			bucketDeleted = true
			return s3Response(http.StatusNoContent, ""), nil
		}
		t.Fatalf("unexpected request: %s %s", req.Method, req.URL)
		return nil, nil
	})
	if err := storage.RemoveBucket(t.Context(), "kb01"); err != nil {
		t.Fatal(err)
	}
	if !bucketDeleted {
		t.Fatal("RemoveBucket() did not delete the empty physical bucket")
	}
}

func TestS3StorageRemoveBucketSingleBucketMode(t *testing.T) {
	var deleteBody string
	storage := newS3TestStorage(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.Method == http.MethodHead:
			if req.URL.Path != "/physical" {
				t.Fatalf("HeadBucket path = %q, want /physical", req.URL.Path)
			}
			return s3Response(http.StatusOK, ""), nil
		case req.URL.Query().Has("versions"):
			if req.URL.Query().Get("prefix") != "prefix/kb01/" {
				t.Fatalf("ListObjectVersions prefix = %q, want prefix/kb01/", req.URL.Query().Get("prefix"))
			}
			return s3Response(http.StatusOK, `<ListVersionsResult><IsTruncated>false</IsTruncated><Version><Key>prefix/kb01/document</Key><VersionId>v1</VersionId></Version></ListVersionsResult>`), nil
		case req.URL.Query().Has("delete"):
			body, _ := io.ReadAll(req.Body)
			deleteBody = string(body)
			return s3Response(http.StatusOK, `<DeleteResult/>`), nil
		case req.Method == http.MethodDelete:
			t.Fatalf("RemoveBucket deleted physical bucket in single-bucket mode")
		}
		t.Fatalf("unexpected request: %s %s", req.Method, req.URL)
		return nil, nil
	})
	storage.bucket = "physical"
	storage.prefixPath = "prefix"
	if err := storage.RemoveBucket(t.Context(), "kb01"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(deleteBody, `<Key>prefix/kb01/document</Key><VersionId>v1</VersionId>`) {
		t.Fatalf("DeleteObjects omitted matching version: %s", deleteBody)
	}
}

func TestS3StorageRemoveBucketDeletesVersionsAndMarkers(t *testing.T) {
	var deleteBody string
	storage := newS3TestStorage(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.Method == http.MethodHead:
			return s3Response(http.StatusOK, ""), nil
		case req.URL.Query().Has("versions"):
			return s3Response(http.StatusOK, `<ListVersionsResult><IsTruncated>false</IsTruncated><Version><Key>current</Key><VersionId>v2</VersionId></Version><Version><Key>old</Key><VersionId>v1</VersionId></Version><DeleteMarker><Key>removed</Key><VersionId>m1</VersionId></DeleteMarker></ListVersionsResult>`), nil
		case req.URL.Query().Has("delete"):
			body, _ := io.ReadAll(req.Body)
			deleteBody = string(body)
			return s3Response(http.StatusOK, `<DeleteResult/>`), nil
		case req.Method == http.MethodDelete:
			return s3Response(http.StatusNoContent, ""), nil
		}
		t.Fatalf("unexpected request: %s %s", req.Method, req.URL)
		return nil, nil
	})
	if err := storage.RemoveBucket(t.Context(), "kb01"); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"<Key>current</Key><VersionId>v2</VersionId>", "<Key>old</Key><VersionId>v1</VersionId>", "<Key>removed</Key><VersionId>m1</VersionId>"} {
		if !strings.Contains(deleteBody, value) {
			t.Fatalf("DeleteObjects did not preserve %q: %s", value, deleteBody)
		}
	}
}

func TestS3StorageRemoveBucketDeletesNullVersion(t *testing.T) {
	var deleteBody string
	storage := newS3TestStorage(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.Method == http.MethodHead:
			return s3Response(http.StatusOK, ""), nil
		case req.URL.Query().Has("versions"):
			return s3Response(http.StatusOK, `<ListVersionsResult><IsTruncated>false</IsTruncated><Version><Key>document</Key><VersionId>null</VersionId></Version></ListVersionsResult>`), nil
		case req.URL.Query().Has("delete"):
			body, _ := io.ReadAll(req.Body)
			deleteBody = string(body)
			return s3Response(http.StatusOK, `<DeleteResult/>`), nil
		case req.Method == http.MethodDelete:
			return s3Response(http.StatusNoContent, ""), nil
		}
		t.Fatalf("unexpected request: %s %s", req.Method, req.URL)
		return nil, nil
	})
	if err := storage.RemoveBucket(t.Context(), "kb01"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(deleteBody, `<Key>document</Key><VersionId>null</VersionId>`) {
		t.Fatalf("DeleteObjects omitted the null version ID: %s", deleteBody)
	}
}

func TestS3StorageRemoveBucketVersionPagination(t *testing.T) {
	versionCalls := 0
	storage := newS3TestStorage(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.Method == http.MethodHead:
			return s3Response(http.StatusOK, ""), nil
		case req.URL.Query().Has("versions"):
			versionCalls++
			if versionCalls == 1 {
				return s3Response(http.StatusOK, `<ListVersionsResult><IsTruncated>true</IsTruncated><NextKeyMarker>a</NextKeyMarker><NextVersionIdMarker>v1</NextVersionIdMarker><Version><Key>a</Key><VersionId>v1</VersionId></Version></ListVersionsResult>`), nil
			}
			if req.URL.Query().Get("key-marker") != "a" || req.URL.Query().Get("version-id-marker") != "v1" {
				t.Fatalf("missing version continuation markers: %s", req.URL)
			}
			return s3Response(http.StatusOK, `<ListVersionsResult><IsTruncated>false</IsTruncated><DeleteMarker><Key>b</Key><VersionId>m1</VersionId></DeleteMarker></ListVersionsResult>`), nil
		case req.URL.Query().Has("delete"):
			return s3Response(http.StatusOK, `<DeleteResult/>`), nil
		case req.Method == http.MethodDelete:
			return s3Response(http.StatusNoContent, ""), nil
		}
		t.Fatalf("unexpected request: %s %s", req.Method, req.URL)
		return nil, nil
	})
	if err := storage.RemoveBucket(t.Context(), "kb01"); err != nil {
		t.Fatal(err)
	}
	if versionCalls != 2 {
		t.Fatalf("ListObjectVersions calls = %d, want 2", versionCalls)
	}
}

func TestS3StorageRemoveBucketFailures(t *testing.T) {
	t.Run("batch limit", func(t *testing.T) {
		requests := 0
		storage := newS3TestStorage(func(req *http.Request) (*http.Response, error) {
			if !req.URL.Query().Has("delete") {
				t.Fatalf("unexpected request: %s %s", req.Method, req.URL)
			}
			requests++
			return s3Response(http.StatusOK, `<DeleteResult/>`), nil
		})
		objects := make([]types.ObjectIdentifier, 1001)
		for i := range objects {
			objects[i].Key = aws.String("key")
		}
		if err := storage.deleteObjects(t.Context(), "kb01", objects); err != nil {
			t.Fatal(err)
		}
		if requests != 2 {
			t.Fatalf("DeleteObjects requests = %d, want 2", requests)
		}
	})

	for _, test := range []struct {
		name, body string
		status     int
	}{
		{"list failure", `<Error><Code>InternalError</Code></Error>`, http.StatusInternalServerError},
		{"embedded delete error", `<DeleteResult><Error><Key>a</Key><Code>AccessDenied</Code><Message>denied</Message></Error></DeleteResult>`, http.StatusOK},
	} {
		t.Run(test.name, func(t *testing.T) {
			bucketDeleted := false
			storage := newS3TestStorage(func(req *http.Request) (*http.Response, error) {
				switch {
				case req.Method == http.MethodHead:
					return s3Response(http.StatusOK, ""), nil
				case req.URL.Query().Has("versions"):
					if test.name == "list failure" {
						return s3Response(test.status, test.body), nil
					}
					return s3Response(http.StatusOK, `<ListVersionsResult><IsTruncated>false</IsTruncated><Version><Key>a</Key><VersionId>v1</VersionId></Version></ListVersionsResult>`), nil
				case req.URL.Query().Has("delete"):
					return s3Response(test.status, test.body), nil
				case req.Method == http.MethodDelete:
					bucketDeleted = true
					return s3Response(http.StatusNoContent, ""), nil
				}
				t.Fatalf("unexpected request: %s %s", req.Method, req.URL)
				return nil, nil
			})
			if err := storage.RemoveBucket(t.Context(), "kb01"); err == nil {
				t.Fatal("RemoveBucket() returned nil after cleanup failure")
			}
			if bucketDeleted {
				t.Fatal("RemoveBucket() deleted the physical bucket after cleanup failure")
			}
		})
	}
}

func TestS3StorageRemoveBucketHeadBucketErrors(t *testing.T) {
	for _, test := range []struct {
		name    string
		status  int
		code    string
		wantErr bool
	}{
		{"missing bucket", http.StatusNotFound, "NotFound", false},
		{"access denied", http.StatusForbidden, "AccessDenied", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			destructiveRequest := false
			storage := newS3TestStorage(func(req *http.Request) (*http.Response, error) {
				if req.Method == http.MethodHead {
					return s3Response(test.status, `<Error><Code>`+test.code+`</Code></Error>`), nil
				}
				destructiveRequest = true
				return s3Response(http.StatusInternalServerError, ""), nil
			})
			err := storage.RemoveBucket(t.Context(), "kb01")
			if (err != nil) != test.wantErr {
				t.Fatalf("RemoveBucket() error = %v, want error: %t", err, test.wantErr)
			}
			if destructiveRequest {
				t.Fatal("RemoveBucket() performed cleanup after HeadBucket failure")
			}
		})
	}
}
