package service

import (
	"errors"
	"testing"
	"time"

	"ragflow/internal/storage"
)

type fakeArtifactStorage struct {
	data      []byte
	err       error
	notExist  bool
	gotBucket string
	gotName   string
}

func (s *fakeArtifactStorage) Health() bool { return true }

func (s *fakeArtifactStorage) Put(bucket, fnm string, binary []byte, tenantID ...string) error {
	return nil
}

func (s *fakeArtifactStorage) Get(bucket, fnm string, tenantID ...string) ([]byte, error) {
	s.gotBucket = bucket
	s.gotName = fnm
	return s.data, s.err
}

func (s *fakeArtifactStorage) Remove(bucket, fnm string, tenantID ...string) error { return nil }

func (s *fakeArtifactStorage) ObjExist(bucket, fnm string, tenantID ...string) bool {
	return !s.notExist
}

func (s *fakeArtifactStorage) GetPresignedURL(bucket, fnm string, expires time.Duration, tenantID ...string) (string, error) {
	return "", nil
}

func (s *fakeArtifactStorage) BucketExists(bucket string) bool { return true }

func (s *fakeArtifactStorage) RemoveBucket(bucket string) error { return nil }

func (s *fakeArtifactStorage) Copy(srcBucket, srcPath, destBucket, destPath string) bool { return true }

func (s *fakeArtifactStorage) Move(srcBucket, srcPath, destBucket, destPath string) bool { return true }

func setDocumentArtifactStorage(t *testing.T, fake storage.Storage) {
	t.Helper()
	factory := storage.GetStorageFactory()
	prev := factory.GetStorage()
	factory.SetStorage(fake)
	t.Cleanup(func() {
		factory.SetStorage(prev)
	})
}

func TestGetDocumentArtifactSuccess(t *testing.T) {
	fake := &fakeArtifactStorage{data: []byte("image bytes")}
	setDocumentArtifactStorage(t, fake)
	t.Setenv("SANDBOX_ARTIFACT_BUCKET", "")

	artifact, err := (&DocumentService{}).GetDocumentArtifact("chart.png")
	if err != nil {
		t.Fatalf("GetDocumentArtifact returned error: %v", err)
	}
	if fake.gotBucket != "sandbox-artifacts" {
		t.Fatalf("expected default bucket sandbox-artifacts, got %q", fake.gotBucket)
	}
	if fake.gotName != "chart.png" {
		t.Fatalf("expected object name chart.png, got %q", fake.gotName)
	}
	if string(artifact.Data) != "image bytes" {
		t.Fatalf("unexpected artifact data %q", string(artifact.Data))
	}
	if artifact.ContentType != "image/png" {
		t.Fatalf("expected image/png, got %q", artifact.ContentType)
	}
	if artifact.SafeFilename != "chart.png" {
		t.Fatalf("expected safe filename chart.png, got %q", artifact.SafeFilename)
	}
	if artifact.ForceAttachment {
		t.Fatal("png should be returned inline")
	}
}

func TestGetDocumentArtifactUsesConfiguredBucket(t *testing.T) {
	fake := &fakeArtifactStorage{data: []byte("csv")}
	setDocumentArtifactStorage(t, fake)
	t.Setenv("SANDBOX_ARTIFACT_BUCKET", "custom-artifacts")

	if _, err := (&DocumentService{}).GetDocumentArtifact("result.csv"); err != nil {
		t.Fatalf("GetDocumentArtifact returned error: %v", err)
	}
	if fake.gotBucket != "custom-artifacts" {
		t.Fatalf("expected configured bucket, got %q", fake.gotBucket)
	}
}

func TestGetDocumentArtifactRejectsInvalidFilename(t *testing.T) {
	tests := []string{
		"folder/chart.png",
		`folder\chart.png`,
		"../chart.png",
	}

	for _, filename := range tests {
		t.Run(filename, func(t *testing.T) {
			_, err := (&DocumentService{}).GetDocumentArtifact(filename)
			if !errors.Is(err, ErrArtifactInvalidFilename) {
				t.Fatalf("expected ErrArtifactInvalidFilename, got %v", err)
			}
		})
	}
}

func TestGetDocumentArtifactRejectsInvalidType(t *testing.T) {
	_, err := (&DocumentService{}).GetDocumentArtifact("script.sh")
	if !errors.Is(err, ErrArtifactInvalidFileType) {
		t.Fatalf("expected ErrArtifactInvalidFileType, got %v", err)
	}
}

func TestGetDocumentArtifactTreatsEmptyDataAsNotFound(t *testing.T) {
	setDocumentArtifactStorage(t, &fakeArtifactStorage{data: []byte{}})

	_, err := (&DocumentService{}).GetDocumentArtifact("empty.pdf")
	if !errors.Is(err, ErrArtifactNotFound) {
		t.Fatalf("expected ErrArtifactNotFound, got %v", err)
	}
}

func TestGetDocumentArtifactTreatsMissingObjectAsNotFound(t *testing.T) {
	setDocumentArtifactStorage(t, &fakeArtifactStorage{notExist: true})

	_, err := (&DocumentService{}).GetDocumentArtifact("missing.png")
	if !errors.Is(err, ErrArtifactNotFound) {
		t.Fatalf("expected ErrArtifactNotFound, got %v", err)
	}
}

func TestGetDocumentArtifactForceAttachmentForUnsafeTypes(t *testing.T) {
	tests := []struct {
		filename    string
		contentType string
	}{
		{filename: "page.html", contentType: "text/html"},
		{filename: "image.svg", contentType: "image/svg+xml"},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			setDocumentArtifactStorage(t, &fakeArtifactStorage{data: []byte("data")})

			artifact, err := (&DocumentService{}).GetDocumentArtifact(tt.filename)
			if err != nil {
				t.Fatalf("GetDocumentArtifact returned error: %v", err)
			}
			if artifact.ContentType != tt.contentType {
				t.Fatalf("expected content type %q, got %q", tt.contentType, artifact.ContentType)
			}
			if !artifact.ForceAttachment {
				t.Fatal("expected unsafe artifact type to force attachment")
			}
		})
	}
}
