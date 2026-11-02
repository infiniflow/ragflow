package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"ragflow/internal/server/config"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type s3RoundTripper func(*http.Request) (*http.Response, error)

func (f s3RoundTripper) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func newS3TestStorage(region string, handler s3RoundTripper) *S3Storage {
	client := s3.NewFromConfig(aws.Config{
		Region:      region,
		Credentials: aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider("access", "secret", "")),
		HTTPClient:  &http.Client{Transport: handler},
	}, func(options *s3.Options) {
		options.BaseEndpoint = aws.String("https://s3.test")
		options.UsePathStyle = true
		options.RetryMaxAttempts = 1
	})
	s := &S3Storage{client: client}
	s.s3Base = s3Base{
		client:     func() *s3.Client { return s.client },
		bucket:     "",
		prefixPath: "",
		typeName:   "s3",
		// Identity folding: these tests exercise the shared surface with no
		// default bucket / prefix configured.
		resolve:    func(bucket, fnm string) (string, string) { return bucket, fnm },
		makeBucket: s.createBucket,
		reconnect:  func(context.Context) {},
	}
	return s
}

func s3Response(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}

func TestS3StorageConnectAppliesAddressingStyle(t *testing.T) {
	for _, test := range []struct {
		name            string
		addressingStyle string
		wantPathStyle   bool
	}{
		{name: "path", addressingStyle: "path", wantPathStyle: true},
		{name: "virtual", addressingStyle: "virtual"},
		{name: "unset"},
	} {
		t.Run(test.name, func(t *testing.T) {
			storage, err := NewS3Storage(t.Context(), config.S3Config{
				AccessKey:       "access",
				SecretKey:       "secret",
				Region:          "us-east-1",
				AddressingStyle: test.addressingStyle,
			})
			if err != nil {
				t.Fatal(err)
			}
			if got := storage.client.Options().UsePathStyle; got != test.wantPathStyle {
				t.Fatalf("UsePathStyle = %t, want %t", got, test.wantPathStyle)
			}
		})
	}
}

func TestS3StorageConnectConfiguresPayloadSigning(t *testing.T) {
	t.Setenv("AWS_REQUEST_CHECKSUM_CALCULATION", "when_supported")
	payload := []byte("content")
	for _, test := range []struct {
		name                string
		signatureVersion    string
		wantPayloadHash     string
		wantContentEncoding string
		wantBody            string
	}{
		{name: "s3", signatureVersion: "s3", wantPayloadHash: "UNSIGNED-PAYLOAD", wantBody: string(payload)},
		{name: "v4", signatureVersion: "v4", wantPayloadHash: "STREAMING-UNSIGNED-PAYLOAD-TRAILER", wantContentEncoding: "aws-chunked"},
		{name: "s3v4", signatureVersion: "s3v4", wantPayloadHash: "STREAMING-UNSIGNED-PAYLOAD-TRAILER", wantContentEncoding: "aws-chunked"},
		{name: "unset", wantPayloadHash: "STREAMING-UNSIGNED-PAYLOAD-TRAILER", wantContentEncoding: "aws-chunked"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var gotPutPayloadHash string
			var gotPutContentEncoding string
			var gotPutBody string
			var gotHeadPayloadHash string
			var gotDeletePayloadHash string
			var gotDeleteContentEncoding string
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				if req.URL.Query().Has("delete") {
					gotDeletePayloadHash = req.Header.Get("x-amz-content-sha256")
					gotDeleteContentEncoding = req.Header.Get("Content-Encoding")
					_, _ = w.Write([]byte("<DeleteResult/>"))
					return
				}
				switch req.Method {
				case http.MethodPut:
					gotPutPayloadHash = req.Header.Get("x-amz-content-sha256")
					gotPutContentEncoding = req.Header.Get("Content-Encoding")
					body, err := io.ReadAll(req.Body)
					if err != nil {
						t.Error(err)
					}
					gotPutBody = string(body)
				case http.MethodHead:
					gotHeadPayloadHash = req.Header.Get("x-amz-content-sha256")
				}
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			storage, err := NewS3Storage(t.Context(), config.S3Config{
				AccessKey:        "access",
				SecretKey:        "secret",
				Region:           "us-east-1",
				EndpointURL:      server.URL,
				SignatureVersion: test.signatureVersion,
				AddressingStyle:  "path",
			})
			if err != nil {
				t.Fatal(err)
			}
			storage.client = s3.New(storage.client.Options(), func(options *s3.Options) {
				options.HTTPClient = server.Client()
			})
			_, err = storage.client.PutObject(t.Context(), &s3.PutObjectInput{
				Bucket: aws.String("bucket"),
				Key:    aws.String("object"),
				Body:   bytes.NewReader(payload),
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = storage.client.HeadBucket(t.Context(), &s3.HeadBucketInput{Bucket: aws.String("bucket")}); err != nil {
				t.Fatal(err)
			}
			if _, err = storage.client.DeleteObjects(t.Context(), &s3.DeleteObjectsInput{
				Bucket: aws.String("bucket"),
				Delete: &types.Delete{Objects: []types.ObjectIdentifier{{Key: aws.String("object")}}},
			}); err != nil {
				t.Fatal(err)
			}
			if gotPutPayloadHash != test.wantPayloadHash {
				t.Fatalf("PutObject x-amz-content-sha256 = %q, want %q", gotPutPayloadHash, test.wantPayloadHash)
			}
			if gotPutContentEncoding != test.wantContentEncoding {
				t.Fatalf("PutObject Content-Encoding = %q, want %q", gotPutContentEncoding, test.wantContentEncoding)
			}
			if test.wantBody != "" && gotPutBody != test.wantBody {
				t.Fatalf("PutObject body = %q, want %q", gotPutBody, test.wantBody)
			}
			wantHeadPayloadHash := fmt.Sprintf("%x", sha256.Sum256(nil))
			if test.signatureVersion == "s3" {
				wantHeadPayloadHash = "UNSIGNED-PAYLOAD"
			}
			if gotHeadPayloadHash != wantHeadPayloadHash {
				t.Fatalf("HeadBucket x-amz-content-sha256 = %q, want %q", gotHeadPayloadHash, wantHeadPayloadHash)
			}
			if test.signatureVersion == "s3" {
				if gotDeletePayloadHash != "UNSIGNED-PAYLOAD" {
					t.Fatalf("DeleteObjects x-amz-content-sha256 = %q, want UNSIGNED-PAYLOAD", gotDeletePayloadHash)
				}
				if gotDeleteContentEncoding != "" {
					t.Fatalf("DeleteObjects Content-Encoding = %q, want empty", gotDeleteContentEncoding)
				}
			}
		})
	}
}

func TestS3StoragePutCreatesBucketInResolvedRegion(t *testing.T) {
	for _, test := range []struct {
		region         string
		wantConstraint bool
	}{
		{region: "eu-west-1", wantConstraint: true},
		{region: "us-east-1", wantConstraint: false},
		{region: "auto", wantConstraint: false},
	} {
		t.Run(test.region, func(t *testing.T) {
			var createBody string
			storage := newS3TestStorage(test.region, func(req *http.Request) (*http.Response, error) {
				switch {
				case req.Method == http.MethodHead && req.URL.Path == "/kb01":
					return s3Response(http.StatusNotFound, `<Error><Code>NotFound</Code></Error>`), nil
				case req.Method == http.MethodPut && req.URL.Path == "/kb01":
					if req.Body != nil {
						body, err := io.ReadAll(req.Body)
						if err != nil {
							t.Fatal(err)
						}
						createBody = string(body)
					}
					return s3Response(http.StatusOK, ""), nil
				case req.Method == http.MethodPut && req.URL.Path == "/kb01/document":
					return s3Response(http.StatusOK, ""), nil
				}
				t.Fatalf("unexpected request: %s %s", req.Method, req.URL)
				return nil, nil
			})

			if err := storage.Put(t.Context(), "kb01", "document", []byte("content")); err != nil {
				t.Fatal(err)
			}
			hasConstraint := strings.Contains(createBody, "<LocationConstraint>"+test.region+"</LocationConstraint>")
			if hasConstraint != test.wantConstraint {
				t.Fatalf("CreateBucket body = %q, want location constraint: %t", createBody, test.wantConstraint)
			}
		})
	}
}

func TestS3StorageHealthCreatesBucketInResolvedRegion(t *testing.T) {
	var createBody string
	storage := newS3TestStorage("ap-southeast-2", func(req *http.Request) (*http.Response, error) {
		switch {
		case req.Method == http.MethodHead && req.URL.Path == "/health-check-bucket":
			return s3Response(http.StatusNotFound, `<Error><Code>NotFound</Code></Error>`), nil
		case req.Method == http.MethodPut && req.URL.Path == "/health-check-bucket":
			body, err := io.ReadAll(req.Body)
			if err != nil {
				t.Fatal(err)
			}
			createBody = string(body)
			return s3Response(http.StatusOK, ""), nil
		case req.Method == http.MethodPut && req.URL.Path == "/health-check-bucket/txtxtxtxt1":
			return s3Response(http.StatusOK, ""), nil
		}
		t.Fatalf("unexpected request: %s %s", req.Method, req.URL)
		return nil, nil
	})

	if !storage.Health(t.Context()) {
		t.Fatal("Health() = false, want true")
	}
	if !strings.Contains(createBody, "<LocationConstraint>ap-southeast-2</LocationConstraint>") {
		t.Fatalf("CreateBucket body = %q, want ap-southeast-2 location constraint", createBody)
	}
}

func TestS3StorageRemoveBucketDeletesEmptyPhysicalBucket(t *testing.T) {
	bucketDeleted := false
	storage := newS3TestStorage("us-east-1", func(req *http.Request) (*http.Response, error) {
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

func TestS3StorageRemoveEmptyBucketPreservesNonemptyPhysicalBucket(t *testing.T) {
	storage := newS3TestStorage("us-east-1", func(req *http.Request) (*http.Response, error) {
		switch req.Method {
		case http.MethodHead:
			return s3Response(http.StatusOK, ""), nil
		case http.MethodDelete:
			return s3Response(http.StatusConflict, `<Error><Code>BucketNotEmpty</Code></Error>`), nil
		}
		t.Fatalf("unexpected request: %s %s", req.Method, req.URL)
		return nil, nil
	})
	if err := storage.RemoveEmptyBucket(t.Context(), "kb01"); err == nil {
		t.Fatal("RemoveEmptyBucket returned nil for a nonempty bucket")
	}
}

func TestS3StorageExistenceChecksDistinguishMissingFromFailure(t *testing.T) {
	for _, test := range []struct {
		name    string
		status  int
		code    string
		missing bool
	}{
		{name: "missing", status: http.StatusNotFound, code: "NoSuchBucket", missing: true},
		{name: "unavailable", status: http.StatusInternalServerError, code: "InternalError"},
	} {
		t.Run(test.name, func(t *testing.T) {
			storage := newS3TestStorage("us-east-1", func(req *http.Request) (*http.Response, error) {
				return s3Response(test.status, `<Error><Code>`+test.code+`</Code></Error>`), nil
			})
			objectExists, objectErr := storage.ObjectExists(t.Context(), "kb01", "file")
			bucketExists, bucketErr := storage.BucketExistsWithError(t.Context(), "kb01")
			if objectExists || bucketExists {
				t.Fatal("existence check reported an existing item")
			}
			if test.missing && (objectErr != nil || bucketErr != nil) {
				t.Fatalf("missing item returned errors: object=%v bucket=%v", objectErr, bucketErr)
			}
			if !test.missing && (objectErr == nil || bucketErr == nil) {
				t.Fatalf("storage failure was treated as missing: object=%v bucket=%v", objectErr, bucketErr)
			}
		})
	}
}

func TestS3StorageRemoveEmptyBucketDeletesEmptyPhysicalBucket(t *testing.T) {
	deleted := false
	storage := newS3TestStorage("us-east-1", func(req *http.Request) (*http.Response, error) {
		switch req.Method {
		case http.MethodHead:
			return s3Response(http.StatusOK, ""), nil
		case http.MethodDelete:
			deleted = true
			return s3Response(http.StatusNoContent, ""), nil
		}
		t.Fatalf("unexpected request: %s %s", req.Method, req.URL)
		return nil, nil
	})
	if err := storage.RemoveEmptyBucket(t.Context(), "kb01"); err != nil {
		t.Fatal(err)
	}
	if !deleted {
		t.Fatal("RemoveEmptyBucket did not delete an empty bucket")
	}
}

func TestS3StorageRemoveEmptyBucketPreservesLogicalBucketObjects(t *testing.T) {
	storage := newS3TestStorage("us-east-1", func(req *http.Request) (*http.Response, error) {
		switch {
		case req.Method == http.MethodHead:
			return s3Response(http.StatusOK, ""), nil
		case req.URL.Query().Has("versions"):
			if got := req.URL.Query().Get("prefix"); got != "prefix/kb01/" {
				t.Fatalf("prefix = %q", got)
			}
			return s3Response(http.StatusOK, `<ListVersionsResult><Version><Key>prefix/kb01/file</Key><VersionId>v1</VersionId></Version></ListVersionsResult>`), nil
		}
		t.Fatalf("unexpected request: %s %s", req.Method, req.URL)
		return nil, nil
	})
	storage.bucket = "physical"
	storage.prefixPath = "prefix"
	if err := storage.RemoveEmptyBucket(t.Context(), "kb01"); err == nil {
		t.Fatal("RemoveEmptyBucket returned nil for a nonempty logical bucket")
	}
}

func TestS3StorageRemoveBucketSingleBucketMode(t *testing.T) {
	var deleteBody string
	storage := newS3TestStorage("us-east-1", func(req *http.Request) (*http.Response, error) {
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
	storage := newS3TestStorage("us-east-1", func(req *http.Request) (*http.Response, error) {
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
	storage := newS3TestStorage("us-east-1", func(req *http.Request) (*http.Response, error) {
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
	storage := newS3TestStorage("us-east-1", func(req *http.Request) (*http.Response, error) {
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
		storage := newS3TestStorage("us-east-1", func(req *http.Request) (*http.Response, error) {
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
			storage := newS3TestStorage("us-east-1", func(req *http.Request) (*http.Response, error) {
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
			storage := newS3TestStorage("us-east-1", func(req *http.Request) (*http.Response, error) {
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
