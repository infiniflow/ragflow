//go:build integration

package api

import (
	"net/http"
	"strings"
	"testing"
)

func stringField(t *testing.T, m map[string]any, key string) string {
	t.Helper()
	value, ok := m[key].(string)
	if !ok || value == "" {
		t.Fatalf("field %q is not a non-empty string: %v", key, m)
	}
	return value
}

func deleteFileOnCleanup(t *testing.T, id string) {
	t.Helper()
	t.Cleanup(func() {
		r := restCall(t, TestConfig, http.MethodDelete, "/files", apiObject{"ids": []string{id}}, nil)
		code, _ := r.JSON["code"].(float64)
		if r.Status != http.StatusOK || (code != 0 && code != 102) {
			t.Errorf("delete file %s cleanup failed: HTTP %d code=%v", id, r.Status, r.JSON["code"])
		}
	})
}

func createFileFolder(t *testing.T, parentID, name string) map[string]any {
	t.Helper()
	body := apiObject{"name": name, "type": "folder"}
	if parentID != "" {
		body["parent_id"] = parentID
	}
	r := restCall(t, TestConfig, http.MethodPost, "/files", body, nil)
	folder := object(t, restOK(t, r)["data"])
	deleteFileOnCleanup(t, stringField(t, folder, "id"))
	return folder
}

func uploadFileToRoot(t *testing.T, filename, content string) map[string]any {
	t.Helper()
	r := restUpload(t, TestConfig, http.MethodPost, "/files", "file", filename, []byte(content), nil)
	items := array(t, restOK(t, r)["data"])
	if len(items) == 0 {
		t.Fatalf("file upload returned no items: %v", r.JSON)
	}
	uploaded := object(t, items[0])
	deleteFileOnCleanup(t, stringField(t, uploaded, "id"))
	return uploaded
}

func TestFilesListCreateFolderAndUploadToRoot(t *testing.T) {
	list := restCall(t, TestConfig, http.MethodGet, "/files", nil, nil)
	data := object(t, restOK(t, list)["data"])
	array(t, data["files"])
	if _, ok := data["total"]; !ok {
		t.Fatalf("files listing has no total: %v", data)
	}

	folderName := uniqueName("rf_folder")
	folder := createFileFolder(t, "", folderName)
	if folder["name"] != folderName {
		t.Fatalf("folder name = %v, want %q", folder["name"], folderName)
	}

	uploadName := uniqueName("rf_upload") + ".txt"
	uploaded := uploadFileToRoot(t, uploadName, "ragflow files upload")
	if uploaded["name"] != uploadName {
		t.Fatalf("uploaded file name = %v, want %q", uploaded["name"], uploadName)
	}
}

func TestFilesFolderCreateFallbackAndRenameDuplicate(t *testing.T) {
	folderName := uniqueName("rf_dup_folder")
	first := createFileFolder(t, "", folderName)
	if first["name"] != folderName {
		t.Fatalf("first folder name = %v, want %q", first["name"], folderName)
	}

	second := createFileFolder(t, "", folderName)
	wantFallback := folderName + "(1)"
	if second["name"] != wantFallback {
		t.Fatalf("duplicate folder name = %v, want %q", second["name"], wantFallback)
	}

	secondID := stringField(t, second, "id")
	duplicate := restCall(t, TestConfig, http.MethodPost, "/files/move", apiObject{
		"src_file_ids": []string{secondID},
		"new_name":     folderName,
	}, nil)
	restCode(t, duplicate, http.StatusOK, 400)
	if message, _ := duplicate.JSON["message"].(string); !strings.Contains(message, "duplicated file name") {
		t.Fatalf("duplicate rename message = %v, want it to contain %q", duplicate.JSON["message"], "duplicated file name")
	}

	caseOnly := restCall(t, TestConfig, http.MethodPost, "/files/move", apiObject{
		"src_file_ids": []string{secondID},
		"new_name":     strings.ToUpper(stringField(t, second, "name")),
	}, nil)
	if restOK(t, caseOnly)["data"] != true {
		t.Fatalf("case-only rename data = %v, want true", caseOnly.JSON["data"])
	}
}

func TestFilesGetParentAndAncestors(t *testing.T) {
	a := createFileFolder(t, "", uniqueName("rf_parent_a"))
	b := createFileFolder(t, stringField(t, a, "id"), uniqueName("rf_child_b"))
	c := createFileFolder(t, stringField(t, b, "id"), uniqueName("rf_child_c"))
	cID := stringField(t, c, "id")

	parent := restCall(t, TestConfig, http.MethodGet, "/files/"+cID+"/parent", nil, nil)
	parentData := object(t, restOK(t, parent)["data"])
	parentFolder := object(t, parentData["parent_folder"])
	if parentFolder["id"] != b["id"] {
		t.Fatalf("parent folder id = %v, want %v", parentFolder["id"], b["id"])
	}

	ancestors := restCall(t, TestConfig, http.MethodGet, "/files/"+cID+"/ancestors", nil, nil)
	ancestorData := object(t, restOK(t, ancestors)["data"])
	folders := array(t, ancestorData["parent_folders"])
	// The contract is the node followed by every ancestor up to the root.
	want := []string{
		cID,
		stringField(t, b, "id"),
		stringField(t, a, "id"),
		stringField(t, a, "parent_id"),
	}
	got := make([]string, 0, len(folders))
	for _, item := range folders {
		got = append(got, stringField(t, object(t, item), "id"))
	}
	if len(got) != len(want) {
		t.Fatalf("ancestors = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ancestors = %v, want %v", got, want)
		}
	}
}

func TestFilesDownloadReturnsContent(t *testing.T) {
	const content = "ragflow files download"
	fileName := uniqueName("rf_download") + ".txt"
	uploaded := uploadFileToRoot(t, fileName, content)

	download := restCall(t, TestConfig, http.MethodGet, "/files/"+stringField(t, uploaded, "id"), nil, nil)
	if download.Status != http.StatusOK {
		t.Fatalf("download status = %d, want %d; body: %s", download.Status, http.StatusOK, download.Body)
	}
	if string(download.Body) != content {
		t.Fatalf("downloaded body = %q, want %q", download.Body, content)
	}
	if contentType := download.Header.Get("Content-Type"); !strings.HasPrefix(contentType, "text/plain") {
		t.Fatalf("download content type = %q, want text/plain", contentType)
	}
	if disposition := download.Header.Get("Content-Disposition"); !strings.Contains(disposition, fileName) {
		t.Fatalf("download content disposition = %q, want it to contain %q", disposition, fileName)
	}
}

func TestFilesLinkToDatasetsContract(t *testing.T) {
	t.Run("missing keys", func(t *testing.T) {
		r := restCall(t, TestConfig, http.MethodPost, "/files/link-to-datasets", apiObject{}, nil)
		restCode(t, r, http.StatusOK, 101)
		if message, _ := r.JSON["message"].(string); !strings.Contains(message, "required argument are missing") {
			t.Fatalf("message = %v, want it to contain %q", r.JSON["message"], "required argument are missing")
		}
	})

	t.Run("invalid mode", func(t *testing.T) {
		r := restCall(t, TestConfig, http.MethodPost, "/files/link-to-datasets", apiObject{
			"file_ids": []string{},
			"kb_ids":   []string{},
		}, apiObject{"mode": "bogus"})
		restCode(t, r, http.StatusOK, 101)
		if r.JSON["message"] != "mode must be 'add' or 'replace'" {
			t.Fatalf("message = %v, want %q", r.JSON["message"], "mode must be 'add' or 'replace'")
		}
	})

	t.Run("missing file", func(t *testing.T) {
		r := restCall(t, TestConfig, http.MethodPost, "/files/link-to-datasets", apiObject{
			"file_ids": []string{TestConfig.InvalidID},
			"kb_ids":   []string{},
		}, nil)
		restCode(t, r, http.StatusOK, 102)
	})

	uploaded := uploadFileToRoot(t, uniqueName("rf_link")+".txt", "ragflow link upload")
	fileID := stringField(t, uploaded, "id")

	t.Run("missing dataset", func(t *testing.T) {
		r := restCall(t, TestConfig, http.MethodPost, "/files/link-to-datasets", apiObject{
			"file_ids": []string{fileID},
			"kb_ids":   []string{TestConfig.InvalidID},
		}, nil)
		restCode(t, r, http.StatusOK, 102)
	})

	t.Run("success", func(t *testing.T) {
		r := restCall(t, TestConfig, http.MethodPost, "/files/link-to-datasets", apiObject{
			"file_ids": []string{fileID},
			"kb_ids":   []string{},
		}, nil)
		if restOK(t, r)["data"] != true {
			t.Fatalf("link data = %v, want true", r.JSON["data"])
		}
	})
}
