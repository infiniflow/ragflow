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

package chunk

import (
	"fmt"
	"regexp"
	"strings"

	"ragflow/internal/common"
)

// maxReportedImageSource bounds the image source echoed back in the rejection
// message: a data URI is megabytes long and would neither fit a response nor
// help the caller.
const maxReportedImageSource = 96

// maxEditedChunkLength bounds an edited body. A chunk is a retrieval unit, not
// a document: past this size the embedding window truncates nearly all of it,
// the engine row grows without matching retrieval value, and the editor is
// almost certainly holding the wrong text (a whole page pasted into a chunk).
const maxEditedChunkLength = 200000

// validateChunkContentLength rejects a body too large to be a chunk.
func validateChunkContentLength(content string) error {
	if len(content) > maxEditedChunkLength {
		return updateChunkError{
			code:    common.CodeArgumentError,
			message: fmt.Sprintf("`content` exceeds %d bytes; split it into several chunks", maxEditedChunkLength),
		}
	}
	return nil
}

// markdownImageRefPattern captures the destination of a Markdown image, with
// the optional angle brackets and whitespace CommonMark allows around it:
// ![alt](<http://x/y.png>), ![alt]( http://x/y.png "title").
var markdownImageRefPattern = regexp.MustCompile("!\\[[^\\]]*\\]\\(\\s*<?([^)\\s>]*)")

// htmlImageSrcPattern captures the src of an <img> tag, quoted or not.
var htmlImageSrcPattern = regexp.MustCompile("(?i)<img\\b[^>]*?\\bsrc\\s*=\\s*[\"']?([^\"'\\s>]*)")

// chunkStoredContent reads the text of a chunk row as the engine returns it,
// preferring the indexed column and falling back to the older raw one.
func chunkStoredContent(row map[string]interface{}) string {
	if content, ok := row["content_with_weight"].(string); ok && content != "" {
		return content
	}
	content, _ := row["content"].(string)
	return content
}

// chunkContentChanged reports whether an edit replaced the text the chunk is
// indexed and embedded by. An update map without content leaves it untouched,
// so every derived field (tokens, vector, positions) keeps its value.
func chunkContentChanged(previous, update map[string]interface{}) bool {
	next, ok := update["content_with_weight"].(string)
	if !ok {
		return false
	}
	return chunkStoredContent(previous) != next
}

// externalImageSources returns the image sources of a content string that point
// outside the document: remote URLs and inline data URIs.
//
// An ingested chunk never carries one. The parser keeps only local references
// in text (shouldPreserveImageReference), a resolved image is uploaded and
// replaced by its img_id, and the retrieval/render path trusts chunk content —
// so a remote source written through the edit API would make every reader's
// browser fetch a URL the platform never stored, vetted or authorized.
func externalImageSources(content string) []string {
	var sources []string
	for _, match := range markdownImageRefPattern.FindAllStringSubmatch(content, -1) {
		if isExternalImageSource(match[1]) {
			sources = append(sources, strings.TrimSpace(match[1]))
		}
	}
	for _, match := range htmlImageSrcPattern.FindAllStringSubmatch(content, -1) {
		if isExternalImageSource(match[1]) {
			sources = append(sources, strings.TrimSpace(match[1]))
		}
	}
	return sources
}

// isExternalImageSource reports whether an image source is remote or inline.
func isExternalImageSource(source string) bool {
	lower := strings.ToLower(strings.TrimSpace(source))
	return strings.HasPrefix(lower, "http://") ||
		strings.HasPrefix(lower, "https://") ||
		strings.HasPrefix(lower, "data:")
}

// validateEditedContent rejects an edit whose content introduces an external
// image source it did not already hold.
//
// Sources the stored content already had are tolerated: chunks ingested by an
// older parser version may still carry one, and refusing to save an unrelated
// edit of such a chunk would trap it forever. Only a source the edit adds is
// refused, with the reason spelled out — the caller has to upload the image
// through `image_base64` instead, which stores it in the dataset's bucket and
// indexes it as the chunk's img_id.
func validateEditedContent(previousContent, newContent string) error {
	alreadyPresent := make(map[string]bool)
	for _, source := range externalImageSources(previousContent) {
		alreadyPresent[source] = true
	}
	for _, source := range externalImageSources(newContent) {
		if alreadyPresent[source] {
			continue
		}
		return updateChunkError{
			code: common.CodeArgumentError,
			message: fmt.Sprintf(
				"Chunk content may not introduce an external image (%s). Upload the image with `image_base64` so the platform stores and indexes it.",
				shortImageSource(source)),
		}
	}
	return nil
}

// shortImageSource trims a source for the error message.
func shortImageSource(source string) string {
	if len(source) <= maxReportedImageSource {
		return source
	}
	return source[:maxReportedImageSource] + "…"
}
