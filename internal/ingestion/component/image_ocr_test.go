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
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/common"
	deepdocpdf "ragflow/internal/deepdoc/parser/pdf"
	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
	"ragflow/internal/parser/parser"
	"ragflow/internal/utility"
)

type imageOCRRequest struct {
	path, auth, model, filename string
	fileType                    int
	content                     []byte
}

type imageOCRRequests struct {
	mu    sync.Mutex
	items []imageOCRRequest
}

// append records a fixture request without racing the HTTP handler.
func (r *imageOCRRequests) append(request imageOCRRequest) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items = append(r.items, request)
}

// list copies the recorded request slice under lock. Content byte slices remain
// shared with the stored requests and must be treated as read-only.
func (r *imageOCRRequests) list() []imageOCRRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]imageOCRRequest(nil), r.items...)
}

// imageOCRFixture replaces only the OCR HTTP service. Tenant model resolution,
// instance configuration, image dispatch and driver calls use production code.
// It seeds instance B alongside decoy instance A, whose requests fail the test.
// Its process-wide SSRF bypass is restored at cleanup, so callers must not run
// in parallel and SSRF-specific tests must explicitly re-enable the guard.
func imageOCRFixture(t *testing.T, provider string, fail bool) (*gorm.DB, *imageOCRRequests) {
	t.Helper()
	withSSRFBypass(t)
	requests := &imageOCRRequests{}
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req := imageOCRRequest{path: r.URL.Path, auth: r.Header.Get("Authorization")}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/layout-parsing":
			var body struct {
				File     string `json:"file"`
				FileType int    `json:"fileType"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			req.fileType = body.FileType
			req.content, _ = base64.StdEncoding.DecodeString(body.File)
			requests.append(req)
			if fail {
				http.Error(w, "fixture OCR unavailable", http.StatusServiceUnavailable)
				return
			}
			_, _ = io.WriteString(w, `{"errorCode":0,"result":{"layoutParsingResults":[{"markdown":{"text":"Selected instance B."}}]}}`)
		case "/v2/ocr/jobs":
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Error(err)
				http.Error(w, "invalid fixture upload", http.StatusBadRequest)
				return
			}
			defer r.MultipartForm.RemoveAll()
			file, header, err := r.FormFile("file")
			if err != nil {
				t.Error(err)
				return
			}
			defer file.Close()
			req.content, _ = io.ReadAll(file)
			req.filename = header.Filename
			req.model = r.FormValue("model")
			requests.append(req)
			if fail {
				http.Error(w, "fixture OCR unavailable", http.StatusServiceUnavailable)
				return
			}
			_, _ = io.WriteString(w, `{"data":{"jobId":"image-job"}}`)
		case "/v2/ocr/jobs/image-job":
			requests.append(req)
			_, _ = fmt.Fprintf(w, `{"data":{"state":"done","resultUrl":{"jsonUrl":%q}}}`, server.URL+"/image-result")
		case "/image-result":
			requests.append(req)
			_, _ = io.WriteString(w, `[{"result":{"layoutParsingResults":[{"markdown":{"text":"Selected instance B."}}]}}]`)
		default:
			t.Errorf("unexpected OCR path: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	decoy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unselected instance was called: %s", r.URL.Path)
		http.Error(w, "wrong instance", http.StatusBadRequest)
	}))
	t.Cleanup(decoy.Close)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&entity.TenantModelProvider{}, &entity.TenantModelInstance{}, &entity.TenantModel{}); err != nil {
		t.Fatal(err)
	}
	rows := []any{&entity.TenantModelProvider{ID: "provider", TenantID: "tenant-1", ProviderName: provider}}
	for _, instance := range []struct{ id, url string }{{"a", decoy.URL}, {"b", server.URL}} {
		rows = append(rows,
			&entity.TenantModelInstance{ID: "instance-" + instance.id, ProviderID: "provider", InstanceName: instance.id,
				APIKey: "fixture-key-" + instance.id, Status: "active", Extra: fmt.Sprintf(`{"base_url":%q}`, instance.url)},
			&entity.TenantModel{ID: "model-" + instance.id, ProviderID: "provider", InstanceID: "instance-" + instance.id,
				ModelName: "PaddleOCR-VL-1.5", ModelType: int(entity.ModelTypeOCR), Status: "active"})
	}
	for _, row := range rows {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	return db, requests
}

// TestImageOCRSelectedTenantModel checks the selected instance, credentials,
// image bytes and upload protocol for both PaddleOCR providers.
func TestImageOCRSelectedTenantModel(t *testing.T) {
	for _, provider := range []string{"PaddleOCR.local", "PaddleOCR"} {
		t.Run(provider, func(t *testing.T) {
			db, requests := imageOCRFixture(t, provider, false)
			data := picturePNG(t)
			setups := imageSwitchSetups(t, true)
			setups["image"]["ocr_model_id"] = "model-b"
			res, handled, err := maybeDispatchImage(t.Context(), db, utility.FileTypeVISUAL,
				"invoice.png", data, map[string]any{"tenant_id": "tenant-1"}, setups, false, visionSettings{})
			if err != nil || !handled {
				t.Fatalf("handled=%v error=%v", handled, err)
			}
			if got := res.JSON[0].ToMap()["text"]; strings.TrimSpace(fmt.Sprint(got)) != "Selected instance B." {
				t.Fatalf("text=%q warnings=%v", got, res.Warnings)
			}
			if len(res.Warnings) != 0 {
				t.Fatalf("warnings=%v", res.Warnings)
			}
			image, _ := res.JSON[0].ToMap()["image"].(string)
			if !strings.HasSuffix(image, base64.StdEncoding.EncodeToString(data)) {
				t.Fatal("image attachment was lost or changed")
			}
			if len(requests.list()) == 0 {
				t.Fatal("selected OCR driver was never called")
			}
			req := requests.list()[0]
			if req.auth != "Bearer fixture-key-b" || !bytes.Equal(req.content, data) {
				t.Fatalf("selected instance credentials or image bytes were not used: %+v", req)
			}
			if provider == "PaddleOCR.local" && req.fileType != 1 {
				t.Fatalf("local fileType=%d, want image type 1", req.fileType)
			}
			if provider == "PaddleOCR" && (req.filename != "document.png" || req.model != "PaddleOCR-VL-1.5") {
				t.Fatalf("cloud image upload=%+v", req)
			}
		})
	}
}

// TestImageOCRReleasesMediaAdmissionBeforeRequest verifies that selected OCR
// never holds image materialization slots during remote I/O, including failures.
func TestImageOCRReleasesMediaAdmissionBeforeRequest(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprint(fail), func(t *testing.T) {
			db, _ := imageOCRFixture(t, "PaddleOCR.local", false)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ctx, cancel := context.WithTimeout(r.Context(), time.Second)
				defer cancel()
				for range deepdocpdf.DeepDocConcurrency() {
					release, err := parser.AcquireImageMedia(ctx)
					if err != nil {
						t.Errorf("selected OCR retained an image-media slot during I/O: %v", err)
						http.Error(w, "image-media admission blocked", http.StatusServiceUnavailable)
						return
					}
					defer release()
				}
				if fail {
					http.Error(w, "fixture OCR unavailable", http.StatusServiceUnavailable)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"errorCode":0,"result":{"layoutParsingResults":[{"markdown":{"text":"Remote OCR text."}}]}}`)
			}))
			t.Cleanup(server.Close)
			if err := db.Model(&entity.TenantModelInstance{}).Where("id = ?", "instance-b").
				Update("extra", fmt.Sprintf(`{"base_url":%q}`, server.URL)).Error; err != nil {
				t.Fatal(err)
			}
			setups := imageSwitchSetups(t, true)
			setups["image"]["ocr_model_id"] = "model-b"
			result, _, err := maybeDispatchImage(t.Context(), db, utility.FileTypeVISUAL,
				"invoice.png", picturePNG(t), map[string]any{"tenant_id": "tenant-1"}, setups, false, visionSettings{})
			if err != nil || len(result.JSON) != 1 {
				t.Fatalf("err=%v result=%+v", err, result)
			}
			item := result.JSON[0].ToMap()
			if fail {
				if item["text"] != "" || !strings.Contains(strings.Join(result.Warnings, "\n"), "fixture OCR unavailable") {
					t.Fatalf("failure result=%+v warnings=%v", item, result.Warnings)
				}
			} else if item["text"] != "Remote OCR text." || len(result.Warnings) != 0 {
				t.Fatalf("success result=%+v warnings=%v", item, result.Warnings)
			}
			if item["image"] == "" || item["image"] == nil {
				t.Fatal("remote OCR lost the image attachment")
			}
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			for range deepdocpdf.DeepDocConcurrency() {
				release, err := parser.AcquireImageMedia(ctx)
				if err != nil {
					t.Fatalf("image-media admission leaked after dispatch: %v", err)
				}
				defer release()
			}
		})
	}
}

// TestImageOCRFailureRetainsAttachment verifies that failures remain visible
// without dropping the image or substituting local OCR.
func TestImageOCRFailureRetainsAttachment(t *testing.T) {
	for _, provider := range []string{"PaddleOCR.local", "PaddleOCR"} {
		t.Run(provider, func(t *testing.T) {
			db, requests := imageOCRFixture(t, provider, true)
			setups := imageSwitchSetups(t, true)
			setups["image"]["ocr_model_id"] = "model-b"
			res, _, err := maybeDispatchImage(t.Context(), db, utility.FileTypeVISUAL,
				"invoice.png", picturePNG(t), map[string]any{"tenant_id": "tenant-1"}, setups, false, visionSettings{})
			if err != nil {
				t.Fatal(err)
			}
			if len(requests.list()) != 1 || !strings.Contains(strings.Join(res.Warnings, "\n"), "fixture OCR unavailable") {
				t.Fatalf("OCR failure was not reported: requests=%v warnings=%v", requests.list(), res.Warnings)
			}
			item := res.JSON[0].ToMap()
			if item["text"] != "" || item["image"] == "" || item["image"] == nil {
				t.Fatalf("failure must retain the attachment, not substitute local OCR: %+v", item)
			}
		})
	}
}

// TestImageOCRInvalidSelectionNeverFallsBack rejects inaccessible or invalid
// model/instance records before making an OCR request.
func TestImageOCRInvalidSelectionNeverFallsBack(t *testing.T) {
	cases := []struct {
		name, tenant, ref, table, field string
		value                           any
		want                            string
	}{
		{name: "missing tenant", ref: "model-b", want: "tenant"},
		{name: "foreign tenant", tenant: "tenant-2", ref: "model-b", want: "not found"},
		{name: "missing model", tenant: "tenant-1", ref: "missing", want: "not found"},
		{name: "disabled model", tenant: "tenant-1", ref: "model-b", table: "tenant_model", field: "status", value: "inactive", want: "disabled"},
		{name: "wrong type", tenant: "tenant-1", ref: "model-b", table: "tenant_model", field: "model_type", value: int(entity.ModelTypeChat), want: "ocr"},
		{name: "disabled instance", tenant: "tenant-1", ref: "model-b", table: "tenant_model_instance", field: "status", value: "inactive", want: "disabled"},
		{name: "mismatched instance provider", tenant: "tenant-1", ref: "model-b", table: "tenant_model_instance", field: "provider_id", value: "other", want: "provider"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, requests := imageOCRFixture(t, "PaddleOCR.local", false)
			if tc.table != "" {
				id := "model-b"
				if tc.table == "tenant_model_instance" {
					id = "instance-b"
				}
				if err := db.Table(tc.table).Where("id = ?", id).Update(tc.field, tc.value).Error; err != nil {
					t.Fatal(err)
				}
			}
			setups := imageSwitchSetups(t, true)
			setups["image"]["ocr_model_id"] = tc.ref
			res, _, err := maybeDispatchImage(t.Context(), db, utility.FileTypeVISUAL,
				"invoice.png", picturePNG(t), map[string]any{"tenant_id": tc.tenant}, setups, false, visionSettings{})
			if err != nil {
				t.Fatal(err)
			}
			if len(requests.list()) != 0 || !strings.Contains(strings.ToLower(strings.Join(res.Warnings, "\n")), tc.want) {
				t.Fatalf("selection was not rejected: requests=%v warnings=%v", requests.list(), res.Warnings)
			}
			if res.JSON[0].ToMap()["text"] != "" {
				t.Fatal("invalid selection silently fell back to another OCR source")
			}
		})
	}
}

// TestImageOCRSelectionRespectsSwitchAndSVG keeps disabled OCR and native SVG
// text extraction independent of a saved model selection.
func TestImageOCRSelectionRespectsSwitchAndSVG(t *testing.T) {
	db, requests := imageOCRFixture(t, "PaddleOCR.local", false)
	for _, tc := range []struct {
		name, filename, text string
		ocr                  bool
		data                 []byte
	}{
		{"off", "invoice.png", "", false, picturePNG(t)},
		{"svg", "invoice.svg", "SVG invoice", true, []byte(`<svg xmlns="http://www.w3.org/2000/svg"><text>SVG invoice</text></svg>`)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setups := imageSwitchSetups(t, tc.ocr)
			setups["image"]["ocr_model_id"] = "model-b"
			res, _, err := maybeDispatchImage(t.Context(), db, utility.FileTypeVISUAL,
				tc.filename, tc.data, map[string]any{"tenant_id": "tenant-1"}, setups, false, visionSettings{})
			if err != nil || len(res.JSON) != 1 {
				t.Fatalf("err=%v result=%+v", err, res)
			}
			if res.JSON[0].ToMap()["text"] != tc.text || len(requests.list()) != 0 {
				t.Fatalf("switch or SVG bypass broken: %+v requests=%v", res, requests.list())
			}
		})
	}
}

// TestImageOCRModelConfigValidation rejects non-string model IDs at setup time.
func TestImageOCRModelConfigValidation(t *testing.T) {
	_, err := NewParserComponent(map[string]any{"image": map[string]any{"ocr_enabled": true, "ocr_model_id": 123}})
	if err == nil || !strings.Contains(err.Error(), "ocr_model_id") {
		t.Fatalf("non-string model id must be rejected, got %v", err)
	}
}

// TestImageOCRDoesNotUseGlobalEndpoint prevents an incomplete selected instance
// from silently borrowing a process-global server address.
func TestImageOCRDoesNotUseGlobalEndpoint(t *testing.T) {
	db, requests := imageOCRFixture(t, "PaddleOCR.local", false)
	t.Setenv("PADDLEOCR_BASE_URL", "http://global.invalid")
	t.Setenv("PADDLEOCR_API_URL", "http://global.invalid")
	if err := db.Model(&entity.TenantModelInstance{}).Where("id = ?", "instance-b").Update("extra", "{}").Error; err != nil {
		t.Fatal(err)
	}
	_, err := extractSelectedImageOCR(t.Context(), db, "tenant-1", "model-b", picturePNG(t))
	if err == nil || !strings.Contains(err.Error(), "no base URL") || len(requests.list()) != 0 {
		t.Fatalf("selected local model must not borrow a global endpoint: %v", err)
	}
}

// TestImageOCRKeepsCloudSSRFGuard preserves public-address validation for cloud
// OCR requests even when the model was explicitly selected.
func TestImageOCRKeepsCloudSSRFGuard(t *testing.T) {
	db, requests := imageOCRFixture(t, "PaddleOCR", false)
	common.AllowAnyHostForTest = false
	_, err := extractSelectedImageOCR(t.Context(), db, "tenant-1", "model-b", picturePNG(t))
	if err == nil || !strings.Contains(err.Error(), "non-public address") || len(requests.list()) != 0 {
		t.Fatalf("cloud transport policy changed: err=%v requests=%v", err, requests.list())
	}
}

// TestImageOCRRejectsOtherOCRDrivers restricts this selector to PaddleOCR.
func TestImageOCRRejectsOtherOCRDrivers(t *testing.T) {
	db, requests := imageOCRFixture(t, "MinerU", false)
	_, err := extractSelectedImageOCR(t.Context(), db, "tenant-1", "model-b", picturePNG(t))
	if err == nil || !strings.Contains(err.Error(), "requires a PaddleOCR") || len(requests.list()) != 0 {
		t.Fatalf("unsupported driver accepted: err=%v requests=%v", err, requests.list())
	}
}

// TestImageOCRReadsInstanceJSONAndRotatedCredentials verifies that each request
// reads the selected instance's current structured credentials.
func TestImageOCRReadsInstanceJSONAndRotatedCredentials(t *testing.T) {
	db, requests := imageOCRFixture(t, "PaddleOCR.local", false)
	var instance entity.TenantModelInstance
	if err := db.Where("id = ?", "instance-b").Take(&instance).Error; err != nil {
		t.Fatal(err)
	}
	var extra map[string]string
	if err := json.Unmarshal([]byte(instance.Extra), &extra); err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{"first-key", "rotated-key"} {
		key := fmt.Sprintf(`{"api_key":{"PADDLEOCR_BASE_URL":%q,"PADDLEOCR_ACCESS_TOKEN":%q}}`, extra["base_url"], token)
		if err := db.Model(&instance).Updates(map[string]any{"api_key": key, "extra": "{}"}).Error; err != nil {
			t.Fatal(err)
		}
		text, err := extractSelectedImageOCR(t.Context(), db, "tenant-1", "model-b", picturePNG(t))
		got := requests.list()
		if err != nil || text != "Selected instance B." || got[len(got)-1].auth != "Bearer "+token {
			t.Fatalf("instance JSON or rotation lost: text=%q err=%v requests=%v", text, err, got)
		}
	}
}

// TestImageOCRAndVisionRemainIndependent checks combined Parser output with
// selected OCR enabled and disabled while global vision stays enabled.
func TestImageOCRAndVisionRemainIndependent(t *testing.T) {
	original := resolveTenantModelByType
	t.Cleanup(func() { resolveTenantModelByType = original })
	resolveTenantModelByType = func(_ context.Context, _ *gorm.DB, _ string, kind entity.ModelType) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
		if kind != entity.ModelTypeImage2Text {
			t.Fatalf("unexpected default model lookup: %v", kind)
		}
		return &imagePromptCaptureDriver{}, "vision", &modelModule.APIConfig{}, 0, nil
	}
	for _, enabled := range []bool{true, false} {
		t.Run(fmt.Sprint(enabled), func(t *testing.T) {
			db, requests := imageOCRFixture(t, "PaddleOCR.local", false)
			params := map[string]any{"enable_vision_enhancement": true,
				"image": map[string]any{"ocr_enabled": enabled, "ocr_model_id": "model-b"}}
			c, err := NewParserComponent(params)
			if err != nil {
				t.Fatal(err)
			}
			out, err := c.Invoke(t.Context(), db, map[string]any{
				"name": "invoice.png", "binary": picturePNG(t), "tenant_id": "tenant-1"})
			if err != nil {
				t.Fatal(err)
			}
			text := out["json"].([]parser.DocItem)[0].ToMap()["text"]
			want := "captured"
			calls := 0
			if enabled {
				want = "Selected instance B.\ncaptured"
				calls = 1
			}
			if text != want || len(requests.list()) != calls {
				t.Fatalf("OCR/vision coupling changed: text=%v requests=%v", text, requests.list())
			}
		})
	}
}

// TestImageOCRFailureReachesProgressMessages checks the actual Parser progress
// callback, not just the dispatch helper's returned warnings.
func TestImageOCRFailureReachesProgressMessages(t *testing.T) {
	db, _ := imageOCRFixture(t, "PaddleOCR.local", true)
	c, err := NewParserComponent(map[string]any{
		"image": map[string]any{"ocr_enabled": true, "ocr_model_id": "model-b"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var messages []string
	ctx := runtime.WithProgressMessageCallback(t.Context(), func(_ string, message string) {
		messages = append(messages, message)
	})
	out, err := c.Invoke(ctx, db, map[string]any{
		"name": "invoice.png", "binary": picturePNG(t), "tenant_id": "tenant-1"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(messages, "\n"), "WARNING: image OCR unavailable") ||
		!strings.Contains(strings.Join(messages, "\n"), "fixture OCR unavailable") {
		t.Fatalf("failure absent from actual Parser progress: %q", messages)
	}
	if item := out["json"].([]parser.DocItem)[0].ToMap(); item["image"] == "" || item["text"] != "" {
		t.Fatalf("failure lost attachment or substituted text: %+v", item)
	}
}
