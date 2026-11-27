//go:build integration

package tool

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestAnySearchExtractLiveFormatContracts(t *testing.T) {
	if os.Getenv("RAGFLOW_TEST_ANYSEARCH") != "1" {
		t.Skip("set RAGFLOW_TEST_ANYSEARCH=1 to call the real AnySearch service")
	}
	key := strings.TrimSpace(os.Getenv("ANYSEARCH_TEST_API_KEY"))
	for _, tc := range []struct {
		name, url string
		rejected  bool
	}{
		{"xhtml", "https://www.w3.org/People/mimasa/test/xhtml/media-types/test.xhtml", false},
		{"office", "https://raw.githubusercontent.com/infiniflow/ragflow/main/internal/parser/parser/testdata/docx_table_image.docx", true},
		{"image", "https://raw.githubusercontent.com/infiniflow/ragflow/main/internal/ingestion/component/testdata/picture_ocr.png", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			extract, transport := liveAnySearchExtract(key)
			ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
			defer cancel()
			out, err := extract.InvokableRun(ctx, anySearchExtractCall(tc.url))
			if err != nil {
				t.Fatal("extraction returned a Go error")
			}
			if key != "" && strings.Contains(out, key) {
				t.Fatal("extraction exposed the configured key")
			}
			env := decodeAnySearchExtractEnvelope(t, out)
			requests := transport.take()
			if len(requests) != 1 || requests[0].HasAuthorization != (key != "") || (key != "" && !requests[0].BearerMatches) || requests[0].Body["url"] != tc.url {
				t.Fatal("request did not preserve URL or authentication")
			}
			if tc.rejected {
				if requests[0].Status != 422 || !strings.Contains(env.Error, "HTTP 422") || env.Content != "" || env.RequestID == "" {
					t.Fatalf("unsupported format: HTTP=%d error=%q content_chars=%d", requests[0].Status, env.Error, env.ContentChars)
				}
			} else if env.Error != "" || !strings.Contains(env.Content, "XHTML") || env.RequestID == "" {
				t.Fatalf("XHTML extraction: HTTP=%d error=%q content_chars=%d", requests[0].Status, env.Error, env.ContentChars)
			}
			t.Logf("real vendor format=%s HTTP=%d request_id=%s content_chars=%d rejected=%v", tc.name, requests[0].Status, requests[0].RequestID, env.ContentChars, tc.rejected)
		})
	}
}
