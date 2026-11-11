// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package component

import (
	"archive/zip"
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gorm.io/gorm"

	modelModule "ragflow/internal/entity/models"
	"ragflow/internal/ingestion/component/schema"
)

func TestPlatformMinerUExtractErrorHidesProviderText(t *testing.T) {
	withSSRFBypass(t)
	const secret = "sk-platform-secret"
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.CreateRaw(&zip.FileHeader{Name: secret + "/content_list.json", Method: 99, CompressedSize64: 2, UncompressedSize64: 2})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write([]byte("[]"))
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	zipBytes := buf.Bytes()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/health":
			_, _ = w.Write([]byte(`{}`))
		case "/v1/uploads":
			_, _ = w.Write([]byte(`{"status":"completed","file":{"id":"f"}}`))
		case "/v1/parse/jobs":
			_, _ = w.Write([]byte(`{"job_id":"j","status":"completed","files":[{"status":"completed","output_files":{"zip":{"file_id":"z"}}}]}`))
		case "/v1/files/z/content":
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write(zipBytes)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	original := resolveMinerUModelForDispatch
	defer func() { resolveMinerUModelForDispatch = original }()
	key := "k"
	baseURL := server.URL
	resolveMinerUModelForDispatch = func(context.Context, *gorm.DB, string, string) (modelModule.ModelDriver, string, *modelModule.APIConfig, error) {
		driver := modelModule.NewMinerUModel(map[string]string{"default": baseURL}, modelModule.URLSuffix{})
		return modelModule.NewPlatformSafeDriver(driver), "model", &modelModule.APIConfig{ApiKey: &key, BaseURL: &baseURL}, nil
	}
	_, err = dispatchMinerUPDF(t.Context(), nil, "file.pdf", []byte("pdf"), "tenant", schema.ParserSetup{}, "model")
	if err == nil {
		t.Fatal("expected an error")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("platform MinerU V1 extract error shows provider text: %v", err)
	}
}

func TestPlatformMinerUConfigErrorHidesStoredSettings(t *testing.T) {
	const stored = `{"mineru_apiserver":"https://mineru.internal","mineru_backend":"vlm-sglang-internal-cluster-7"}`
	err := modelModule.ValidateMinerUConfigForAPI(modelModule.ResolveMinerUBackend("", stored), "", false)
	if err == nil {
		t.Fatal("expected invalid backend")
	}
	if got := hidePlatformConfigError(true, "parser: MinerU", err).Error(); strings.Contains(got, "internal-cluster-7") {
		t.Fatalf("stored platform setting shown: %q", got)
	}
	// A tenant's own error stays as it was, without an extra prefix.
	if got := hidePlatformConfigError(false, "parser: MinerU", err); got != err {
		t.Fatalf("tenant error changed: %v", got)
	}
	_, extractErr := finishMinerURemoteResult(context.Background(), "a.pdf", nil, &modelModule.MinerUV1Result{})
	if got := hidePlatformConfigError(false, "parser: MinerU", extractErr).Error(); strings.Count(got, "parser: MinerU") != 1 {
		t.Fatalf("tenant extract error = %q", got)
	}
}
