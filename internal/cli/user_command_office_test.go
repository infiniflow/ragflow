//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

//go:build cgo

package cli

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseLocalFileCorrectsOfficeFormat(t *testing.T) {
	data, err := os.ReadFile("../parser/parser/testdata/docx_table_image.docx")
	if err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(t.TempDir(), "renamed.doc")
	if err := os.WriteFile(filename, data, 0600); err != nil {
		t.Fatal(err)
	}
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	previous := os.Stderr
	os.Stderr = w
	t.Cleanup(func() { os.Stderr = previous })
	c := &CLI{Config: &CommandLineConfig{CLIMode: APIMode}}
	response, err := c.APIParseLocalFileCommand(1, &Command{Params: map[string]interface{}{"filename": filename}})
	if err != nil {
		t.Fatal(err)
	}
	if result, ok := response.(*SimpleResponse); !ok || result.Code != 0 {
		t.Fatalf("unexpected response: %#v", response)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	warning, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(warning), "WARNING:") || !strings.Contains(string(warning), "docx") {
		t.Fatalf("missing format correction warning: %q", warning)
	}
}
