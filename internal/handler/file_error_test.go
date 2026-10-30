package handler

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/permission"

	"github.com/gin-gonic/gin"
)

func TestRespondFileServiceError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name string
		err  error
		code int
		msg  string
	}{{"authorization", permission.ErrPermissionDenied, int(common.CodeNotFound), "Resource not found"}} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			respondFileServiceError(c, tc.err)
			if w.Code != 200 {
				t.Fatalf("HTTP status = %d, want 200", w.Code)
			}
			var body struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body.Code != tc.code || body.Message != tc.msg {
				t.Fatalf("body = %s, want code=%d message=%q", w.Body, tc.code, tc.msg)
			}
		})
	}
}

// TestUploadFileHandler_CleansUpMultipartTempFiles verifies that temporary files
// spooled to disk during file uploads are removed after handler completion.
func TestUploadFileHandler_CleansUpMultipartTempFiles(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("parent_id", "root-1"); err != nil {
		t.Fatalf("write parent_id field: %v", err)
	}
	part, err := writer.CreateFormFile("other", "temp.txt")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	part.Write([]byte("large content to spool to disk"))
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/files", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	c.Request = req
	c.Set("user", &entity.User{ID: "u-1"})

	// Force multipart to disk
	if err := c.Request.ParseMultipartForm(1); err != nil {
		t.Fatalf("parse multipart: %v", err)
	}
	fh := c.Request.MultipartForm.File["other"][0]
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
		t.Fatalf("expected temp file to exist: %v", err)
	}

	h := &FileHandler{}
	h.UploadFile(c)

	if _, err := os.Stat(tempPath); !os.IsNotExist(err) {
		t.Fatalf("expected temp file to be removed after handler, stat err: %v", err)
	}
}

