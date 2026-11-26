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

//go:build cgo

package parser

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"image/png"
	"io"
	"os"
	"path"
	"strings"
	"testing"

	officeOxide "github.com/yfedoseev/office_oxide/go"
)

func TestOfficeCompressedMetafiles(t *testing.T) {
	docx, err := os.ReadFile("testdata/docx_table_image.docx")
	if err != nil {
		t.Fatal(err)
	}
	w := officeOxide.NewPptxWriter()
	defer w.Close()
	slide := w.AddSlide()
	if err := w.AddSlideImage(slide, officeTestPNG(t, 0), "png", 914400, 914400, 914400, 457200); err != nil {
		t.Fatal(err)
	}
	pptx, err := w.ToBytes()
	if err != nil {
		t.Fatal(err)
	}

	for _, doc := range []struct {
		format string
		data   []byte
		parse  func(context.Context, string, []byte) ParseResult
	}{
		{"docx", docx, NewDOCXParser().ParseWithResult},
		{"pptx", pptx, NewPPTXParser().ParseWithResult},
	} {
		for _, pic := range []struct{ ext, fixture string }{{"wmz", "wmf"}, {"emz", "emf"}} {
			t.Run(doc.format+"/"+pic.ext, func(t *testing.T) {
				raw, err := os.ReadFile("../media/testdata/metafile_test." + pic.fixture)
				if err != nil {
					t.Fatal(err)
				}
				var zipped bytes.Buffer
				zw := gzip.NewWriter(&zipped)
				if _, err := zw.Write(raw); err != nil {
					t.Fatal(err)
				}
				if err := zw.Close(); err != nil {
					t.Fatal(err)
				}
				data := replaceOOXMLImage(t, doc.data, pic.ext, zipped.Bytes())
				ctx, cache := officeMediaTestContext()
				result := doc.parse(ctx, "compressed-metafile."+doc.format, data)
				if result.Err != nil || len(result.Warnings) != 0 {
					t.Fatalf("parse error = %v, warnings = %v", result.Err, result.Warnings)
				}
				for _, item := range ItemsToMaps(result.JSON) {
					if item["doc_type_kwd"] != "image" {
						continue
					}
					ref := requireOfficeMediaRef(t, item)
					if ref.Format != doc.format || ref.MIME != "image/png" {
						t.Fatalf("media_ref = %+v", ref)
					}
					decoded, err := cache.Get(ctx, *ref)
					if err != nil {
						t.Fatal(err)
					}
					img, err := png.Decode(bytes.NewReader(decoded))
					if err != nil {
						t.Fatal(err)
					}
					center := img.Bounds().Min.Add(img.Bounds().Size().Div(2))
					r, g, b, _ := img.At(center.X, center.Y).RGBA()
					if r < 0x8000 || g > 0x2000 || b > 0x2000 {
						t.Fatalf("central pixel is not red: %04x %04x %04x", r, g, b)
					}
					return
				}
				t.Fatal("embedded image not emitted")
			})
		}
	}
}

func replaceOOXMLImage(t *testing.T, data []byte, ext string, payload []byte) []byte {
	t.Helper()
	source, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	var oldMedia string
	for _, file := range source.File {
		if strings.Contains(file.Name, "/media/") && path.Ext(file.Name) == ".png" {
			if oldMedia != "" {
				t.Fatal("fixture must contain exactly one image")
			}
			oldMedia = file.Name
		}
	}
	if oldMedia == "" {
		t.Fatal("fixture is missing its image")
	}
	newMedia := strings.TrimSuffix(oldMedia, ".png") + "." + ext
	var buf bytes.Buffer
	dest := zip.NewWriter(&buf)
	for _, file := range source.File {
		if !strings.HasSuffix(file.Name, ".rels") && file.Name != "[Content_Types].xml" && file.Name != oldMedia {
			if err := dest.Copy(file); err != nil {
				t.Fatal(err)
			}
			continue
		}
		name := file.Name
		var body []byte
		if name == oldMedia {
			name, body = newMedia, payload
		} else {
			r, err := file.Open()
			if err != nil {
				t.Fatal(err)
			}
			body, err = io.ReadAll(r)
			r.Close()
			if err != nil {
				t.Fatal(err)
			}
			if name == "[Content_Types].xml" {
				contentType := []byte(`<Default Extension="` + ext + `" ContentType="image/x-` + ext + `"/></Types>`)
				body = bytes.Replace(body, []byte("</Types>"), contentType, 1)
			} else {
				body = bytes.ReplaceAll(body, []byte(path.Base(oldMedia)), []byte(path.Base(newMedia)))
			}
		}
		w, err := dest.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	if err := dest.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
