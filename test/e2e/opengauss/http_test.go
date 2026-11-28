//go:build e2e

package opengauss_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	_ "gitcode.com/opengauss/openGauss-connector-go-pq"
)

type object = map[string]any
type suite struct {
	base, owner, guest, model, tenant, prefix string
	client                                    *http.Client
	db                                        *sql.DB
}
type result struct {
	Code    *int            `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func required(t *testing.T, key string) string {
	t.Helper()
	value := os.Getenv(key)
	if value == "" {
		t.Fatalf("set %s", key)
	}
	return value
}

func TestOpenGaussHTTP(t *testing.T) {
	base := os.Getenv("RAGFLOW_E2E_URL")
	if base == "" {
		t.Skip("set RAGFLOW_E2E_URL to run against a deployed server")
	}
	db, err := sql.Open("opengauss", required(t, "RAGFLOW_E2E_OPENGAUSS_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	random := make([]byte, 8)
	if _, err := rand.Read(random); err != nil {
		t.Fatal(err)
	}
	s := suite{
		base:   strings.TrimRight(base, "/") + "/api/v1",
		owner:  required(t, "RAGFLOW_E2E_OWNER_AUTH"),
		guest:  required(t, "RAGFLOW_E2E_GUEST_AUTH"),
		model:  required(t, "RAGFLOW_E2E_EMBEDDING_MODEL"),
		prefix: "og-e2e-" + hex.EncodeToString(random),
		client: &http.Client{Timeout: 90 * time.Second}, db: db,
	}
	s.tenant = field(t, s.call(t, "GET", "/users/me", nil), "id")
	other := s
	other.owner = s.guest
	if field(t, other.call(t, "GET", "/users/me", nil), "id") == s.tenant {
		t.Fatal("two independent user accounts required")
	}
	var version, mode, encoding string
	if err := db.QueryRowContext(t.Context(), "SELECT version(),datcompatibility,getdatabaseencoding() FROM pg_database WHERE datname=current_database()").Scan(&version, &mode, &encoding); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(version, "openGauss") || mode != "A" || encoding != "UTF8" {
		t.Fatalf("unexpected database %s %s %s", version, mode, encoding)
	}
	t.Run("document_and_chunk_lifecycle", s.chunks)
	t.Run("upload_parse_and_retrieve", s.ingest)
	t.Run("skill_indexes_and_ranking", s.skills)
	t.Run("raw_memory_lifecycle", s.memory)
	t.Run("concurrent_chunk_writes", s.concurrentWrites)
}

func (s suite) request(t *testing.T, method, path, auth, contentType string, body io.Reader) (int, result) {
	t.Helper()
	req, err := http.NewRequest(method, s.base+path, body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", auth)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	var out result
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil || out.Code == nil {
		t.Fatalf("%s %s: HTTP %d, invalid API envelope: %v", method, path, resp.StatusCode, err)
	}
	return resp.StatusCode, out
}
func (s suite) call(t *testing.T, method, path string, body any) json.RawMessage {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	status, res := s.request(t, method, path, s.owner, "application/json", &buf)
	if status/100 != 2 || *res.Code != 0 {
		t.Fatalf("%s %s: HTTP %d code %d: %s", method, path, status, *res.Code, res.Message)
	}
	return res.Data
}
func decode[T any](t *testing.T, b json.RawMessage) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("decode %.400s: %v", b, err)
	}
	return v
}
func field(t *testing.T, b json.RawMessage, key string) string {
	t.Helper()
	v, _ := decode[object](t, b)[key].(string)
	if v == "" {
		t.Fatalf("missing %s: %.400s", key, b)
	}
	return v
}
func (s suite) dataset(t *testing.T, suffix string) string {
	t.Helper()
	id := s.createDataset(t, suffix)
	t.Cleanup(func() { s.call(t, "DELETE", "/datasets", object{"ids": []string{id}}) })
	return id
}
func (s suite) createDataset(t *testing.T, suffix string) string {
	t.Helper()
	return field(t, s.call(t, "POST", "/datasets", object{"name": s.prefix + suffix, "embedding_model": s.model, "parse_type": 1, "parser_id": "naive", "permission": "me"}), "id")
}
func (s suite) upload(t *testing.T, kb, name, content string) string {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("file", name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = io.WriteString(part, content); err != nil {
		t.Fatal(err)
	}
	if err = w.Close(); err != nil {
		t.Fatal(err)
	}
	status, res := s.request(t, "POST", "/datasets/"+kb+"/documents", s.owner, w.FormDataContentType(), &buf)
	if status/100 != 2 || *res.Code != 0 {
		t.Fatalf("upload: HTTP %d code %d: %s", status, *res.Code, res.Message)
	}
	rows := decode[[]json.RawMessage](t, res.Data)
	if len(rows) != 1 {
		t.Fatalf("upload returned %d documents", len(rows))
	}
	return field(t, rows[0], "id")
}
func (s suite) search(t *testing.T, kbs []string, question string, weight float64, extra object) []object {
	t.Helper()
	q := object{"dataset_ids": kbs, "question": question, "vector_similarity_weight": weight, "similarity_threshold": 0, "page_size": 20}
	for k, v := range extra {
		q[k] = v
	}
	out := decode[struct {
		Chunks []object `json:"chunks"`
	}](t, s.call(t, "POST", "/retrieval", q))
	return out.Chunks
}
func hasHit(hits []object, id, kb string) bool {
	for _, h := range hits {
		if (h["chunk_id"] == id || h["id"] == id) && (h["kb_id"] == kb || h["dataset_id"] == kb) {
			return true
		}
	}
	return false
}
func waitFor(t *testing.T, description string, probe func() (bool, string)) {
	t.Helper()
	deadline := time.Now().Add(150 * time.Second)
	for {
		ok, state := probe()
		if ok {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s: timeout, last state %s", description, state)
		}
		time.Sleep(700 * time.Millisecond)
	}
}

// chunkCount independently checks rows in the exact openGauss database and schema
// used by the server. Catalog lookup is read-only; HTTP owns all fixture writes.
func (s suite) chunkCount(t *testing.T, kb, id string) int {
	t.Helper()
	table := "ragflow_" + s.tenant
	var n int
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	sqlText := fmt.Sprintf("SELECT count(*) FROM %s WHERE kb_id=$1 AND id=$2", `"`+strings.ReplaceAll(table, `"`, `""`)+`"`)
	if err := s.db.QueryRowContext(ctx, sqlText, kb, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
