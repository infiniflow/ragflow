//go:build integration

package api

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"mime"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

func documentDownloadFixtures(t *testing.T) []documentUpload {
	t.Helper()
	var imageBytes bytes.Buffer
	picture := image.NewRGBA(image.Rect(0, 0, 2, 2))
	picture.Set(0, 0, color.RGBA{R: 255, A: 255})
	if err := png.Encode(&imageBytes, picture); err != nil {
		t.Fatal(err)
	}
	pdf, err := os.ReadFile("../benchmark/test_docs/Doc1.pdf")
	if err != nil {
		t.Fatal(err)
	}
	files := []documentUpload{
		{Name: "plain.txt", Content: []byte("Download integrity: 中文\r\nsecond line\x00end")},
		{Name: "notes.md", Content: []byte("# Download\n\n**unaltered** content\n")},
		{Name: "record.json", Content: []byte("{\"code\":102,\"message\":\"this is uploaded content, not an API error\"}\n")},
		{Name: "message.eml", Content: []byte("From: a@example.com\r\nTo: b@example.com\r\nSubject: integrity\r\n\r\nDownload bytes.\r\n")},
		{Name: "page.html", Content: []byte("<!doctype html><html><body>Download integrity</body></html>")},
		{Name: "pixels.png", Content: imageBytes.Bytes()},
		{Name: "source.pdf", Content: pdf},
	}
	for _, office := range []struct{ name, member, contentType, xml string }{
		{"text.docx", "word/document.xml", "application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml", `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>Download integrity</w:t></w:r></w:p></w:body></w:document>`},
		{"cells.xlsx", "xl/workbook.xml", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml", `<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheets/></workbook>`},
		{"slides.pptx", "ppt/presentation.xml", "application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml", `<p:presentation xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"><p:sldIdLst/></p:presentation>`},
	} {
		var content bytes.Buffer
		writer := zip.NewWriter(&content)
		for _, part := range []struct{ name, value string }{
			{"[Content_Types].xml", `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Override PartName="/` + office.member + `" ContentType="` + office.contentType + `"/></Types>`},
			{"_rels/.rels", `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="r1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="` + office.member + `"/></Relationships>`},
			{office.member, office.xml},
		} {
			member, err := writer.Create(part.name)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := member.Write([]byte(part.value)); err != nil {
				t.Fatal(err)
			}
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		files = append(files, documentUpload{Name: office.name, Content: content.Bytes()})
	}
	return files
}

func requireDocumentDownload(t *testing.T, result restResult, file documentUpload) {
	t.Helper()
	if result.Status != http.StatusOK {
		t.Fatalf("download HTTP=%d want=200 (see request diagnostic)", result.Status)
	}
	mediaType, _, err := mime.ParseMediaType(result.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/octet-stream" {
		t.Fatalf("download Content-Type=%q error=%v", result.Header.Get("Content-Type"), err)
	}
	disposition, params, err := mime.ParseMediaType(result.Header.Get("Content-Disposition"))
	if err != nil || disposition != "attachment" || params["filename"] != file.Name {
		t.Fatalf("download disposition=%q filename=%q want=%q error=%v", disposition, params["filename"], file.Name, err)
	}
	if !bytes.Equal(result.Body, file.Content) {
		t.Fatalf("download %q bytes=%d sha256=%x want bytes=%d sha256=%x", file.Name, len(result.Body), sha256.Sum256(result.Body), len(file.Content), sha256.Sum256(file.Content))
	}
}

func TestRESTDocumentDownloadIntegrity(t *testing.T) {
	dataset := createRESTDataset(t, apiObject{"name": uniqueName("download")})
	datasetID := dataset["id"].(string)
	files := documentDownloadFixtures(t)
	documents := uploadRESTDocuments(t, TestConfig, datasetID, files)
	paths := make([]string, len(files))
	for i, file := range files {
		document := object(t, documents[i])
		if document["name"] != file.Name {
			t.Fatalf("uploaded filename=%v want=%q", document["name"], file.Name)
		}
		paths[i] = "/datasets/" + datasetID + "/documents/" + document["id"].(string)
		t.Run(file.Name, func(t *testing.T) {
			for range 2 {
				requireDocumentDownload(t, rawRESTCall(t, TestConfig, http.MethodGet, paths[i], nil, ""), file)
			}
		})
	}
	t.Run("concurrent", func(t *testing.T) {
		for i, file := range files {
			t.Run(file.Name, func(t *testing.T) {
				t.Parallel()
				requireDocumentDownload(t, rawRESTCall(t, TestConfig, http.MethodGet, paths[i], nil, ""), file)
			})
		}
	})
	t.Run("authentication", func(t *testing.T) {
		for _, token := range []string{"", TestConfig.InvalidAPIKey} {
			client := *TestConfig
			client.APIKey, client.AuthToken = token, ""
			restCode(t, rawRESTCall(t, &client, http.MethodGet, paths[0], nil, ""), http.StatusUnauthorized, 401)
		}
		requireDocumentDownload(t, rawRESTCall(t, TestConfig, http.MethodGet, paths[0], nil, ""), files[0])
	})
	t.Run("missing_or_wrong_dataset", func(t *testing.T) {
		other := createRESTDataset(t, apiObject{"name": uniqueName("download_other")})
		otherID := other["id"].(string)
		for _, path := range []string{
			"/datasets/" + datasetID + "/documents/" + TestConfig.InvalidID,
			"/datasets/" + TestConfig.InvalidID + "/documents/" + object(t, documents[0])["id"].(string),
			"/datasets/" + otherID + "/documents/" + object(t, documents[0])["id"].(string),
		} {
			result := rawRESTCall(t, TestConfig, http.MethodGet, path, nil, "")
			restCode(t, result, http.StatusOK, 404)
			if result.Header.Get("Content-Disposition") != "" {
				t.Fatal("missing document returned an attachment")
			}
		}
		requireDocumentDownload(t, rawRESTCall(t, TestConfig, http.MethodGet, paths[0], nil, ""), files[0])
	})
	t.Run("document_id_route_is_metadata", func(t *testing.T) {
		id := object(t, documents[0])["id"].(string)
		result := restCall(t, TestConfig, http.MethodGet, "/documents/"+id, nil, nil)
		if result.Status != http.StatusOK || object(t, result.JSON["data"])["id"] != id {
			t.Fatalf("document metadata missing uploaded ID %q", id)
		}
		missing := restCall(t, TestConfig, http.MethodGet, "/documents/"+TestConfig.InvalidID, nil, nil)
		if missing.Status != http.StatusOK || missing.JSON["code"] != float64(404) || missing.JSON["message"] != "document not found" {
			t.Fatalf("missing document metadata contract=%d %v", missing.Status, missing.JSON)
		}
		for _, token := range []string{"", TestConfig.InvalidAPIKey} {
			client := *TestConfig
			client.APIKey, client.AuthToken = token, ""
			restCode(t, restCall(t, &client, http.MethodGet, "/documents/"+id, nil, nil), http.StatusUnauthorized, 401)
		}
	})
}

func TestRESTPrivateDocumentRawRoutes(t *testing.T) {
	paths := []string{
		"/documents/images/not-a-valid-image-id",
		"/documents/" + TestConfig.InvalidID + "/images/imagetemps-page-1.png",
		"/documents/" + TestConfig.InvalidID + "/thumbnail",
	}
	for _, path := range paths {
		t.Run(strings.TrimPrefix(path, "/documents/"), func(t *testing.T) {
			for _, token := range []string{"", TestConfig.InvalidAPIKey} {
				client := *TestConfig
				client.APIKey, client.AuthToken = token, ""
				result := rawRESTCall(t, &client, http.MethodGet, path, nil, "")
				restCode(t, result, http.StatusOK, 102)
				if result.JSON["message"] != "Authorization is not valid!" {
					t.Fatalf("private image authorization message=%v", result.JSON["message"])
				}
			}
			result := rawRESTCall(t, TestConfig, http.MethodGet, path, nil, "")
			restCode(t, result, http.StatusOK, 102)
			if result.JSON["message"] != "Image not found." {
				t.Fatalf("missing image message=%v", result.JSON["message"])
			}
		})
	}
}

func TestRESTDocumentRawImagesRequireIndexedOwnership(t *testing.T) {
	dataset := createRESTDataset(t, apiObject{"name": uniqueName("raw_image")})
	datasetID := dataset["id"].(string)
	file := documentDownloadFixtures(t)[5]
	documents := uploadRESTDocuments(t, TestConfig, datasetID, []documentUpload{file})
	id := object(t, documents[0])["id"].(string)
	imageID := datasetID + "-" + file.Name
	for _, path := range []string{
		"/documents/images/" + imageID,
		"/documents/" + id + "/images/" + imageID,
		"/documents/" + id + "/thumbnail",
	} {
		result := rawRESTCall(t, TestConfig, http.MethodGet, path, nil, "")
		restCode(t, result, http.StatusOK, 102)
		if result.JSON["message"] != "Image not found." {
			t.Fatalf("unindexed image message=%v", result.JSON["message"])
		}
	}
	requireDocumentDownload(t, rawRESTCall(t, TestConfig, http.MethodGet, "/datasets/"+datasetID+"/documents/"+id, nil, ""), file)
}

func TestRESTDocumentIndexedImageIntegrity(t *testing.T) {
	dataset := createRESTDataset(t, apiObject{"name": uniqueName("indexed_image")})
	datasetID := dataset["id"].(string)
	file := documentDownloadFixtures(t)[5]
	docs := uploadRESTDocuments(t, TestConfig, datasetID, []documentUpload{
		file, {Name: "other.txt", Content: []byte("This document does not own the image.")},
	})
	id := object(t, docs[0])["id"].(string)
	otherID := object(t, docs[1])["id"].(string)
	data := object(t, restOK(t, restCall(t, TestConfig, http.MethodPost, "/datasets/"+datasetID+"/documents/"+id+"/chunks", apiObject{
		"content":      "A red pixel in an image used to test document ownership.",
		"image_base64": base64.StdEncoding.EncodeToString(file.Content),
	}, nil))["data"])
	chunk := object(t, data["chunk"])
	imageID, ok := chunk["image_id"].(string)
	if !ok || imageID != datasetID+"-"+chunk["id"].(string) {
		t.Fatalf("chunk image ID=%v", chunk["image_id"])
	}
	paths := []string{"/documents/images/" + imageID, "/documents/" + id + "/images/" + imageID}
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		result := restCallContext(t, ctx, TestConfig, http.MethodGet, paths[0], nil, nil)
		if result.JSON == nil {
			if result.Status != http.StatusOK || !bytes.Equal(result.Body, file.Content) {
				t.Fatal("indexed image does not match uploaded PNG")
			}
			break
		}
		restCode(t, result, http.StatusOK, 102)
		if result.JSON["message"] != "Image not found." {
			t.Fatalf("unexpected image response=%v", result.JSON)
		}
		select {
		case <-ctx.Done():
			t.Fatalf("indexed image was not readable before deadline: %v", ctx.Err())
		case <-ticker.C:
		}
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			for range 2 {
				result := rawRESTCall(t, TestConfig, http.MethodGet, path, nil, "")
				if result.Status != http.StatusOK || result.Header.Get("Content-Type") != "image/png" || result.Header.Get("Cache-Control") != "no-store" || !bytes.Equal(result.Body, file.Content) {
					t.Fatalf("image HTTP=%d type=%q cache=%q bytes=%d checksum=%x want=%x", result.Status, result.Header.Get("Content-Type"), result.Header.Get("Cache-Control"), len(result.Body), sha256.Sum256(result.Body), sha256.Sum256(file.Content))
				}
			}
			for _, token := range []string{"", TestConfig.InvalidAPIKey} {
				client := *TestConfig
				client.APIKey, client.AuthToken = token, ""
				result := rawRESTCall(t, &client, http.MethodGet, path, nil, "")
				restCode(t, result, http.StatusOK, 102)
				if result.JSON["message"] != "Authorization is not valid!" {
					t.Fatalf("existing image auth error=%v", result.JSON)
				}
			}
		})
	}
	result := rawRESTCall(t, TestConfig, http.MethodGet, "/documents/"+otherID+"/images/"+imageID, nil, "")
	restCode(t, result, http.StatusOK, 102)
	if result.JSON["message"] != "Image not found." {
		t.Fatalf("foreign document image response=%v", result.JSON)
	}
}

func TestRESTDocumentArtifactSafety(t *testing.T) {
	for _, token := range []string{"", TestConfig.InvalidAPIKey} {
		client := *TestConfig
		client.APIKey, client.AuthToken = token, ""
		restCode(t, rawRESTCall(t, &client, http.MethodGet, "/documents/artifact/not-an-artifact.png", nil, ""), http.StatusUnauthorized, 401)
	}
	for _, tc := range []struct{ filename, message string }{
		{"payload.exe", "invalid file type"},
		{"payload.png.exe", "invalid file type"},
		{"no-extension", "invalid file type"},
		{`..\escape.png`, "invalid filename"},
		{`folder\escape.json`, "invalid filename"},
		{uniqueName("missing") + ".png", "artifact not found"},
		{uniqueName("missing") + ".PNG", "artifact not found"},
		{uniqueName("missing") + ".json", "artifact not found"},
		{uniqueName("missing") + ".html", "artifact not found"},
	} {
		t.Run(tc.filename, func(t *testing.T) {
			path := "/documents/artifact/" + url.PathEscape(tc.filename)
			result := rawRESTCall(t, TestConfig, http.MethodGet, path, nil, "")
			restCode(t, result, http.StatusOK, 102)
			if result.JSON["message"] != tc.message {
				t.Fatalf("artifact message=%v want=%q", result.JSON["message"], tc.message)
			}
			if result.Header.Get("Content-Disposition") != "" {
				t.Fatal("rejected artifact returned file headers")
			}
		})
	}
}
