package component

import (
	"strings"
	"testing"
)

// TestAgent_ArtifactCollector_Stub verifies the stub returns nil.
// The harness-based collector is a v1 placeholder; when a real
// implementation is wired in, update these tests.
func TestAgent_ArtifactCollector_Stub(t *testing.T) {
	tests := []struct {
		name string
		msg  *ComponentMessage
	}{
		{"nil message", nil},
		{"empty message", &ComponentMessage{Role: RoleAssistant, Content: "ok"}},
		{"message with tool calls", &ComponentMessage{
			Role: RoleAssistant, Content: "calling tools",
			ToolCalls: []ComponentToolCall{{
				ID: "call_1", Type: "function",
				Function: ComponentFunctionCall{Name: "code_exec", Arguments: `{"code":"print(1)"}`},
			}},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := collectArtifactsFromToolCalls(tt.msg); got != nil {
				t.Errorf("collectArtifactsFromToolCalls = %v, want nil (v1 stub)", got)
			}
		})
	}
}

// TestAgent_EmptyArtifactList verifies emptyArtifactList returns nil.
func TestAgent_EmptyArtifactList(t *testing.T) {
	if got := emptyArtifactList(); got != nil {
		t.Errorf("emptyArtifactList() = %v, want nil", got)
	}
}

// TestAgent_FormatArtifactMarkdown_ImageURL renders an image artifact
// as a Markdown image link. Mirrors the original eino test's assertion
// on the URL shape.
func TestAgent_FormatArtifactMarkdown_ImageURL(t *testing.T) {
	artifacts := []artifactEntry{
		{Name: "agent_artifact_bug_demo.png", URL: "/api/v1/documents/artifact/1ae8d553478544628bb8be267d502371.png"},
	}
	md := formatArtifactMarkdown(artifacts, "done")
	want := "![agent_artifact_bug_demo.png](/api/v1/documents/artifact/1ae8d553478544628bb8be267d502371.png)"
	if !strings.Contains(md, want) {
		t.Errorf("markdown=%q, want substring %q", md, want)
	}
}

// TestAgent_FormatArtifactMarkdown_DownloadURL renders a non-image
// artifact as a download link.
func TestAgent_FormatArtifactMarkdown_DownloadURL(t *testing.T) {
	artifacts := []artifactEntry{
		{Name: "report.txt", URL: "https://example.com/report.txt"},
	}
	md := formatArtifactMarkdown(artifacts, "")
	if !strings.Contains(md, "[Download report.txt](https://example.com/report.txt)") {
		t.Errorf("markdown=%q, want download link", md)
	}
}

// TestAgent_FormatArtifactMarkdown_EmptyInput returns empty string.
func TestAgent_FormatArtifactMarkdown_EmptyInput(t *testing.T) {
	if got := formatArtifactMarkdown(nil, "existing"); got != "" {
		t.Errorf("formatArtifactMarkdown(nil) = %q, want \"\"", got)
	}
	if got := formatArtifactMarkdown([]artifactEntry{}, "existing"); got != "" {
		t.Errorf("formatArtifactMarkdown([]) = %q, want \"\"", got)
	}
	if got := formatArtifactMarkdown([]artifactEntry{{Name: "", URL: ""}}, "existing"); got != "" {
		t.Errorf("formatArtifactMarkdown(empty entry) = %q, want \"\"", got)
	}
}

// TestAgent_FormatArtifactMarkdown_DeDuplicates skips URLs already
// present in the existing text.
func TestAgent_FormatArtifactMarkdown_DeDuplicates(t *testing.T) {
	artifacts := []artifactEntry{
		{Name: "a.png", URL: "https://example.com/a.png"},
		{Name: "b.png", URL: "https://example.com/a.png"},
	}
	md := formatArtifactMarkdown(artifacts, "https://example.com/a.png")
	if strings.Contains(md, "a.png") {
		t.Errorf("dedup failed: markdown=%q should not contain a.png", md)
	}
}

// TestAgent_FormatArtifactMarkdown_MultipleArtifacts renders all
// non-duplicate entries.
func TestAgent_FormatArtifactMarkdown_MultipleArtifacts(t *testing.T) {
	artifacts := []artifactEntry{
		{Name: "image.png", URL: "https://example.com/image.png"},
		{Name: "doc.pdf", URL: "https://example.com/doc.pdf"},
	}
	md := formatArtifactMarkdown(artifacts, "existing")
	if !strings.Contains(md, "![image.png]") {
		t.Errorf("missing image link: %q", md)
	}
	if !strings.Contains(md, "[Download doc.pdf]") {
		t.Errorf("missing download link: %q", md)
	}
}
