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
	"compress/gzip"
	"os"
	"strings"
	"testing"

	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/parser/parser"
	"ragflow/internal/utility"
)

func TestStandaloneMetafileIsParsedAsPNG(t *testing.T) {
	for _, tc := range []struct {
		ext, fixture string
		compressed   bool
	}{
		{ext: "WMF", fixture: "wmf"},
		{ext: "EMF", fixture: "emf"},
		{ext: "WMZ", fixture: "wmf", compressed: true},
		{ext: "EMZ", fixture: "emf", compressed: true},
	} {
		t.Run(tc.ext, func(t *testing.T) {
			data, err := os.ReadFile("../../parser/media/testdata/metafile_test." + tc.fixture)
			if err != nil {
				t.Fatal(err)
			}
			if tc.compressed {
				var compressed bytes.Buffer
				w := gzip.NewWriter(&compressed)
				if _, err := w.Write(data); err != nil {
					t.Fatal(err)
				}
				if err := w.Close(); err != nil {
					t.Fatal(err)
				}
				data = compressed.Bytes()
			}
			name := "drawing." + tc.ext
			producer, fileType, err := parser.GetParser(utility.GetFileType(name), data)
			if err != nil || fileType != utility.FileTypeVISUAL || utility.FilenameType(name) != utility.FileTypeVISUAL {
				t.Fatalf("standalone file type = %q, error = %v", fileType, err)
			}
			result, handled, err := maybeDispatchImage(t.Context(), producer, nil, fileType, name, data, nil,
				map[string]schema.ParserSetup{"image": {"ocr_enabled": false}}, false, visionSettings{})
			if err != nil || !handled || len(result.JSON) != 1 {
				t.Fatalf("parsed items = %d, handled = %v, error = %v", len(result.JSON), handled, err)
			}
			image, _ := result.JSON[0].ToMap()["image"].(string)
			if !strings.HasPrefix(image, "data:image/png;base64,iVBORw0KGgo") {
				t.Fatal("standalone metafile did not produce a PNG image")
			}
			if result.File["size"] != len(data) {
				t.Fatalf("reported file size = %v, want %d", result.File["size"], len(data))
			}
		})
	}
}
