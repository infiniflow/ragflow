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

package handler

import (
	"context"
	"errors"
	"testing"
)

func TestCollectMindMapStream(t *testing.T) {
	t.Run("joins successful chunks", func(t *testing.T) {
		chunks := make(chan string, 2)
		chunks <- "# Title"
		chunks <- "\n## Section"
		close(chunks)
		streamErrs := make(chan error)
		close(streamErrs)

		got, err := collectMindMapStream(t.Context(), chunks, streamErrs)
		if err != nil {
			t.Fatalf("collect stream: %v", err)
		}
		if got != "# Title\n## Section" {
			t.Fatalf("unexpected stream content: %q", got)
		}
	})

	t.Run("returns asynchronous provider error after partial output", func(t *testing.T) {
		providerErr := errors.New("provider stream failed")
		chunks := make(chan string, 1)
		chunks <- "# Partial"
		close(chunks)
		streamErrs := make(chan error, 1)
		streamErrs <- providerErr
		close(streamErrs)

		_, err := collectMindMapStream(t.Context(), chunks, streamErrs)
		if !errors.Is(err, providerErr) {
			t.Fatalf("expected provider error, got %v", err)
		}
	})

	t.Run("returns canceled context after channels close", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		chunks := make(chan string)
		close(chunks)
		streamErrs := make(chan error)
		close(streamErrs)

		_, err := collectMindMapStream(ctx, chunks, streamErrs)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context cancellation, got %v", err)
		}
	})
}

// The mind map contract: when there is nothing to map, the backend returns an
// honest empty tree (root with no children) and the frontend renders an empty
// state. No synthetic nodes are fabricated.
func TestParseMindMapMarkdownEmpty(t *testing.T) {
	t.Run("prose without headings or list items yields an empty tree", func(t *testing.T) {
		node := parseMindMapMarkdown("I cannot summarize this text right now.")
		if node.ID != "root" || len(node.Children) != 0 {
			t.Fatalf("expected root-only tree, got %+v", node)
		}
	})

	t.Run("think-only answer yields an empty tree", func(t *testing.T) {
		node := parseMindMapMarkdown("<think>reasoning without any outline</think>")
		if node.ID != "root" || len(node.Children) != 0 {
			t.Fatalf("expected root-only tree, got %+v", node)
		}
	})

	t.Run("blank answer yields an empty tree", func(t *testing.T) {
		node := parseMindMapMarkdown("  \n\n")
		if node.ID != "root" || len(node.Children) != 0 {
			t.Fatalf("expected root-only tree, got %+v", node)
		}
	})
}

func TestParseMindMapMarkdownTree(t *testing.T) {
	t.Run("headings and nested lists build a real tree", func(t *testing.T) {
		node := parseMindMapMarkdown("# Title\n## Section A\n- point 1\n  - point 1.1\n## Section B\n")
		if node.ID != "Title" {
			t.Fatalf("expected single top heading as root, got %+v", node)
		}
		if len(node.Children) != 2 {
			t.Fatalf("expected 2 sections, got %+v", node.Children)
		}
		sectionA := node.Children[0]
		if sectionA.ID != "Section A" || len(sectionA.Children) != 1 {
			t.Fatalf("unexpected Section A: %+v", sectionA)
		}
		point := sectionA.Children[0]
		if point.ID != "point 1" || len(point.Children) != 1 || point.Children[0].ID != "point 1.1" {
			t.Fatalf("unexpected nested list parse: %+v", point)
		}
	})

	t.Run("multiple top headings keep an explicit root with children", func(t *testing.T) {
		node := parseMindMapMarkdown("# Alpha\n# Beta\n")
		if node.ID != "root" || len(node.Children) != 2 {
			t.Fatalf("expected root with 2 children, got %+v", node)
		}
	})
}
