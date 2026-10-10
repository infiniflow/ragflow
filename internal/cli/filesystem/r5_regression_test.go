package filesystem

import (
	stdctx "context"
	"io"
	"net/url"
	"testing"
)

type datasetClient struct {
	paths   []string
	headers []map[string]string
}

func (f *datasetClient) Request(method, path, authKind string, headers map[string]string, body map[string]interface{}) (*HTTPResponse, error) {
	f.paths = append(f.paths, path)
	f.headers = append(f.headers, headers)
	response := `{"code":0,"data":{"docs":[]}}`
	if path == "/datasets" {
		response = `{"code":0,"data":[{"id":"kb1","name":"test"}]}`
	}
	return &HTTPResponse{StatusCode: 200, Body: []byte(response)}, nil
}
func (f *datasetClient) UploadMultipart(path, contentType string, body io.Reader) error { return nil }
func TestDatasetDocumentPaginationUsesQueryNotHeaders(t *testing.T) {
	client := &datasetClient{}
	p := NewDatasetProvider(client)
	_, err := p.listDocuments(stdctx.Background(), "test", &ListOptions{Limit: 10, Offset: 20})
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(client.paths[len(client.paths)-1])
	if u.Query().Get("page") != "3" || u.Query().Get("page_size") != "10" {
		t.Fatalf("pagination missing from URL: %s", u)
	}
	if len(client.headers[len(client.headers)-1]) != 0 {
		t.Fatal("pagination must not be sent as HTTP headers")
	}
}
func TestDatasetDocumentOffsetWithoutLimitDoesNotPanic(t *testing.T) {
	client := &datasetClient{}
	p := NewDatasetProvider(client)
	_, err := p.listDocuments(stdctx.Background(), "test", &ListOptions{Offset: 100})
	if err != nil {
		t.Fatal(err)
	}
}
