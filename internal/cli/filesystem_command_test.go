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

package cli

import (
	"context"
	"testing"

	"ragflow/internal/cli/filesystem"
)

type recordingFileProvider struct {
	searchOptions *filesystem.SearchOptions
}

func (p *recordingFileProvider) Name() string        { return "files" }
func (p *recordingFileProvider) Description() string { return "test file provider" }
func (p *recordingFileProvider) Supports(path string) bool {
	return path == "files"
}
func (p *recordingFileProvider) List(context.Context, string, *filesystem.ListOptions) (*filesystem.Result, error) {
	return &filesystem.Result{}, nil
}
func (p *recordingFileProvider) Search(_ context.Context, _ string, opts *filesystem.SearchOptions) (*filesystem.Result, error) {
	p.searchOptions = opts
	return &filesystem.Result{}, nil
}
func (p *recordingFileProvider) Cat(context.Context, string) ([]byte, error) {
	return nil, nil
}

func TestSearchFilesPassesNumberAsLimit(t *testing.T) {
	provider := &recordingFileProvider{}
	engine := filesystem.NewEngine()
	engine.RegisterProvider(provider)
	cli := &CLI{
		ContextEngine: engine,
		Config: &CommandLineConfig{
			OutputFormat: OutputFormatJSON,
		},
	}

	if err := cli.executeFilesystemInner("search foo files -n 20"); err != nil {
		t.Fatalf("executeFilesystemInner() error = %v", err)
	}
	if provider.searchOptions == nil {
		t.Fatal("file provider Search() was not called")
	}
	if provider.searchOptions.Limit != 20 {
		t.Errorf("SearchOptions.Limit = %d, want 20", provider.searchOptions.Limit)
	}
	if provider.searchOptions.TopK != 20 {
		t.Errorf("SearchOptions.TopK = %d, want 20", provider.searchOptions.TopK)
	}
}
