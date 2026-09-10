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

package chat

import (
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"

	"ragflow/internal/tokenizer"
)

func TestFitMessages_EverythingFits(t *testing.T) {
	msgs := []schema.Message{
		{Role: schema.System, Content: "you are helpful"},
		{Role: schema.User, Content: "hello"},
	}
	fitted, fitErr := FitMessages("", msgs, 100000)
	if fitErr != "" {
		t.Fatalf("unexpected fit error: %s", fitErr)
	}
	if len(fitted) != 2 {
		t.Fatalf("got %d messages, want 2", len(fitted))
	}
	if fitted[0].Content != "you are helpful" || fitted[1].Content != "hello" {
		t.Fatalf("messages modified when they fit: %+v", fitted)
	}
}

func TestFitMessages_PreservesImageOnlyTurn(t *testing.T) {
	imgURL := "data:image/png;base64,AAAA"
	msgs := []schema.Message{
		{Role: schema.System, Content: "you are helpful"},
		{Role: schema.User, UserInputMultiContent: []schema.MessageInputPart{
			{Type: schema.ChatMessagePartTypeImageURL, Image: &schema.MessageInputImage{
				MessagePartCommon: schema.MessagePartCommon{URL: &imgURL},
			}},
		}},
	}
	fitted, fitErr := FitMessages("", msgs, 100000)
	if fitErr != "" {
		t.Fatalf("unexpected fit error: %s", fitErr)
	}
	if len(fitted) != 2 {
		t.Fatalf("got %d messages, want 2 (image-only turn must be preserved)", len(fitted))
	}
	last := fitted[len(fitted)-1]
	if len(last.UserInputMultiContent) != 1 || last.UserInputMultiContent[0].Type != schema.ChatMessagePartTypeImageURL {
		t.Fatalf("image parts lost after fitting: %+v", last)
	}
}

func TestFitMessages_IncludesSyntheticSystemPrompt(t *testing.T) {
	msgs := []schema.Message{{Role: schema.User, Content: "hello"}}
	fitted, fitErr := FitMessages("be brief", msgs, 100000)
	if fitErr != "" {
		t.Fatalf("unexpected fit error: %s", fitErr)
	}
	if len(fitted) != 2 {
		t.Fatalf("got %d messages, want 2 (synthetic system prompt + user)", len(fitted))
	}
	if fitted[0].Role != schema.System || fitted[0].Content != "be brief" {
		t.Fatalf("synthetic system prompt not preserved: %+v", fitted[0])
	}
}

func TestFitMessages_DropsMiddleWhenOverBudget(t *testing.T) {
	long := strings.Repeat("x ", 5000)
	msgs := []schema.Message{
		{Role: schema.System, Content: long},
		{Role: schema.User, Content: "middle"},
		{Role: schema.User, Content: "last"},
	}
	fitted, fitErr := FitMessages("", msgs, 1000)
	if fitErr != "" {
		t.Fatalf("unexpected fit error: %s", fitErr)
	}
	if len(fitted) != 2 {
		t.Fatalf("got %d messages, want 2 (middle dropped, system + last user kept)", len(fitted))
	}
	if fitted[0].Role != schema.System || fitted[1].Role != schema.User {
		t.Fatalf("unexpected roles: %+v", fitted)
	}
	if !strings.Contains(fitted[1].Content, "last") {
		t.Fatalf("last user message not preserved: %+v", fitted[1])
	}
}

// TestFitMessages_SystemKeptButEmptied locks the write-back for a system
// message that the fitter keeps but trims to empty (the final user turn alone
// fills the budget): the fitted (empty) content must be written back instead
// of the original, so the conversation stays within the budget.
func TestFitMessages_SystemKeptButEmptied(t *testing.T) {
	origSys := strings.Repeat("s ", 3000) // dominates (>80% of tokens)
	msgs := []schema.Message{
		{Role: schema.System, Content: origSys},
		{Role: schema.User, Content: strings.Repeat("u ", 600)}, // alone exceeds the budget
	}
	fitted, fitErr := FitMessages("", msgs, 500)
	if fitErr != "" {
		t.Fatalf("unexpected fit error: %s", fitErr)
	}
	if len(fitted) != 2 {
		t.Fatalf("got %d messages, want 2 (both kept)", len(fitted))
	}
	if fitted[0].Role != schema.System || fitted[0].Content != "" {
		t.Fatalf("system should be kept but trimmed to empty, got %+v", fitted[0])
	}
	if fitted[1].Role != schema.User || fitted[1].Content == origSys {
		t.Fatalf("user turn wrong after fitting: %+v", fitted[1])
	}
	total := tokenizer.NumTokensFromString(fitted[0].Content) + tokenizer.NumTokensFromString(fitted[1].Content)
	if total > 500 {
		t.Fatalf("fitted total %d exceeds budget 500", total)
	}
}

// TestFitMessages_FoldsMultipleTextParts verifies that every non-empty text
// part of a multi-modal message participates in the token budget: the parts
// are folded into a single fitted text on the first text part and additional
// text parts are removed, so no text escapes the budget after reconstruction.
func TestFitMessages_FoldsMultipleTextParts(t *testing.T) {
	long1 := strings.Repeat("a ", 3000)
	long2 := strings.Repeat("b ", 3000)
	imgURL := "data:image/png;base64,AAAA"
	msgs := []schema.Message{
		{Role: schema.System, Content: "sys"},
		{Role: schema.User, UserInputMultiContent: []schema.MessageInputPart{
			{Type: schema.ChatMessagePartTypeText, Text: long1},
			{Type: schema.ChatMessagePartTypeImageURL, Image: &schema.MessageInputImage{
				MessagePartCommon: schema.MessagePartCommon{URL: &imgURL},
			}},
			{Type: schema.ChatMessagePartTypeText, Text: long2},
		}},
	}
	fitted, fitErr := FitMessages("", msgs, 2000)
	if fitErr != "" {
		t.Fatalf("unexpected fit error: %s", fitErr)
	}
	if len(fitted) != 2 {
		t.Fatalf("got %d messages, want 2", len(fitted))
	}
	last := fitted[len(fitted)-1]
	textParts := 0
	imageParts := 0
	for _, part := range last.UserInputMultiContent {
		switch part.Type {
		case schema.ChatMessagePartTypeText:
			textParts++
		case schema.ChatMessagePartTypeImageURL:
			imageParts++
		}
	}
	if textParts != 1 {
		t.Fatalf("got %d text parts, want 1 (folded): %+v", textParts, last.UserInputMultiContent)
	}
	if imageParts != 1 {
		t.Fatalf("image part lost after trimming: %+v", last.UserInputMultiContent)
	}
	if total := tokenizer.NumTokensFromString(last.UserInputMultiContent[0].Text); total > 2000 {
		t.Fatalf("fitted text totals %d tokens, exceeds budget 2000", total)
	}
}

// TestFitMessages_ImageOnlyLastTurnOverBudget locks the over-budget path where
// an image-only turn is the last non-system message (ll2 = 0 tokens): the
// fitter keeps it, gives the whole budget to the system messages, and the
// image-only turn must survive reconstruction untouched.
func TestFitMessages_ImageOnlyLastTurnOverBudget(t *testing.T) {
	imgURL := "data:image/png;base64,AAAA"
	msgs := []schema.Message{
		{Role: schema.System, Content: strings.Repeat("s ", 3000)}, // dominates (>80% of tokens)
		{Role: schema.User, UserInputMultiContent: []schema.MessageInputPart{
			{Type: schema.ChatMessagePartTypeImageURL, Image: &schema.MessageInputImage{
				MessagePartCommon: schema.MessagePartCommon{URL: &imgURL},
			}},
		}},
	}
	fitted, fitErr := FitMessages("", msgs, 500)
	if fitErr != "" {
		t.Fatalf("unexpected fit error: %s", fitErr)
	}
	if len(fitted) != 2 {
		t.Fatalf("got %d messages, want 2 (system + image-only turn)", len(fitted))
	}
	if fitted[0].Role != schema.System || fitted[0].Content == msgs[0].Content {
		t.Fatalf("system should be trimmed to the budget: %+v", fitted[0])
	}
	if total := tokenizer.NumTokensFromString(fitted[0].Content); total > 500 {
		t.Fatalf("system exceeds budget after fit: %d tokens", total)
	}
	last := fitted[len(fitted)-1]
	if last.Role != schema.User || len(last.UserInputMultiContent) != 1 || last.UserInputMultiContent[0].Type != schema.ChatMessagePartTypeImageURL {
		t.Fatalf("image-only turn lost or modified after over-budget fitting: %+v", last)
	}
}
