package admin

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"testing"

	"github.com/gin-gonic/gin"
)

func getTempFilePath(f multipart.File) string {
	if n, ok := f.(interface{ Name() string }); ok {
		return n.Name()
	}
	v := reflect.ValueOf(f)
	if v.Kind() == reflect.Struct {
		closerField := v.FieldByName("Closer")
		if closerField.IsValid() && !closerField.IsNil() {
			if n, ok := closerField.Interface().(interface{ Name() string }); ok {
				return n.Name()
			}
		}
	}
	return ""
}

// TestUploadSensitiveWords_CleansUpMultipartTempFiles verifies that temporary files
// spooled to disk during sensitive word uploads are removed after handler completion.
func TestUploadSensitiveWords_CleansUpMultipartTempFiles(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	// Create two files so it triggers "Only one file is allowed" and returns early
	part1, err := writer.CreateFormFile("file", "word1.txt")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	part1.Write([]byte("sensitive word content spooled to disk"))
	part2, err := writer.CreateFormFile("file", "word2.txt")
	if err != nil {
		t.Fatalf("create form file 2: %v", err)
	}
	part2.Write([]byte("second word content"))
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/admin/api/v1/sensitive-words", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	c.Request = req

	// Force multipart to disk by parsing with 1 byte limit
	if err := c.Request.ParseMultipartForm(1); err != nil {
		t.Fatalf("parse multipart: %v", err)
	}
	fh := c.Request.MultipartForm.File["file"][0]
	f, err := fh.Open()
	if err != nil {
		t.Fatalf("open file header: %v", err)
	}
	tempPath := getTempFilePath(f)
	f.Close()
	if tempPath == "" {
		t.Fatal("opened multipart file does not expose temporary path")
	}

	if _, err := os.Stat(tempPath); err != nil {
		t.Fatalf("expected temp file to exist before handler: %v", err)
	}

	h := &Handler{}
	h.UploadSensitiveWords(c)

	if _, err := os.Stat(tempPath); !os.IsNotExist(err) {
		t.Fatalf("expected temp file to be removed after handler, stat err: %v", err)
	}
}

// TestBatchAddWhiteList_CleansUpMultipartTempFiles verifies that temporary files
// spooled to disk during whitelist uploads are removed after handler completion.
func TestBatchAddWhiteList_CleansUpMultipartTempFiles(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	// Create two files so it triggers "Only one file is allowed" and returns early
	part1, err := writer.CreateFormFile("file", "white1.txt")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	part1.Write([]byte("whitelist content spooled to disk"))
	part2, err := writer.CreateFormFile("file", "white2.txt")
	if err != nil {
		t.Fatalf("create form file 2: %v", err)
	}
	part2.Write([]byte("second whitelist content"))
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/admin/api/v1/white-list/batch", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	c.Request = req

	// Force multipart to disk by parsing with 1 byte limit
	if err := c.Request.ParseMultipartForm(1); err != nil {
		t.Fatalf("parse multipart: %v", err)
	}
	fh := c.Request.MultipartForm.File["file"][0]
	f, err := fh.Open()
	if err != nil {
		t.Fatalf("open file header: %v", err)
	}
	tempPath := getTempFilePath(f)
	f.Close()
	if tempPath == "" {
		t.Fatal("opened multipart file does not expose temporary path")
	}

	if _, err := os.Stat(tempPath); err != nil {
		t.Fatalf("expected temp file to exist before handler: %v", err)
	}

	h := &Handler{}
	h.BatchAddWhiteList(c)

	if _, err := os.Stat(tempPath); !os.IsNotExist(err) {
		t.Fatalf("expected temp file to be removed after handler, stat err: %v", err)
	}
}
