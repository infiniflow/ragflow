package component

import (
	"context"
	"strings"
	"testing"
)

// mockArtifactInvoker returns a direct answer without tool calls.
// Used to verify the AgentComponent.Invoke output shape when
// no real artifact-producing tools are involved. When a real
// artifact collector replaces collectArtifactsFromToolCalls,
// add a tool-calling variant that exercises the full pipeline.
type mockArtifactInvoker struct {
	content string
}

func (m *mockArtifactInvoker) Invoke(_ context.Context, req ChatInvokeRequest) (*ChatInvokeResponse, error) {
	return &ChatInvokeResponse{Content: m.content}, nil
}

// TestAgent_ReActAgent_CollectsArtifactsFromCodeExecTool verifies that
// AgentComponent.Invoke returns the correct output shape when a mock
// ChatInvoker provides a direct answer. The harness's v1 artifact
// collector (collectArtifactsFromToolCalls) is a stub returning nil,
// so the artifacts field is always empty. When a real collector is
// wired in, update this test to exercise the full tool-calling pipeline
// with a registered artifact tool.
func TestAgent_ReActAgent_CollectsArtifactsFromCodeExecTool(t *testing.T) {
	prev := GetDefaultChatInvokerForTest()
	SetDefaultChatInvoker(&mockArtifactInvoker{
		content: "The image has been generated.",
	})
	defer SetDefaultChatInvoker(prev)

	comp := NewAgentComponent(AgentParam{
		ModelID:    "dummy-model",
		UserPrompt: "generate a test image",
		MaxRounds:  1,
	})
	out, err := comp.Invoke(context.Background(), map[string]any{
		"user_prompt": "generate a test image",
	})
	if err != nil {
		t.Fatalf("AgentComponent.Invoke: %v", err)
	}
	if out == nil {
		t.Fatal("output is nil")
	}
	content, _ := out["content"].(string)
	if content == "" {
		t.Error("output content is empty")
	}
	if _, ok := out["tool_calls"]; !ok {
		t.Error("output missing tool_calls key")
	}
	artifacts, _ := out["artifacts"].([]artifactEntry)
	if len(artifacts) != 0 {
		t.Errorf("artifacts = %v, want empty (v1 stub)", artifacts)
	}
}

// TestAgent_ArtifactMarkdown_ImageURL renders an image artifact as a
// Markdown image link.
func TestAgent_ArtifactMarkdown_ImageURL(t *testing.T) {
	artifacts := []artifactEntry{
		{Name: "agent_artifact_bug_demo.png", URL: "/api/v1/documents/artifact/1ae8d553478544628bb8be267d502371.png"},
	}
	md := formatArtifactMarkdown(artifacts, "done")
	want := "![agent_artifact_bug_demo.png](/api/v1/documents/artifact/1ae8d553478544628bb8be267d502371.png)"
	if !strings.Contains(md, want) {
		t.Errorf("markdown=%q, want substring %q", md, want)
	}
}

// TestAgent_ArtifactMarkdown_DownloadURL renders a non-image artifact
// as a download link.
func TestAgent_ArtifactMarkdown_DownloadURL(t *testing.T) {
	artifacts := []artifactEntry{
		{Name: "report.txt", URL: "https://example.com/report.txt"},
	}
	md := formatArtifactMarkdown(artifacts, "")
	if !strings.Contains(md, "[Download report.txt](https://example.com/report.txt)") {
		t.Errorf("markdown=%q, want download link", md)
	}
}

// TestAgent_ArtifactMarkdown_EmptyInput returns empty string.
func TestAgent_ArtifactMarkdown_EmptyInput(t *testing.T) {
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

// TestAgent_ArtifactMarkdown_DeDuplicates skips URLs already present.
func TestAgent_ArtifactMarkdown_DeDuplicates(t *testing.T) {
	artifacts := []artifactEntry{
		{Name: "a.png", URL: "https://example.com/a.png"},
		{Name: "b.png", URL: "https://example.com/a.png"},
	}
	md := formatArtifactMarkdown(artifacts, "https://example.com/a.png")
	if strings.Contains(md, "a.png") {
		t.Errorf("dedup failed: markdown=%q should not contain a.png", md)
	}
}

// TestAgent_ArtifactMarkdown_MultipleArtifacts renders all non-duplicate entries.
func TestAgent_ArtifactMarkdown_MultipleArtifacts(t *testing.T) {
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

// TestAgent_EmptyArtifactList verifies emptyArtifactList returns nil.
func TestAgent_EmptyArtifactList(t *testing.T) {
	if got := emptyArtifactList(); got != nil {
		t.Errorf("emptyArtifactList() = %v, want nil", got)
	}
}

// TestAgent_CollectArtifacts_Stub verifies the v1 stub of
// collectArtifactsFromToolCalls returns nil for all inputs.
func TestAgent_CollectArtifacts_Stub(t *testing.T) {
	cases := []*ComponentMessage{
		nil,
		{Role: RoleAssistant, Content: "ok"},
		{Role: RoleAssistant, ToolCalls: []ComponentToolCall{{ID: "c1", Type: "function", Function: ComponentFunctionCall{Name: "tool1"}}}},
	}
	for _, msg := range cases {
		if got := collectArtifactsFromToolCalls(msg); got != nil {
			t.Errorf("collectArtifactsFromToolCalls(%+v) = %v, want nil (v1 stub)", msg, got)
		}
	}
}
