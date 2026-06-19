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

package cli

import (
	"encoding/json"
	"fmt"
	"strings"
)

type ResponseIf interface {
	Type() string
	PrintOut()
	TimeCost() float64
	SetOutputFormat(format OutputFormat)
}

type CommonResponse struct {
	Code         int                      `json:"code"`
	Data         []map[string]interface{} `json:"data"`
	Message      string                   `json:"message"`
	Duration     float64
	OutputFormat OutputFormat
}

func (r *CommonResponse) Type() string {
	return "common"
}

func (r *CommonResponse) TimeCost() float64 {
	return r.Duration
}

func (r *CommonResponse) SetOutputFormat(format OutputFormat) {
	r.OutputFormat = format
}

func (r *CommonResponse) PrintOut() {
	if r.Code == 0 {
		PrintTableSimpleByFormat(r.Data, r.OutputFormat)
	} else {
		fmt.Println("ERROR")
		fmt.Printf("%d, %s\n", r.Code, r.Message)
	}
}

type ModelsResponse struct {
	Code         int                                 `json:"code"`
	Data         map[string][]map[string]interface{} `json:"data"`
	Message      string                              `json:"message"`
	Duration     float64
	OutputFormat OutputFormat
}

func (r *ModelsResponse) Type() string {
	return "models"
}

func (r *ModelsResponse) TimeCost() float64 {
	return r.Duration
}

func (r *ModelsResponse) SetOutputFormat(format OutputFormat) {
	r.OutputFormat = format
}

func (r *ModelsResponse) PrintOut() {
	if r.Code == 0 {
		models := r.Data["models"]
		PrintTableSimpleByFormat(models, r.OutputFormat)
	} else {
		fmt.Println("ERROR")
		fmt.Printf("%d, %s\n", r.Code, r.Message)
	}
}

type CommonDataResponse struct {
	Code         int                    `json:"code"`
	Data         map[string]interface{} `json:"data"`
	Message      string                 `json:"message"`
	Duration     float64
	OutputFormat OutputFormat
}

func (r *CommonDataResponse) Type() string {
	return "show"
}

func (r *CommonDataResponse) TimeCost() float64 {
	return r.Duration
}

func (r *CommonDataResponse) SetOutputFormat(format OutputFormat) {
	r.OutputFormat = format
}

func (r *CommonDataResponse) PrintOut() {
	if r.Code == 0 {
		table := make([]map[string]interface{}, 0)
		for key, value := range r.Data {
			elem := map[string]interface{}{
				"field": key,
				"value": value,
			}
			table = append(table, elem)
		}
		//table = append(table, r.Data)
		PrintTableSimpleByFormat(table, r.OutputFormat)
	} else {
		fmt.Println("ERROR")
		fmt.Printf("%d, %s\n", r.Code, r.Message)
	}
}

type ListDocumentsResponse struct {
	Code         int                    `json:"code"`
	Data         map[string]interface{} `json:"data"`
	Message      string                 `json:"message"`
	Duration     float64
	OutputFormat OutputFormat
}

func (r *ListDocumentsResponse) Type() string {
	return "list_documents"
}

func (r *ListDocumentsResponse) TimeCost() float64 {
	return r.Duration
}

func (r *ListDocumentsResponse) SetOutputFormat(format OutputFormat) {
	r.OutputFormat = format
}

func (r *ListDocumentsResponse) PrintOut() {
	if r.Code == 0 {
		total := r.Data["total"].(float64)
		fmt.Printf("Total: %0.0f\n", total)
		docs := r.Data["docs"].([]interface{})
		table := make([]map[string]interface{}, 0)
		for _, doc := range docs {
			table = append(table, doc.(map[string]interface{}))
		}
		PrintTableSimpleByFormat(table, r.OutputFormat)
	} else {
		fmt.Println("ERROR")
		fmt.Printf("%d, %s\n", r.Code, r.Message)
	}
}

type ChunkResponse struct {
	Code         int                    `json:"code"`
	Data         map[string]interface{} `json:"data"`
	Message      string                 `json:"message"`
	Duration     float64
	OutputFormat OutputFormat
}

func (r *ChunkResponse) Type() string {
	return "chunk"
}

func (r *ChunkResponse) TimeCost() float64 {
	return r.Duration
}

func (r *ChunkResponse) SetOutputFormat(format OutputFormat) {
	r.OutputFormat = format
}

func (r *ChunkResponse) PrintOut() {
	if r.Code == 0 {
		for k, v := range r.Data {
			fmt.Printf("%s: %v\n", k, v)
		}
	} else {
		fmt.Println("ERROR")
		fmt.Printf("%d, %s\n", r.Code, r.Message)
	}
}

type MetadataResponse struct {
	Code         int                    `json:"code"`
	Data         map[string]interface{} `json:"data"`
	Message      string                 `json:"message"`
	Duration     float64
	OutputFormat OutputFormat
}

func (r *MetadataResponse) Type() string {
	return "metadata"
}

func (r *MetadataResponse) TimeCost() float64 {
	return r.Duration
}

func (r *MetadataResponse) SetOutputFormat(format OutputFormat) {
	r.OutputFormat = format
}

func (r *MetadataResponse) PrintOut() {
	if r.Code == 0 {
		// Data is map[field]map[value][]doc_id - print flattened metadata
		if r.Data != nil {
			printFlattenedMetadata(r.Data, r.OutputFormat)
		}
	} else {
		fmt.Println("ERROR")
		fmt.Printf("%d, %s\n", r.Code, r.Message)
	}
}

func printFlattenedMetadata(data map[string]interface{}, format OutputFormat) {
	// Convert flattened metadata to table format
	// {field: {value: [doc_ids]}} -> [{field, value, document_ids}, ...]
	tableData := make([]map[string]interface{}, 0)
	for field, values := range data {
		valueMap, ok := values.(map[string]interface{})
		if !ok {
			continue
		}
		for value, docIDs := range valueMap {
			var docIDStr string
			switch v := docIDs.(type) {
			case []string:
				docIDStr = strings.Join(v, ", ")
			case []interface{}:
				docStrs := make([]string, 0, len(v))
				for _, d := range v {
					if s, ok := d.(string); ok {
						docStrs = append(docStrs, s)
					}
				}
				docIDStr = strings.Join(docStrs, ", ")
			default:
				docIDStr = fmt.Sprintf("%v", docIDs)
			}
			tableData = append(tableData, map[string]interface{}{
				"field":        field,
				"value":        value,
				"document_ids": docIDStr,
			})
		}
	}
	PrintTableSimpleByFormat(tableData, format)
}

type SimpleResponse struct {
	Code         int    `json:"code"`
	Message      string `json:"message"`
	Duration     float64
	OutputFormat OutputFormat
}

func (r *SimpleResponse) Type() string {
	return "simple"
}

func (r *SimpleResponse) TimeCost() float64 {
	return r.Duration
}

func (r *SimpleResponse) SetOutputFormat(format OutputFormat) {
	r.OutputFormat = format
}

func (r *SimpleResponse) PrintOut() {
	if r.Code == 0 {
		fmt.Println("SUCCESS")
	} else {
		fmt.Println("ERROR")
		fmt.Printf("%d, %s\n", r.Code, r.Message)
	}
}

type MessageResponse struct {
	Code         int    `json:"code"`
	Message      string `json:"message"`
	Duration     float64
	OutputFormat OutputFormat
}

func (r *MessageResponse) Type() string {
	return "message"
}

func (r *MessageResponse) TimeCost() float64 {
	return r.Duration
}

func (r *MessageResponse) SetOutputFormat(format OutputFormat) {
	r.OutputFormat = format
}

func (r *MessageResponse) PrintOut() {
	if r.Code == 0 {
		fmt.Println(r.Message)
	} else {
		fmt.Println("ERROR")
		fmt.Printf("%d, %s\n", r.Code, r.Message)
	}
}

type NonStreamResponse struct {
	Code             int    `json:"code"`
	ReasoningContent string `json:"reasoning_content"`
	Answer           string `json:"answer"`
	Message          string `json:"message"`
	Duration         float64
	OutputFormat     OutputFormat
}

func (r *NonStreamResponse) Type() string {
	return "non_stream_message"
}

func (r *NonStreamResponse) TimeCost() float64 {
	return r.Duration
}

func (r *NonStreamResponse) SetOutputFormat(format OutputFormat) {
	r.OutputFormat = format
}

func (r *NonStreamResponse) PrintOut() {
	if r.Code == 0 {
		if r.ReasoningContent != "" {
			fmt.Printf("Thinking: %s\n", r.ReasoningContent)
		}
		fmt.Printf("Answer: %s\n", r.Answer)
		fmt.Printf("Time: %f\n", r.Duration)
	} else {
		fmt.Println("ERROR")
		fmt.Printf("%d, %s\n", r.Code, r.Message)
	}
}

type StreamMessageResponse struct {
	Code         int    `json:"code"`
	Message      string `json:"message"`
	Duration     float64
	OutputFormat OutputFormat
}

func (r *StreamMessageResponse) Type() string {
	return "stream_message"
}

func (r *StreamMessageResponse) TimeCost() float64 {
	return r.Duration
}

func (r *StreamMessageResponse) SetOutputFormat(format OutputFormat) {
	r.OutputFormat = format
}

func (r *StreamMessageResponse) PrintOut() {
	if r.Code == 0 {
		fmt.Printf("Time: %f\n", r.Duration)
	} else {
		fmt.Println("ERROR")
		fmt.Printf("%d, %s\n", r.Code, r.Message)
	}
}

type RegisterResponse struct {
	Code         int    `json:"code"`
	Message      string `json:"message"`
	Duration     float64
	OutputFormat OutputFormat
}

func (r *RegisterResponse) Type() string {
	return "register"
}

func (r *RegisterResponse) TimeCost() float64 {
	return r.Duration
}

func (r *RegisterResponse) SetOutputFormat(format OutputFormat) {
	r.OutputFormat = format
}

func (r *RegisterResponse) PrintOut() {
	if r.Code == 0 {
		fmt.Println("Register successfully")
	} else {
		fmt.Println("ERROR")
		fmt.Printf("%d, %s\n", r.Code, r.Message)
	}
}

type BenchmarkResponse struct {
	Code         int     `json:"code"`
	Duration     float64 `json:"duration"`
	SuccessCount int     `json:"success_count"`
	FailureCount int     `json:"failure_count"`
	Concurrency  int
	OutputFormat OutputFormat
}

func (r *BenchmarkResponse) Type() string {
	return "benchmark"
}

func (r *BenchmarkResponse) SetOutputFormat(format OutputFormat) {
	r.OutputFormat = format
}

func (r *BenchmarkResponse) PrintOut() {
	if r.Code != 0 {
		fmt.Printf("ERROR, Code: %d\n", r.Code)
		return
	}

	iterations := r.SuccessCount + r.FailureCount
	if r.Concurrency == 1 {
		if iterations == 1 {
			fmt.Printf("Latency: %fs\n", r.Duration)
		} else {
			fmt.Printf("Latency: %fs, QPS: %.1f, SUCCESS: %d, FAILURE: %d\n", r.Duration, float64(iterations)/r.Duration, r.SuccessCount, r.FailureCount)
		}
	} else {
		fmt.Printf("Concurrency: %d, Latency: %fs, QPS: %.1f, SUCCESS: %d, FAILURE: %d\n", r.Concurrency, r.Duration, float64(iterations)/r.Duration, r.SuccessCount, r.FailureCount)
	}
}

func (r *BenchmarkResponse) TimeCost() float64 {
	return r.Duration
}

type KeyValueResponse struct {
	Code         int    `json:"code"`
	Key          string `json:"key"`
	Value        string `json:"data"`
	Duration     float64
	OutputFormat OutputFormat
}

func (r *KeyValueResponse) Type() string {
	return "data"
}

func (r *KeyValueResponse) TimeCost() float64 {
	return r.Duration
}

func (r *KeyValueResponse) SetOutputFormat(format OutputFormat) {
	r.OutputFormat = format
}

func (r *KeyValueResponse) PrintOut() {
	if r.Code == 0 {
		table := make([]map[string]interface{}, 0)
		// insert r.key and r.value into table
		table = append(table, map[string]interface{}{
			"key":   r.Key,
			"value": r.Value,
		})
		PrintTableSimpleByFormat(table, r.OutputFormat)
	} else {
		fmt.Println("ERROR")
		fmt.Printf("%d\n", r.Code)
	}
}

type EmbeddingData struct {
	Index     int       `json:"index"`
	Embedding []float64 `json:"embedding"`
}

type EmbeddingsResponse struct {
	Code         int             `json:"code"`
	Data         []EmbeddingData `json:"data"`
	Message      string          `json:"message"`
	Duration     float64
	OutputFormat OutputFormat
}

func (r *EmbeddingsResponse) Type() string {
	return "common"
}

func (r *EmbeddingsResponse) TimeCost() float64 {
	return r.Duration
}

func (r *EmbeddingsResponse) SetOutputFormat(format OutputFormat) {
	r.OutputFormat = format
}

func (r *EmbeddingsResponse) PrintOut() {
	var data []map[string]interface{}
	for _, embedding := range r.Data {
		data = append(data, map[string]interface{}{
			"index":     formatValue(embedding.Index),
			"dimension": len(embedding.Embedding),
		})
	}

	if r.Code == 0 {
		PrintTableSimpleByFormat(data, r.OutputFormat)
	} else {
		fmt.Println("ERROR")
		fmt.Printf("%d, %s\n", r.Code, r.Message)
	}
}

type SegmentResponse struct {
	Segments []map[string]interface{} `json:"segments"`
}

type TaskResponse struct {
	Code         int                    `json:"code"`
	Data         map[string]interface{} `json:"data"`
	Message      string                 `json:"message"`
	Duration     float64
	OutputFormat OutputFormat
}

func (r *TaskResponse) Type() string {
	return "task"
}

func (r *TaskResponse) TimeCost() float64 {
	return r.Duration
}

func (r *TaskResponse) SetOutputFormat(format OutputFormat) {
	r.OutputFormat = format
}

func (r *TaskResponse) PrintOut() {
	if r.Code == 0 {
		segmentsRaw := r.Data["segments"].([]interface{})
		segments := make([]map[string]interface{}, len(segmentsRaw))
		for i, v := range segmentsRaw {
			segments[i] = v.(map[string]interface{})
		}
		PrintTableSimpleByFormat(segments, r.OutputFormat)
	} else {
		fmt.Println("ERROR")
		fmt.Printf("%d, %s\n", r.Code, r.Message)
	}
}

type ExplainResponse struct {
	Code         int    `json:"code"`
	Message      string `json:"message"`
	Duration     float64
	OutputFormat OutputFormat
}

func (r *ExplainResponse) Type() string {
	return "explain"
}

func (r *ExplainResponse) TimeCost() float64 {
	return r.Duration
}

func (r *ExplainResponse) SetOutputFormat(format OutputFormat) {
	r.OutputFormat = format
}

func (r *ExplainResponse) PrintOut() {
	if r.Code == 0 {
		fmt.Printf("\n%s\n", r.Message)
	} else {
		fmt.Printf("ERROR %d\n", r.Code)
	}
}

// FileSystemResponse wraps the raw text output from executeFilesystem().
type FileSystemResponse struct {
	Output       string
	Duration     float64
	OutputFormat OutputFormat
}

func (r *FileSystemResponse) Type() string                        { return "filesystem" }
func (r *FileSystemResponse) TimeCost() float64                   { return r.Duration }
func (r *FileSystemResponse) SetOutputFormat(format OutputFormat) { r.OutputFormat = format }
func (r *FileSystemResponse) PrintOut() {
	fmt.Print(r.Output)
}

type OpenAIChatResponse struct {
	Code         int             `json:"code,omitempty"`
	Data         *openAIChatData `json:"data,omitempty"`
	Message      string          `json:"message,omitempty"`
	Duration     float64         `json:"-"`
	OutputFormat OutputFormat    `json:"-"`
	// Reasoning from the model's chain-of-thought.
	Reasoning string `json:"-"`
	// streamed skips the "Answer:" line in PrintOut to avoid duplication.
	streamed bool
	// raw HTTP body for the "raw" output format.
	raw []byte
}

type openAIChatData struct {
	ID               string             `json:"id"`
	Object           string             `json:"object"`
	Created          int64              `json:"created"`
	Model            string             `json:"model"`
	Choices          []openAIChatChoice `json:"choices"`
	Usage            *openAIChatUsage   `json:"usage"`
	ReferencePayload json.RawMessage    `json:"reference,omitempty"`
}

type openAIChatChoice struct {
	Index        int               `json:"index"`
	FinishReason string            `json:"finish_reason"`
	Logprobs     interface{}       `json:"logprobs"`
	Message      openAIChatMessage `json:"message"`
}

type openAIChatMessage struct {
	Role             string          `json:"role"`
	Content          string          `json:"content"`
	Reference        json.RawMessage `json:"reference,omitempty"`
	ReasoningContent string          `json:"reasoning_content,omitempty"`
}

type openAIChatUsage struct {
	PromptTokens            int `json:"prompt_tokens"`
	CompletionTokens        int `json:"completion_tokens"`
	TotalTokens             int `json:"total_tokens"`
	CompletionTokensDetails *struct {
		ReasoningTokens          int `json:"reasoning_tokens"`
		AcceptedPredictionTokens int `json:"accepted_prediction_tokens"`
		RejectedPredictionTokens int `json:"rejected_prediction_tokens"`
	} `json:"completion_tokens_details"`
}

func (r *OpenAIChatResponse) Type() string                   { return "openai_chat" }
func (r *OpenAIChatResponse) TimeCost() float64              { return r.Duration }
func (r *OpenAIChatResponse) SetOutputFormat(f OutputFormat) { r.OutputFormat = f }
func (r *OpenAIChatResponse) Raw() []byte                    { return r.raw }

func (r *OpenAIChatResponse) SetRaw(b []byte) { r.raw = b }

func (r *OpenAIChatResponse) Content() string {
	if r.Data == nil || len(r.Data.Choices) == 0 {
		return ""
	}
	return r.Data.Choices[0].Message.Content
}

func (r *OpenAIChatResponse) Model() string {
	if r.Data == nil {
		return ""
	}
	return r.Data.Model
}

func (r *OpenAIChatResponse) Usage() *openAIChatUsage {
	if r.Data == nil {
		return nil
	}
	return r.Data.Usage
}

func (r *OpenAIChatResponse) PrintOut() {
	if r.OutputFormat == "raw" && r.raw != nil {
		fmt.Println(string(r.raw))
		return
	}
	if r.Code != 0 {
		fmt.Println("ERROR")
		fmt.Printf("%d, %s\n", r.Code, r.Message)
		return
	}
	if r.Data == nil {
		fmt.Println("(no data)")
		return
	}
	if !r.streamed {
		if r.Reasoning != "" {
			fmt.Printf("Thinking: %s\n", r.Reasoning)
		}
		if content := r.Content(); content != "" {
			fmt.Printf("Answer: %s\n", content)
		}
	}

	// Print reference chunks and their document_metadata when available.
	// Reference can be on the data-level or on the message-level.
	refRaw := r.Data.ReferencePayload
	if len(refRaw) == 0 && len(r.Data.Choices) > 0 {
		refRaw = r.Data.Choices[0].Message.Reference
	}
	if len(refRaw) > 0 {
		printReferenceChunks(refRaw)
	}

	fmt.Printf("Time: %f\n", r.Duration)
}

// printReferenceChunks parses a reference JSON blob and prints each chunk
// together with its document_metadata (if any).
func printReferenceChunks(raw json.RawMessage) {
	var chunks []map[string]interface{}

	// direct array: [...]
	if err := json.Unmarshal(raw, &chunks); err != nil {
		// object with "chunks" key: {"chunks": [...], "doc_aggs": [...]}
		var ref struct {
			Chunks []map[string]interface{} `json:"chunks"`
		}
		if err2 := json.Unmarshal(raw, &ref); err2 != nil || len(ref.Chunks) == 0 {
			return
		}
		chunks = ref.Chunks
	}
	if len(chunks) == 0 {
		return
	}

	fmt.Println("Reference:")
	for i, chunk := range chunks {
		id := chunkID(chunk)
		content := chunkContent(chunk)
		docName := chunkDocName(chunk)
		fmt.Printf("  [ID:%d] id=%s content=%q", i, id, truncateStr(content, 120))
		if docName != "" {
			fmt.Printf(" doc=%s", docName)
		}
		fmt.Println()

		// Print document_metadata if present.
		if meta, ok := chunk["document_metadata"].(map[string]interface{}); ok && len(meta) > 0 {
			for k, v := range meta {
				fmt.Printf("       metadata.%s = %v\n", k, v)
			}
		}
	}
}

func chunkID(c map[string]interface{}) string {
	for _, key := range []string{"chunk_id", "id"} {
		if v, ok := c[key]; ok {
			return fmt.Sprint(v)
		}
	}
	return "-"
}

func chunkContent(c map[string]interface{}) string {
	if v, ok := c["content"]; ok {
		s := fmt.Sprint(v)
		return strings.TrimSpace(s)
	}
	return ""
}

func chunkDocName(c map[string]interface{}) string {
	if v, ok := c["document_name"]; ok {
		return fmt.Sprint(v)
	}
	if v, ok := c["doc_name"]; ok {
		return fmt.Sprint(v)
	}
	return ""
}

func truncateStr(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}
