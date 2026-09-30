package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/service/dataset"
	"ragflow/internal/service/document"
)

func uploadWithParserConfig(t *testing.T, parserConfig string) (*fakeDocumentService, *http.Response) {
	t.Helper()
	db := setupUploadHandlerDB(t, "normal")
	orig := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = orig })

	fake := &fakeDocumentService{uploadLocalData: []map[string]interface{}{
		{"id": "doc-1", "kb_id": uploadTestDatasetID, "name": "sales.csv"},
	}}
	h := &DocumentHandler{documentService: fake, datasetService: dataset.NewDatasetService()}

	fields := map[string]string{"type": "local"}
	if parserConfig != "" {
		fields["parser_config"] = parserConfig
	}
	c, w := setupUploadContext(t, "/api/v1/datasets/"+uploadTestDatasetID+"/documents?type=local",
		fields, "sales.csv", []byte("订单,金额\nDD-1,100\n"))
	h.UploadDocuments(c)

	return fake, w.Result()
}

func decodeResponseBody(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	return body
}

// TestUploadDocumentsAcceptsComponentScopedColumns is the contract the frontend
// sends: the column settings live on the TableChunker node, in the same shape
// the pipeline reads them.
func TestUploadDocumentsAcceptsComponentScopedColumns(t *testing.T) {
	fake, resp := uploadWithParserConfig(t, `{"TableChunker:FastFoxesJump":{"column_mode":"manual","column_roles":{"金额":"metadata"}}}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	override, ok := fake.uploadOverride["TableChunker:FastFoxesJump"].(map[string]any)
	if !ok {
		t.Fatalf("override lost the node: %v", fake.uploadOverride)
	}
	if override["column_mode"] != "manual" {
		t.Errorf("column_mode = %v", override["column_mode"])
	}
}

// TestUploadDocumentsRefusesRetiredFlatKeys: these uploads used to report
// success while the roles did nothing, which is worse than an error.
func TestUploadDocumentsRefusesRetiredFlatKeys(t *testing.T) {
	fake, resp := uploadWithParserConfig(t, `{"table_column_mode":"manual","table_column_roles":{"金额":"metadata"}}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want the JSON error envelope", resp.StatusCode)
	}
	body := decodeResponseBody(t, resp)
	if code := body["code"]; code != float64(common.CodeArgumentError) {
		t.Errorf("code = %v, want %d", code, common.CodeArgumentError)
	}
	message, _ := body["message"].(string)
	if !strings.Contains(message, "TableChunker") {
		t.Errorf("message does not name the replacement shape: %q", message)
	}
	if fake.uploadOverride != nil {
		t.Errorf("a refused upload still reached the service: %v", fake.uploadOverride)
	}
}

func TestUploadDocumentsRefusesInvalidColumnValues(t *testing.T) {
	cases := map[string]string{
		"unknown role":   `{"TableChunker:FastFoxesJump":{"column_roles":{"金额":"keyword"}}}`,
		"unknown mode":   `{"TableChunker:FastFoxesJump":{"column_mode":"assist"}}`,
		"roles not json": `{"TableChunker:FastFoxesJump":{"column_roles":"金额"}}`,
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			fake, resp := uploadWithParserConfig(t, payload)
			body := decodeResponseBody(t, resp)
			if code := body["code"]; code != float64(common.CodeArgumentError) {
				t.Errorf("code = %v, want %d", code, common.CodeArgumentError)
			}
			if fake.uploadOverride != nil {
				t.Errorf("a refused upload still reached the service: %v", fake.uploadOverride)
			}
		})
	}
}

// TestProbeTableColumnsReportsStableBusinessCodes: the client branches on
// data.error, not on the wording of a message.
func TestProbeTableColumnsReportsStableBusinessCodes(t *testing.T) {
	db := setupUploadHandlerDB(t, "normal")
	orig := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = orig })

	fake := &fakeDocumentService{tableProbeErr: &document.TableProbeError{
		Code:    document.TableProbeUnsupportedFormat,
		Message: `"notes.tsv" cannot be probed`,
	}}
	h := &DocumentHandler{documentService: fake, datasetService: dataset.NewDatasetService()}

	c, w := setupUploadContext(t, "/api/v1/datasets/ds-1/documents/probe-table",
		map[string]string{}, "notes.tsv", []byte("a\tb\n1\t2\n"))
	// The access check matches the stored dataset id, which the upload fixture
	// keeps without dashes.
	c.Params = gin.Params{{Key: "dataset_id", Value: "123e4567e89b12d3a456426614174000"}}
	h.ProbeTableColumns(c)

	body := decodeResponseBody(t, w.Result())
	if code := body["code"]; code != float64(common.CodeArgumentError) {
		t.Errorf("code = %v, want %d", code, common.CodeArgumentError)
	}
	data, _ := body["data"].(map[string]any)
	if data["error"] != document.TableProbeUnsupportedFormat {
		t.Errorf("data = %v, want the business code", body["data"])
	}
}
