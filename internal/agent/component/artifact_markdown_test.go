//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package component

import (
	"strings"
	"testing"
)

// TestFormatArtifactMarkdown_Empty: no artifacts → empty string.
func TestFormatArtifactMarkdown_Empty(t *testing.T) {
	if got := formatArtifactMarkdown(nil, "answer"); got != "" {
		t.Errorf("expected empty for nil artifacts, got %q", got)
	}
	if got := formatArtifactMarkdown([]artifactEntry{}, "answer"); got != "" {
		t.Errorf("expected empty for empty slice, got %q", got)
	}
}

// TestFormatArtifactMarkdown_ImageLink: image URL → markdown image syntax.
func TestFormatArtifactMarkdown_ImageLink(t *testing.T) {
	arts := []artifactEntry{
		{Name: "chart", URL: "https://example.com/chart.png"},
	}
	got := formatArtifactMarkdown(arts, "answer")
	want := "\n\n![chart](https://example.com/chart.png)"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestFormatArtifactMarkdown_DownloadLink: non-image URL → download link.
func TestFormatArtifactMarkdown_DownloadLink(t *testing.T) {
	arts := []artifactEntry{
		{Name: "report.pdf", URL: "https://example.com/report.pdf"},
	}
	got := formatArtifactMarkdown(arts, "answer")
	want := "\n\n[Download report.pdf](https://example.com/report.pdf)"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestFormatArtifactMarkdown_DedupesAgainstExistingText: if the URL is
// already in the answer, the artifact link is omitted.
func TestFormatArtifactMarkdown_DedupesAgainstExistingText(t *testing.T) {
	arts := []artifactEntry{
		{Name: "chart", URL: "https://example.com/chart.png"},
	}
	got := formatArtifactMarkdown(arts, "see https://example.com/chart.png above")
	if got != "" {
		t.Errorf("expected empty (URL already in text), got %q", got)
	}
}

// TestFormatArtifactMarkdown_Mixed: image + non-image + deduped link.
func TestFormatArtifactMarkdown_Mixed(t *testing.T) {
	arts := []artifactEntry{
		{Name: "a.png", URL: "https://example.com/a.png"},
		{Name: "report.pdf", URL: "https://example.com/report.pdf"},
		{Name: "b.png", URL: "https://example.com/b.png"}, // already in text
	}
	got := formatArtifactMarkdown(arts, "see https://example.com/b.png here")
	if !strings.Contains(got, "![a.png](https://example.com/a.png)") {
		t.Errorf("missing image link for a.png; got %q", got)
	}
	if !strings.Contains(got, "[Download report.pdf](https://example.com/report.pdf)") {
		t.Errorf("missing download link for report.pdf; got %q", got)
	}
	if strings.Contains(got, "b.png") {
		t.Errorf("b.png should be deduped; got %q", got)
	}
}

// TestFormatArtifactMarkdown_SkipsEmptyFields: entries with empty URL
// or name are skipped.
func TestFormatArtifactMarkdown_SkipsEmptyFields(t *testing.T) {
	arts := []artifactEntry{
		{Name: "", URL: "https://example.com/a.png"},
		{Name: "valid", URL: ""},
		{Name: "good", URL: "https://example.com/good.pdf"},
	}
	got := formatArtifactMarkdown(arts, "")
	if !strings.Contains(got, "good.pdf") {
		t.Errorf("expected valid entry; got %q", got)
	}
	if strings.Contains(got, "example.com/a.png") {
		t.Errorf("empty-name entry should be skipped; got %q", got)
	}
}
