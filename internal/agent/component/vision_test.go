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

package component

import (
	"context"
	"reflect"
	"testing"

	"github.com/cloudwego/eino/schema"
)

// TestToEinoMessages_PreservesUserInputMultiContent guards against the
// regression where toEinoMessages dropped UserInputMultiContent at the
// LLM-component → chat-invoker boundary. Without this guard, vision
// inputs would pass the component-level test (which asserts on
// ChatInvokeRequest.Messages, the value slice) but be silently stripped
// before reaching the eino chat model layer.
func TestToEinoMessages_PreservesUserInputMultiContent(t *testing.T) {
	uri := "data:image/png;base64,iVBORw0KGgo="
	src := []schema.Message{
		{Role: schema.System, Content: "sys"},
		{
			Role: schema.User,
			UserInputMultiContent: []schema.MessageInputPart{
				{Type: schema.ChatMessagePartTypeText, Text: "describe"},
				{Type: schema.ChatMessagePartTypeImageURL,
					Image: &schema.MessageInputImage{
						MessagePartCommon: schema.MessagePartCommon{URL: &uri},
					}},
			},
		},
	}
	got := toEinoMessages(src)
	if len(got) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(got))
	}
	if got[1].Content != "" {
		t.Errorf("Content should be empty (text moved to multi-content), got %q", got[1].Content)
	}
	if len(got[1].UserInputMultiContent) != 2 {
		t.Fatalf("expected 2 parts in UserInputMultiContent, got %d", len(got[1].UserInputMultiContent))
	}
	if got[1].UserInputMultiContent[0].Text != "describe" {
		t.Errorf("text part=%q, want %q", got[1].UserInputMultiContent[0].Text, "describe")
	}
	if got[1].UserInputMultiContent[1].Image == nil ||
		got[1].UserInputMultiContent[1].Image.URL == nil ||
		*got[1].UserInputMultiContent[1].Image.URL != uri {
		t.Errorf("image URL not preserved; got %+v", got[1].UserInputMultiContent[1])
	}
}

// TestToEinoMessages_EmptyMultiContent verifies the no-images path
// produces a clean *schema.Message (no nil slice leak).
func TestToEinoMessages_EmptyMultiContent(t *testing.T) {
	src := []schema.Message{
		{Role: schema.User, Content: "hi"},
	}
	got := toEinoMessages(src)
	if len(got) != 1 {
		t.Fatalf("expected 1 message, got %d", len(got))
	}
	if len(got[0].UserInputMultiContent) != 0 {
		t.Errorf("UserInputMultiContent should be empty for text-only, got %d parts",
			len(got[0].UserInputMultiContent))
	}
}

// TestExtractDataImages_NoMatches: text without data URIs returns empty.
func TestExtractDataImages_NoMatches(t *testing.T) {
	got := extractDataImages([]string{
		"hello world",
		"plain text with no images",
		"",
	})
	if len(got) != 0 {
		t.Errorf("expected no images, got %v", got)
	}
}

// TestExtractDataImages_FindsDataURIs: data:image/* base64 URIs are
// extracted from text values.
func TestExtractDataImages_FindsDataURIs(t *testing.T) {
	uri1 := "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8/5+hHgAHggJ/PchI7wAAAABJRU5ErkJggg=="
	uri2 := "data:image/jpeg;base64,/9j/4AAQSkZJRgABAQAAAQABAAD/2wBDAAEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQH/2wBDAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQH/wAARCAABAAEDASIAAhEBAxEB/8QAFQABAQAAAAAAAAAAAAAAAAAAAAr/xAAUEAEAAAAAAAAAAAAAAAAAAAAA/8QAFAEBAAAAAAAAAAAAAAAAAAAAAP/EABQRAQAAAAAAAAAAAAAAAAAAAAD/2gAMAwEAAhEDEQA/AL+AB//Z"

	got := extractDataImages([]string{
		"Here is an image: " + uri1 + " and another " + uri2 + " end",
	})
	want := []string{uri1, uri2}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// TestExtractDataImages_Dedupes: same URI in multiple inputs is returned
// once, in first-seen order.
func TestExtractDataImages_Dedupes(t *testing.T) {
	uri := "data:image/png;base64,aGVsbG8="
	got := extractDataImages([]string{
		"first " + uri,
		"second " + uri,
		"third " + uri,
	})
	if len(got) != 1 {
		t.Fatalf("expected 1 deduped URI, got %d: %v", len(got), got)
	}
	if got[0] != uri {
		t.Errorf("URI mismatch: got %q want %q", got[0], uri)
	}
}

// TestExtractDataImages_SkipsNonImageDataURIs: data URLs that are not
// image/* are ignored (e.g. data:text/plain).
func TestExtractDataImages_SkipsNonImageDataURIs(t *testing.T) {
	got := extractDataImages([]string{
		"data:text/plain;base64,aGVsbG8=",
		"data:application/json;base64,e30=",
	})
	if len(got) != 0 {
		t.Errorf("expected non-image data URIs to be ignored, got %v", got)
	}
}

// TestExtractDataImages_RegexEdgeCases exercises the boundaries of
// dataImageRe: empty payloads, structured subtypes (+ in subtype),
// URL-safe alphabet, missing ";base64," prefix, parameter prefix.
//
// Note: the regex uses a permissive trailing character class
// ([A-Za-z0-9+/=_-]+) so it can accept real-world payloads. This means
// two data URIs concatenated without a non-base64 delimiter would
// over-match into one — but real canvas inputs always come as a list
// (visual_files: []string), not a concatenated blob. The
// "two_uris_space_separated" case below verifies the realistic case.
func TestExtractDataImages_RegexEdgeCases(t *testing.T) {
	const png = "data:image/png;base64,iVBORw0KGgo="
	const svg = "data:image/svg+xml;base64,PHN2ZyB4bWxucz0naHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmcnLz4="

	cases := []struct {
		name  string
		input string
		want  []string
	}{
		{"png_standard_alphabet", png, []string{png}},
		{"svg_subtype_with_plus", svg, []string{svg}},
		{"url_safe_alphabet_dashes", "data:image/png;base64,abc-_def=", []string{"data:image/png;base64,abc-_def="}},
		{"two_uris_space_separated", png + " " + svg, []string{png, svg}},
		{"uri_embedded_in_text", "see " + png + " for detail", []string{png}},
		{"missing_base64_token_no_match", "data:image/png,iVBORw0KGgo=", nil},
		{"missing_payload_no_match", "data:image/png;base64,", nil},
		{"charset_prefix_no_match", "data:image/png;charset=utf-8;base64,iVBORw0KGgo=", nil},
		{"non_image_data_uri_no_match", "data:text/plain;base64,aGVsbG8=", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractDataImages([]string{tc.input})
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

// TestToEinoMessages_URLPointerIsolation verifies that mutating the
// URL string in the cloned message does NOT affect the source — guards
// against the shallow-copy footgun surfaced by code review.
func TestToEinoMessages_URLPointerIsolation(t *testing.T) {
	uri := "data:image/png;base64,AAAA"
	src := []schema.Message{
		{
			Role: schema.User,
			UserInputMultiContent: []schema.MessageInputPart{
				{Type: schema.ChatMessagePartTypeImageURL,
					Image: &schema.MessageInputImage{
						MessagePartCommon: schema.MessagePartCommon{URL: &uri},
					}},
			},
		},
	}
	cloned := toEinoMessages(src)
	if cloned[0].UserInputMultiContent[0].Image == nil ||
		cloned[0].UserInputMultiContent[0].Image.URL == nil {
		t.Fatal("cloned message missing image URL")
	}
	// Mutate the cloned side.
	*cloned[0].UserInputMultiContent[0].Image.URL = "mutated"
	// The source should still point to the original URI.
	if *src[0].UserInputMultiContent[0].Image.URL != "data:image/png;base64,AAAA" {
		t.Errorf("source URL mutated through cloned pointer; got %q",
			*src[0].UserInputMultiContent[0].Image.URL)
	}
}

// TestBuildMessagesWithImages_EmptyImages_ReturnsTextMessage: backward
// compat — when no images, the function returns the same shape as
// buildMessages (User message has plain Content, no UserInputMultiContent).
func TestBuildMessagesWithImages_EmptyImages_ReturnsTextMessage(t *testing.T) {
	msgs := buildMessagesWithImages("sys", "user", nil, false)
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages (system + user), got %d", len(msgs))
	}
	if msgs[1].Content != "user" {
		t.Errorf("user message content=%q, want %q", msgs[1].Content, "user")
	}
	if len(msgs[1].UserInputMultiContent) != 0 {
		t.Errorf("UserInputMultiContent should be empty for text-only path, got %d parts",
			len(msgs[1].UserInputMultiContent))
	}
}

// TestBuildMessagesWithImages_WithImages_UsesUserInputMultiContent:
// when images are present, the user message is built with a Text part
// followed by an Image part per URI.
func TestBuildMessagesWithImages_WithImages_UsesUserInputMultiContent(t *testing.T) {
	uri := "data:image/png;base64,iVBORw0KGgo="
	msgs := buildMessagesWithImages("sys", "describe this", []string{uri}, false)
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}

	user := msgs[1]
	if user.Role != schema.User {
		t.Errorf("user msg role=%v, want %v", user.Role, schema.User)
	}
	if user.Content != "" {
		t.Errorf("user msg Content=%q, want empty (text moved to UserInputMultiContent)", user.Content)
	}
	if len(user.UserInputMultiContent) != 2 {
		t.Fatalf("expected 2 parts (text + image), got %d", len(user.UserInputMultiContent))
	}
	if user.UserInputMultiContent[0].Type != schema.ChatMessagePartTypeText {
		t.Errorf("part[0] type=%v, want text", user.UserInputMultiContent[0].Type)
	}
	if user.UserInputMultiContent[0].Text != "describe this" {
		t.Errorf("part[0] text=%q, want %q", user.UserInputMultiContent[0].Text, "describe this")
	}
	if user.UserInputMultiContent[1].Type != schema.ChatMessagePartTypeImageURL {
		t.Errorf("part[1] type=%v, want image_url", user.UserInputMultiContent[1].Type)
	}
	if user.UserInputMultiContent[1].Image == nil {
		t.Fatal("part[1] Image is nil")
	}
	if user.UserInputMultiContent[1].Image.URL == nil {
		t.Fatal("part[1] Image.URL is nil")
	}
	if *user.UserInputMultiContent[1].Image.URL != uri {
		t.Errorf("part[1] URL=%q, want %q", *user.UserInputMultiContent[1].Image.URL, uri)
	}
}

// TestLLM_Invoke_ForwardsImagesToInvoker: end-to-end — a stub ChatInvoker
// captures the messages built from inputs["visual_files"]; the test
// asserts the user message carries the image as multi-modal content.
func TestLLM_Invoke_ForwardsImagesToInvoker(t *testing.T) {
	stub := &stubInvoker{resp: &ChatInvokeResponse{Content: "ok", Model: "echo"}}
	withStubInvoker(t, stub)

	uri := "data:image/png;base64,iVBORw0KGgo="
	c := NewLLMComponent(LLMParam{ModelID: "echo"})
	_, err := c.Invoke(context.Background(), map[string]any{
		"user_prompt":  "what is this?",
		"visual_files": []string{uri},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if stub.captured == nil {
		t.Fatal("invoker captured no request")
	}
	msgs := stub.captured.Messages
	if len(msgs) != 1 { // system is empty, so only user
		t.Fatalf("expected 1 message (user), got %d", len(msgs))
	}
	user := msgs[0]
	if len(user.UserInputMultiContent) != 2 {
		t.Fatalf("expected 2 parts in UserInputMultiContent, got %d", len(user.UserInputMultiContent))
	}
	if user.UserInputMultiContent[1].Image == nil ||
		user.UserInputMultiContent[1].Image.URL == nil ||
		*user.UserInputMultiContent[1].Image.URL != uri {
		t.Errorf("image not forwarded to invoker; got %+v", user.UserInputMultiContent[1])
	}
}

// TestLLM_Invoke_NoVisualFiles_BackwardCompat: when inputs has no
// visual_files key, the user message is text-only (no regression).
func TestLLM_Invoke_NoVisualFiles_BackwardCompat(t *testing.T) {
	stub := &stubInvoker{resp: &ChatInvokeResponse{Content: "ok", Model: "echo"}}
	withStubInvoker(t, stub)

	c := NewLLMComponent(LLMParam{ModelID: "echo"})
	_, err := c.Invoke(context.Background(), map[string]any{
		"user_prompt": "hi",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if stub.captured == nil {
		t.Fatal("invoker captured no request")
	}
	if len(stub.captured.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(stub.captured.Messages))
	}
	if stub.captured.Messages[0].Content != "hi" {
		t.Errorf("content=%q, want %q", stub.captured.Messages[0].Content, "hi")
	}
	if len(stub.captured.Messages[0].UserInputMultiContent) != 0 {
		t.Errorf("UserInputMultiContent should be empty for backward compat, got %d parts",
			len(stub.captured.Messages[0].UserInputMultiContent))
	}
}

// TestLLM_Invoke_VisualFilesAsString: visual_files can also be a single
// string (not a slice) when there's only one image — extractDataImages
// still picks it up.
func TestLLM_Invoke_VisualFilesAsString(t *testing.T) {
	stub := &stubInvoker{resp: &ChatInvokeResponse{Content: "ok", Model: "echo"}}
	withStubInvoker(t, stub)

	uri := "data:image/jpeg;base64,/9j/4AAQ"
	c := NewLLMComponent(LLMParam{ModelID: "echo"})
	_, err := c.Invoke(context.Background(), map[string]any{
		"user_prompt":  "describe",
		"visual_files": "see " + uri,
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if stub.captured == nil {
		t.Fatal("invoker captured no request")
	}
	user := stub.captured.Messages[0]
	if len(user.UserInputMultiContent) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(user.UserInputMultiContent))
	}
	if user.UserInputMultiContent[1].Image == nil ||
		user.UserInputMultiContent[1].Image.URL == nil ||
		*user.UserInputMultiContent[1].Image.URL != uri {
		t.Errorf("image not extracted from single-string visual_files; got %+v",
			user.UserInputMultiContent[1])
	}
}
