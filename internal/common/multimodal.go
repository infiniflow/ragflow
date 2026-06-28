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

package common

import (
	"encoding/base64"
	"fmt"
	"regexp"
	"strings"
)

// ContentPart is the internal representation of a multimodal content
// fragment, decoupled from any provider's wire format. Drivers consume
// the result of RenderContentPartsForFactory to produce their per-
// provider JSON.
type ContentPart struct {
	// Type is one of: "text", "image_url", "image", "inline_data".
	Type string
	// Text is set when Type == "text".
	Text string
	// ImageURL is set when Type == "image_url" (OpenAI shape).
	ImageURL *ImageURL
	// Source is set when Type == "image" (Anthropic) or
	// Type == "inline_data" (Gemini).
	Source *ContentSource
}

// ImageURL is the OpenAI-shaped image reference.
type ImageURL struct {
	URL string `json:"url"`
}

// ContentSource is the Anthropic / Gemini source payload.
type ContentSource struct {
	Type      string `json:"type"`           // "base64" or "url"
	MediaType string `json:"media_type"`     // e.g. "image/png"
	Data      string `json:"data,omitempty"` // base64 payload
	URL       string `json:"url,omitempty"`
}

// dataURIRE detects a "data:<mediatype>;base64,<data>" string.
var dataURIRE = regexp.MustCompile(`^data:([^;,]+)(?:;base64)?,(.*)$`)

// parseDataURIOrB64 accepts a string and classifies it as a data URI,
// a plain https URL, or a raw base64 payload
func parseDataURIOrB64(s string) (ContentSource, error) {
	if s == "" {
		return ContentSource{}, fmt.Errorf("empty image source")
	}
	if m := dataURIRE.FindStringSubmatch(s); m != nil {
		mediaType := strings.TrimSpace(m[1])
		if mediaType == "" {
			mediaType = "image/png"
		}
		return ContentSource{
			Type:      "base64",
			MediaType: mediaType,
			Data:      m[2],
		}, nil
	}
	if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") {
		return ContentSource{Type: "url", URL: s}, nil
	}
	// Assume raw base64 (no data URI, no http scheme). The provider
	// uses the file extension or a content-type hint from the call site
	// to pick the right media type; we default to image/png.
	if _, err := base64.StdEncoding.DecodeString(s); err != nil {
		return ContentSource{}, fmt.Errorf("not a valid data URI, URL, or base64: %w", err)
	}
	return ContentSource{
		Type:      "base64",
		MediaType: "image/png",
		Data:      s,
	}, nil
}

// normalizeTextFromContent extracts a single text string from a content
// value that may be a string, []map[string]interface{}, or []interface{}.
func normalizeTextFromContent(content interface{}) string {
	switch v := content.(type) {
	case string:
		return v
	case []map[string]interface{}:
		var parts []string
		for _, p := range v {
			if t, ok := p["type"].(string); ok && (t == "text" || t == "input_text") {
				if txt, ok := p["text"].(string); ok {
					parts = append(parts, txt)
				}
			} else if txt, ok := p["text"]; ok {
				// Fallback: "text" key present even though type didn't match.
				switch tv := txt.(type) {
				case string:
					parts = append(parts, tv)
				case float64:
					parts = append(parts, fmt.Sprintf("%v", tv))
				case int:
					parts = append(parts, fmt.Sprintf("%v", tv))
				}
			}
		}
		return strings.Join(parts, "\n")
	case []interface{}:
		var parts []string
		for _, item := range v {
			switch p := item.(type) {
			case map[string]interface{}:
				if t, ok := p["type"].(string); ok && (t == "text" || t == "input_text") {
					if txt, ok := p["text"].(string); ok {
						parts = append(parts, txt)
					}
				} else if txt, ok := p["text"]; ok {
					// Fallback: "text" key present even though type didn't match.
					switch tv := txt.(type) {
					case string:
						parts = append(parts, tv)
					case float64:
						parts = append(parts, fmt.Sprintf("%v", tv))
					case int:
						parts = append(parts, fmt.Sprintf("%v", tv))
					}
				}
			case string:
				parts = append(parts, p)
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}

// extractImageURLs pulls image_url values out of a content value. Used
// by ConvertLastUserMsgToMultimodal to assemble the ContentPart slice.
func extractImageURLs(content interface{}) []string {
	var urls []string
	process := func(p map[string]interface{}) {
		t, _ := p["type"].(string)
		if t == "image_url" {
			if u, ok := p["image_url"].(string); ok && u != "" {
				urls = append(urls, u)
			} else if obj, ok := p["image_url"].(map[string]interface{}); ok {
				if u, ok := obj["url"].(string); ok && u != "" {
					urls = append(urls, u)
				}
			}
		}
	}
	switch v := content.(type) {
	case []map[string]interface{}:
		for _, p := range v {
			process(p)
		}
	case []interface{}:
		for _, item := range v {
			if p, ok := item.(map[string]interface{}); ok {
				process(p)
			}
		}
	}
	return urls
}

// ConvertLastUserMsgToMultimodal converts a user message whose content
// is a multimodal parts array into a message whose content is a
// driver-ready content-parts value, dispatched by `factory` (provider
// name).
//
// `imageAttachments` is an additional list of image URLs from the
// `messages[-1]["files"]` array.
// When non-empty, each URL is added to the content as an image
// part regardless of the original message content.
//
// factory values supported:
//   - "gemini"     → {"text": ...} / {"inline_data": {...}}
//   - "anthropic"  → {"type": "text", ...} / {"type": "image", "source": {...}}
//   - default      → {"type": "text", ...} / {"type": "image_url", "image_url": {...}}
//
// If the message is already a string, it is returned unchanged.
// If the message has no image parts and `imageAttachments` is empty,
// the text is returned as a string for compatibility with providers
// that don't accept content arrays.
func ConvertLastUserMsgToMultimodal(msg map[string]interface{}, imageAttachments []string, factory string) (map[string]interface{}, error) {
	if msg == nil {
		return nil, fmt.Errorf("nil message")
	}
	originalContent, ok := msg["content"]
	if !ok {
		return msg, nil
	}
	// If the content is already a plain string and there are no
	// imageAttachments to add, leave it alone.
	if _, isString := originalContent.(string); isString && len(imageAttachments) == 0 {
		return msg, nil
	}

	// Combine images from the content array and from imageAttachments
	// (the `files` array on the last user message).
	// Order: content-array images first, then files-array images.
	textPart := normalizeTextFromContent(originalContent)
	imageURLs := extractImageURLs(originalContent)
	allImageURLs := append(imageURLs, imageAttachments...)
	if len(allImageURLs) == 0 {
		// No images — collapse to a string for compatibility.
		out := make(map[string]interface{}, len(msg))
		for k, v := range msg {
			out[k] = v
		}
		out["content"] = textPart
		return out, nil
	}

	// Build ContentPart slice.
	parts := make([]ContentPart, 0, 1+len(allImageURLs))
	if textPart != "" {
		parts = append(parts, ContentPart{Type: "text", Text: textPart})
	}
	for _, u := range allImageURLs {
		src, err := parseDataURIOrB64(u)
		if err != nil {
			return nil, fmt.Errorf("image_url %q: %w", u, err)
		}
		// OpenAI / default: pass the raw URL through (provider accepts
		// both data: and http(s):). Anthropic / Gemini need a Source.
		if factory == "anthropic" || factory == "gemini" {
			parts = append(parts, ContentPart{
				Type:   pickImageType(factory),
				Source: &src,
			})
		} else {
			parts = append(parts, ContentPart{
				Type:     "image_url",
				ImageURL: &ImageURL{URL: u},
			})
		}
	}

	// Render to the driver's wire format.
	rendered, err := RenderContentPartsForFactory(parts, factory)
	if err != nil {
		return nil, err
	}
	out := make(map[string]interface{}, len(msg))
	for k, v := range msg {
		out[k] = v
	}
	out["content"] = rendered
	return out, nil
}

func pickImageType(factory string) string {
	if factory == "gemini" {
		return "inline_data"
	}
	return "image"
}

// RenderContentPartsForFactory converts internal ContentPart values
// into the per-provider JSON wire format:
//
//   - gemini:    [{"text": ...}, {"inline_data": {"mime_type": ..., "data": ...}}]
//   - anthropic: [{"type": "text", "text": ...}, {"type": "image", "source": {...}}]
//   - default:   [{"type": "text", "text": ...}, {"type": "image_url", "image_url": {"url": ...}}]
//
// The return value is suitable for direct assignment to a Message's
// `Content` field (`interface{}`).
func RenderContentPartsForFactory(parts []ContentPart, factory string) (interface{}, error) {
	factory = strings.ToLower(factory)
	switch factory {
	case "gemini":
		out := make([]map[string]interface{}, 0, len(parts))
		for _, p := range parts {
			switch p.Type {
			case "text":
				out = append(out, map[string]interface{}{"text": p.Text})
			case "image", "inline_data":
				if p.Source == nil {
					return nil, fmt.Errorf("gemini image part missing source")
				}
				if p.Source.Type == "url" {
					out = append(out, map[string]interface{}{
						"file_data": map[string]interface{}{
							"file_uri":  p.Source.URL,
							"mime_type": p.Source.MediaType,
						},
					})
				} else {
					out = append(out, map[string]interface{}{
						"inline_data": map[string]interface{}{
							"mime_type": p.Source.MediaType,
							"data":      p.Source.Data,
						},
					})
				}
			}
		}
		return out, nil
	case "anthropic":
		out := make([]map[string]interface{}, 0, len(parts))
		for _, p := range parts {
			switch p.Type {
			case "text":
				out = append(out, map[string]interface{}{
					"type": "text",
					"text": p.Text,
				})
			case "image":
				if p.Source == nil {
					return nil, fmt.Errorf("anthropic image part missing source")
				}
				out = append(out, map[string]interface{}{
					"type":   "image",
					"source": p.Source,
				})
			}
		}
		return out, nil
	default:
		// OpenAI-compatible.
		out := make([]map[string]interface{}, 0, len(parts))
		for _, p := range parts {
			switch p.Type {
			case "text":
				out = append(out, map[string]interface{}{
					"type": "text",
					"text": p.Text,
				})
			case "image_url":
				if p.ImageURL == nil {
					return nil, fmt.Errorf("openai image_url part missing URL")
				}
				out = append(out, map[string]interface{}{
					"type":      "image_url",
					"image_url": p.ImageURL,
				})
			}
		}
		return out, nil
	}
}
