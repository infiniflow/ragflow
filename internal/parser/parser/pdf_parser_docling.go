package parser

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	models "ragflow/internal/entity/models"
)

type doclingChunk struct {
	Text  string `json:"text"`
	Chunk *struct {
		Text string `json:"text"`
	} `json:"chunk"`
}

type doclingDocument struct {
	MDContent   string         `json:"md_content"`
	TextContent string         `json:"text_content"`
	JSONContent map[string]any `json:"json_content"`
}

type doclingResult struct {
	Document *doclingDocument `json:"document"`
	Result   *doclingDocument `json:"result"`
}

type doclingResponse struct {
	Document  *doclingDocument  `json:"document"`
	Documents []doclingDocument `json:"documents"`
	Results   []doclingResult   `json:"results"`
}

func parsePDFWithDocling(filename string, data []byte, parser *PDFParser) ParseResult {
	if len(data) == 0 {
		return emptyPDFResult(filename)
	}
	serverURL := strings.TrimSpace(parser.DoclingServerURL)
	if serverURL == "" {
		serverURL = strings.TrimSpace(os.Getenv("DOCLING_SERVER_URL"))
	}
	if serverURL == "" {
		return ParseResult{Err: fmt.Errorf("parser: Docling requires docling_server_url or DOCLING_SERVER_URL")}
	}
	apiKey := strings.TrimSpace(parser.DoclingAPIKey)
	if apiKey == "" {
		apiKey = strings.TrimSpace(os.Getenv("DOCLING_API_KEY"))
	}

	baseURL := strings.TrimRight(serverURL, "/")
	auth := ""
	if apiKey != "" {
		auth = "Bearer " + apiKey
	}
	encoded := base64.StdEncoding.EncodeToString(data)
	payloads := []struct {
		endpoint string
		body     func() map[string]any
		chunked  bool
	}{
		{
			endpoint: "/v1/convert/source",
			chunked:  true,
			body:     func() map[string]any { return doclingChunkedPayload(filename, encoded, false) },
		},
		{
			endpoint: "/v1alpha/convert/source",
			chunked:  true,
			body:     func() map[string]any { return doclingChunkedPayload(filename, encoded, true) },
		},
		{
			endpoint: "/v1/convert/source",
			chunked:  false,
			body:     func() map[string]any { return doclingStandardPayload(filename, encoded, false) },
		},
		{
			endpoint: "/v1alpha/convert/source",
			chunked:  false,
			body:     func() map[string]any { return doclingStandardPayload(filename, encoded, true) },
		},
	}

	var lastErr error
	for _, candidate := range payloads {
		url := baseURL + candidate.endpoint
		resp, err := models.PostJSONRequest(context.Background(), models.NewDriverHTTPClient(), url, auth, candidate.body())
		if err != nil {
			lastErr = fmt.Errorf("%s: %w", candidate.endpoint, err)
			continue
		}
		body, readErr := func() ([]byte, error) {
			defer resp.Body.Close()
			return io.ReadAll(resp.Body)
		}()
		if readErr != nil {
			lastErr = fmt.Errorf("%s: read response: %w", candidate.endpoint, readErr)
			continue
		}
		if resp.StatusCode >= 300 {
			if candidate.chunked {
				lastErr = fmt.Errorf("%s: HTTP %d", candidate.endpoint, resp.StatusCode)
				continue
			}
			lastErr = fmt.Errorf("%s: HTTP %d %s", candidate.endpoint, resp.StatusCode, string(body))
			continue
		}
		if candidate.chunked {
			if res, ok := parseDoclingChunkedResult(filename, body, parser.OutputFormat); ok {
				return res
			}
			lastErr = fmt.Errorf("%s: chunked response contained no usable text", candidate.endpoint)
			continue
		}
		if res, ok := parseDoclingStandardResult(filename, body, parser.OutputFormat); ok {
			return res
		}
		lastErr = fmt.Errorf("%s: standard response contained no parsed document", candidate.endpoint)
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("Docling remote convert failed")
	}
	return ParseResult{Err: fmt.Errorf("parser: Docling convert: %w", lastErr)}
}

func doclingStandardPayload(filename string, encoded string, alpha bool) map[string]any {
	source := map[string]any{"filename": filename, "base64_string": encoded}
	options := map[string]any{"from_formats": []string{"pdf"}, "to_formats": []string{"json", "md", "text"}}
	if alpha {
		return map[string]any{
			"options":      options,
			"file_sources": []map[string]any{source},
		}
	}
	source["kind"] = "file"
	return map[string]any{
		"options": options,
		"sources": []map[string]any{source},
	}
}

func doclingChunkedPayload(filename string, encoded string, alpha bool) map[string]any {
	payload := doclingStandardPayload(filename, encoded, alpha)
	payload["options"] = map[string]any{
		"from_formats": []string{"pdf"},
		"to_formats":   []string{"json", "md", "text"},
		"do_chunking":  true,
		"chunking_options": map[string]any{
			"max_tokens": 512,
			"overlap":    50,
			"tokenizer":  "sentencepiece",
		},
	}
	return payload
}

func parseDoclingChunkedResult(filename string, body []byte, outputFormat string) (ParseResult, bool) {
	var chunks []doclingChunk
	if err := json.Unmarshal(body, &chunks); err != nil {
		var wrapped struct {
			Results []doclingChunk `json:"results"`
		}
		if err := json.Unmarshal(body, &wrapped); err != nil {
			return ParseResult{}, false
		}
		chunks = wrapped.Results
	}
	texts := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		text := strings.TrimSpace(chunk.Text)
		if text == "" && chunk.Chunk != nil {
			text = strings.TrimSpace(chunk.Chunk.Text)
		}
		if text != "" {
			texts = append(texts, text)
		}
	}
	if len(texts) == 0 {
		return ParseResult{}, false
	}
	pageCount := 0
	if len(texts) > 0 {
		pageCount = len(texts)
	}
	return doclingTextsToResult(filename, texts, outputFormat, pageCount), true
}

func parseDoclingStandardResult(filename string, body []byte, outputFormat string) (ParseResult, bool) {
	var payload doclingResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return ParseResult{}, false
	}
	docs := make([]doclingDocument, 0, 1+len(payload.Documents)+len(payload.Results))
	if payload.Document != nil {
		docs = append(docs, *payload.Document)
	}
	docs = append(docs, payload.Documents...)
	for _, result := range payload.Results {
		switch {
		case result.Document != nil:
			docs = append(docs, *result.Document)
		case result.Result != nil:
			docs = append(docs, *result.Result)
		}
	}
	for _, doc := range docs {
		if md := strings.TrimSpace(doc.MDContent); md != "" {
			return parseMinerUMarkdownResult(filename, md, outputFormat, len(docs)), true
		}
		if txt := strings.TrimSpace(doc.TextContent); txt != "" {
			return doclingTextsToResult(filename, []string{txt}, outputFormat, len(docs)), true
		}
		if md, _ := doc.JSONContent["md_content"].(string); strings.TrimSpace(md) != "" {
			return parseMinerUMarkdownResult(filename, md, outputFormat, len(docs)), true
		}
	}
	return ParseResult{}, false
}

func doclingTextsToResult(filename string, texts []string, outputFormat string, pageCount int) ParseResult {
	fileMeta := pdfFileMeta(filename, pageCount)
	switch strings.ToLower(strings.TrimSpace(outputFormat)) {
	case "", "json":
		items := make([]map[string]any, 0, len(texts))
		for _, text := range texts {
			items = append(items, map[string]any{
				"text":         text,
				"doc_type_kwd": "text",
			})
		}
		return ParseResult{
			OutputFormat: "json",
			File:         fileMeta,
			JSON:         items,
		}
	case "markdown":
		return ParseResult{
			OutputFormat: "markdown",
			File:         fileMeta,
			Markdown:     strings.Join(texts, "\n\n"),
		}
	default:
		return ParseResult{Err: fmt.Errorf("parser: unsupported PDF output_format %q", outputFormat)}
	}
}
