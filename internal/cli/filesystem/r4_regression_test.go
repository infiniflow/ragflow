package filesystem

import (
	stdctx "context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

type fakeHTTPClient struct {
	requests []string
	total    int
}

func (f *fakeHTTPClient) Request(method, path, authKind string, headers map[string]string, jsonBody map[string]interface{}) (*HTTPResponse, error) {
	f.requests = append(f.requests, path)
	u, _ := url.Parse(path)
	page, _ := strconv.Atoi(u.Query().Get("page"))
	pageSize, _ := strconv.Atoi(u.Query().Get("page_size"))
	start := (page - 1) * pageSize
	files := []map[string]interface{}{}
	for i := start; i < start+pageSize && i < f.total; i++ {
		files = append(files, map[string]interface{}{
			"id": fmt.Sprintf("id-%d", i), "name": fmt.Sprintf("file-%d.txt", i),
			"type": "file", "size": float64(1),
		})
	}
	body, _ := json.Marshal(map[string]interface{}{
		"code": 0,
		"data": map[string]interface{}{"total": f.total, "files": files},
	})
	return &HTTPResponse{StatusCode: 200, Body: body}, nil
}

func (f *fakeHTTPClient) UploadMultipart(path, contentType string, body io.Reader) error {
	return nil
}

func TestListFilesTraversesAllPages(t *testing.T) {
	client := &fakeHTTPClient{total: 250}
	p := NewFileProvider(client)
	res, err := p.listFilesByParentID(stdctx.Background(), "x", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Nodes) != 250 {
		t.Fatalf("internal traversal got %d nodes, want all 250", len(res.Nodes))
	}
}

func TestListFilesHonorsOffsetWindow(t *testing.T) {
	client := &fakeHTTPClient{total: 250}
	p := NewFileProvider(client)
	res, err := p.listFilesByParentID(stdctx.Background(), "x", "", &ListOptions{Limit: 50, Offset: 125})
	if err != nil {
		t.Fatal(err)
	}
	if len(client.requests) != 1 || !strings.Contains(client.requests[0], "page=2") {
		t.Fatalf("offset window should request only page 2, requests: %v", client.requests)
	}
	if len(res.Nodes) != 50 {
		t.Fatalf("offset window returned %d nodes, want 50", len(res.Nodes))
	}
	if res.Total != 250 {
		t.Fatalf("offset window reported total %d, want 250", res.Total)
	}
	if res.Nodes[0].Name != "file-125.txt" || res.Nodes[49].Name != "file-174.txt" {
		t.Fatalf("offset window returned %s through %s, want file-125.txt through file-174.txt", res.Nodes[0].Name, res.Nodes[49].Name)
	}
}

func TestListFilesOffsetWindowCrossesPageBoundary(t *testing.T) {
	client := &fakeHTTPClient{total: 250}
	p := NewFileProvider(client)
	res, err := p.listFilesByParentID(stdctx.Background(), "x", "", &ListOptions{Limit: 30, Offset: 90})
	if err != nil {
		t.Fatal(err)
	}
	if len(client.requests) != 2 || !strings.Contains(client.requests[0], "page=1") || !strings.Contains(client.requests[1], "page=2") {
		t.Fatalf("expected pages 1 and 2, got %v", client.requests)
	}
	if len(res.Nodes) != 30 {
		t.Fatalf("got %d nodes, want 30", len(res.Nodes))
	}
	for i, node := range res.Nodes {
		if want := fmt.Sprintf("file-%d.txt", 90+i); node.Name != want {
			t.Fatalf("node %d = %s, want %s", i, node.Name, want)
		}
	}
}

type laterPageErrorClient struct{ fakeHTTPClient }

func (f *laterPageErrorClient) Request(method, path, authKind string, headers map[string]string, body map[string]interface{}) (*HTTPResponse, error) {
	u, _ := url.Parse(path)
	if u.Query().Get("page") == "2" {
		return &HTTPResponse{Body: []byte(`{"code":100,"message":"unavailable"}`)}, nil
	}
	return f.fakeHTTPClient.Request(method, path, authKind, headers, body)
}
func TestListFilesPropagatesLaterPageError(t *testing.T) {
	client := &laterPageErrorClient{fakeHTTPClient{total: 250}}
	p := NewFileProvider(client)
	if _, err := p.listFilesByParentID(stdctx.Background(), "x", "", nil); err == nil {
		t.Fatal("later page error must not become a partial successful listing")
	}
}

type rootFolderClient struct{ fakeHTTPClient }

func (f *rootFolderClient) Request(method, path, authKind string, headers map[string]string, body map[string]interface{}) (*HTTPResponse, error) {
	if path == "/files" {
		return &HTTPResponse{Body: []byte(`{"code":0,"data":{"root_id":"x"}}`)}, nil
	}
	resp, err := f.fakeHTTPClient.Request(method, path, authKind, headers, body)
	if err != nil {
		return nil, err
	}
	var v map[string]interface{}
	json.Unmarshal(resp.Body, &v)
	for _, item := range v["data"].(map[string]interface{})["files"].([]interface{}) {
		m := item.(map[string]interface{})
		if m["id"] == "id-200" {
			m["type"] = "folder"
			m["name"] = "tail-folder"
		}
	}
	resp.Body, _ = json.Marshal(v)
	return resp, nil
}
func TestGetFolderIDByNameFindsFolderPastFirstPage(t *testing.T) {
	p := NewFileProvider(&rootFolderClient{fakeHTTPClient{total: 250}})
	id, err := p.getFolderIDByName(stdctx.Background(), "tail-folder")
	if err != nil || id != "id-200" {
		t.Fatalf("late folder lookup: %q, %v", id, err)
	}
}
