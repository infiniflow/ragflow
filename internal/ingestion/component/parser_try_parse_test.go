package component

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"ragflow/internal/agent/runtime"
	"ragflow/internal/common"
)

func TestParserCorrectionUsesTargetSetupForRemoteParsing(t *testing.T) {
	withSSRFBypass(t)
	f := excelize.NewFile()
	defer f.Close()
	if err := f.SetCellValue("Sheet1", "A1", "Workbook"); err != nil {
		t.Fatal(err)
	}
	data, err := f.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	payload := tcadpZipFixtureForComponent(t)
	var receivedType string
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/reconstruct_document":
			var request struct {
				FileType string `json:"file_type"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
			}
			receivedType = request.FileType
			_, _ = w.Write([]byte(`{"DocumentRecognizeResultUrl":"` + server.URL + `/download.zip"}`))
		case "/download.zip":
			_, _ = w.Write(payload)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	setups := defaultSetups()
	setups["spreadsheet"]["parse_method"] = "tcadp"
	setups["spreadsheet"]["tcadp_apiserver"] = server.URL
	c := &ParserComponent{setups: setups}
	out, err := c.Invoke(t.Context(), nil, map[string]any{"name": "renamed.doc", "binary": data.Bytes()})
	if err != nil {
		t.Fatal(err)
	}
	if receivedType != "XLSX" {
		t.Fatalf("remote file_type=%q, want XLSX", receivedType)
	}
	if out["file_type"] != "xlsx" {
		t.Fatalf("output file_type=%v", out["file_type"])
	}
	requireJSONText(t, out, "Hello TCADP")
}

func TestParserComponentCorrectsDOCXUploadedAsDOC(t *testing.T) {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, content := range map[string]string{
		"[Content_Types].xml": `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`,
		"_rels/.rels":         `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>`,
		"word/document.xml":   `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>Recognized document</w:t></w:r></w:p></w:body></w:document>`,
	} {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	c, err := NewParserComponent(map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	core, logs := observer.New(zap.WarnLevel)
	originalLogger := common.Logger
	common.Logger = zap.New(core)
	t.Cleanup(func() { common.Logger = originalLogger })
	for _, suffix := range []string{"doc", "xls", "ppt", "wps", "docx"} {
		t.Run(suffix, func(t *testing.T) {
			var messages []string
			ctx := runtime.WithProgressMessageCallback(t.Context(), func(_, message string) { messages = append(messages, message) })
			logs.TakeAll()
			name := "renamed." + suffix
			out, err := c.Invoke(ctx, nil, map[string]any{"name": name, "binary": buf.Bytes()})
			if err != nil {
				t.Fatal(err)
			}
			if out["file_type"] != "docx" || out["name"] != name {
				t.Fatalf("incorrect output metadata: type=%v name=%v", out["file_type"], out["name"])
			}
			requireJSONText(t, out, "Recognized document")
			if suffix == "docx" {
				if len(messages) != 0 || logs.Len() != 0 {
					t.Fatal("matching format should not warn")
				}
				return
			}
			if len(messages) != 1 || !strings.Contains(messages[0], "WARNING:") || !strings.Contains(messages[0], "docx") {
				t.Fatalf("expected correction warning, got %v", messages)
			}
			if logs.Len() != 1 {
				t.Fatalf("expected one server warning, got %d", logs.Len())
			}
		})
	}
}

func TestParserComponentKeepsUnrecognizedFormat(t *testing.T) {
	c, err := NewParserComponent(map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	var messages []string
	ctx := runtime.WithProgressMessageCallback(t.Context(), func(_, message string) { messages = append(messages, message) })
	_, err = c.Invoke(ctx, nil, map[string]any{"name": "broken.doc", "binary": []byte("unrecognized content")})
	if err == nil || !strings.Contains(err.Error(), `"doc"`) {
		t.Fatalf("expected original DOC parser error, got %v", err)
	}
	if len(messages) != 0 {
		t.Fatalf("unknown content should not trigger a correction warning: %v", messages)
	}
}

func TestParserComponentSkipsUnavailableTypeCheck(t *testing.T) {
	c, err := NewParserComponent(map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	var messages []string
	ctx := runtime.WithProgressMessageCallback(t.Context(), func(_, message string) { messages = append(messages, message) })
	out, err := c.Invoke(ctx, nil, map[string]any{"name": "notes.txt", "binary": []byte("Unchanged text")})
	if err != nil {
		t.Fatal(err)
	}
	if out["file_type"] != "txt" || len(messages) != 0 {
		t.Fatalf("unexpected type correction: type=%v messages=%v", out["file_type"], messages)
	}
	requireJSONText(t, out, "Unchanged text")
}
