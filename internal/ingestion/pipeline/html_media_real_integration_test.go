//go:build integration

package pipeline

import (
	"encoding/base64"
	"os"
	"testing"
)

func TestHTMLMediaRealServices(t *testing.T) {
	if os.Getenv("RAGFLOW_REAL_HTML_MEDIA_TEST") != "1" {
		t.Skip("set RAGFLOW_REAL_HTML_MEDIA_TEST=1 with configured MySQL, Kvrocks and MinIO")
	}
	const png64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jRZkAAAAASUVORK5CYII="
	png, err := base64.StdEncoding.DecodeString(png64)
	if err != nil {
		t.Fatal(err)
	}
	source := []byte("<p>HTML image fixture</p><img alt='sample figure' src='data:image/png;base64," + png64 + "'>")
	runMediaRealServices(t, "html", source, png)
}
