package handler

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	compilercommon "ragflow/internal/ingestion/component/knowledge_compiler/common"
	"ragflow/internal/ingestion/component/knowledge_compiler/wiki"
)

type claimGuidanceChat struct {
	requests []compilercommon.ChatRequest
	err      error
}

func (c *claimGuidanceChat) Chat(_ context.Context, req compilercommon.ChatRequest) (*compilercommon.ChatResponse, error) {
	c.requests = append(c.requests, req)
	return nil, c.err
}

// Saved claim guidance must reach the actual MAP model request on both create
// and update, using the same statement/subject shape as the template editor.
func TestCompilationTemplateGroupSavedClaimGuidanceReachesWiki(t *testing.T) {
	db := setupGroupHandlerDB(t)
	r := newCompilationTemplateGroupHandlerRouter()
	groupID := ""
	for _, operation := range []string{"create", "update"} {
		t.Run(operation, func(t *testing.T) {
			statement := "Extract audited financial claims for " + operation + "."
			subject := "Use the legal entity name for " + operation + "."
			body := fmt.Sprintf(`{"name":"Wiki","templates":[{"name":"Wiki","kind":"wiki","config":{"kind":"wiki","claim":{"fields":[{"statement":%q,"subject":%q}]}}}]}`, statement, subject)
			var saved groupSaveResponse
			if operation == "create" {
				saved = postCompilationTemplateGroup(t, r, body)
				groupID, _ = saved.Data["id"].(string)
			} else {
				saved = putCompilationTemplateGroup(t, r, groupID, body)
			}
			if saved.Code != int(common.CodeSuccess) || groupID == "" {
				t.Fatalf("save: code=%d message=%s id=%s", saved.Code, saved.Message, groupID)
			}
			var template entity.CompilationTemplate
			if err := db.Where("group_id = ?", groupID).Take(&template).Error; err != nil {
				t.Fatal(err)
			}
			captured := context.Canceled
			chat := &claimGuidanceChat{err: captured}
			param := compilercommon.Param{}.Defaults()
			param.LLMID, param.Language = "test-llm", "English"
			_, err := wiki.Run(t.Context(), compilercommon.Deps{Chat: chat}, param, compilercommon.Inputs{
				DocID:           "doc-1",
				Chunks:          []compilercommon.Chunk{{ID: "chunk-1", Text: "Acme reported audited revenue."}},
				VariantSpecific: map[string]any{"parser_config": map[string]any(template.Config)},
			})
			if !errors.Is(err, captured) {
				t.Fatalf("expected to stop after capturing MAP request, got %v", err)
			}
			if len(chat.requests) != 1 {
				t.Fatalf("expected one MAP model request, got %d", len(chat.requests))
			}
			for _, want := range []string{statement, subject} {
				if !strings.Contains(chat.requests[0].UserPrompt, want) {
					t.Errorf("successfully saved guidance missing from MAP model request: %q", want)
				}
			}
		})
	}
}
