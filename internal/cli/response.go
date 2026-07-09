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

func HandleCommonResponse(response *Response, command string) (ResponseIf, error) {
	if response.StatusCode != 200 {
		return nil, fmt.Errorf("failed to %s: HTTP %d, body: %s", command, response.StatusCode, string(response.Body))
	}

	var result CommonResponse
	if err := json.Unmarshal(response.Body, &result); err != nil {
		return nil, fmt.Errorf("%s failed: invalid JSON (%w)", command, err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}

	result.Duration = response.Duration
	return &result, nil
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

func (r *CommonDataResponse) orderedMetricTable() []map[string]interface{} {
	table := make([]map[string]interface{}, 0)
	for key, value := range r.Data {
		table = append(table, map[string]interface{}{
			"Metric": key,
			"Value":  value,
		})
	}
	return table
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

func HandleCommonDataResponse(response *Response, command string) (ResponseIf, error) {
	if response.StatusCode != 200 {
		return nil, fmt.Errorf("failed to %s: HTTP %d, body: %s", command, response.StatusCode, string(response.Body))
	}

	var result CommonDataResponse
	if err := json.Unmarshal(response.Body, &result); err != nil {
		return nil, fmt.Errorf("%s failed: invalid JSON (%w)", command, err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}

	result.Duration = response.Duration
	return &result, nil
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

type ListAgentsResponse struct {
	Code         int                    `json:"code"`
	Data         map[string]interface{} `json:"data"`
	Message      string                 `json:"message"`
	Duration     float64
	OutputFormat OutputFormat
}

func (r *ListAgentsResponse) Type() string {
	return "list_agents"
}

func (r *ListAgentsResponse) TimeCost() float64 {
	return r.Duration
}

func (r *ListAgentsResponse) SetOutputFormat(format OutputFormat) {
	r.OutputFormat = format
}

func (r *ListAgentsResponse) PrintOut() {
	if r.Code == 0 {
		total := r.Data["total"].(float64)
		fmt.Printf("Total: %0.0f\n", total)
		docs := r.Data["canvas"].([]interface{})
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

type ListChatsResponse struct {
	Code         int                    `json:"code"`
	Data         map[string]interface{} `json:"data"`
	Message      string                 `json:"message"`
	Duration     float64
	OutputFormat OutputFormat
}

func (r *ListChatsResponse) Type() string {
	return "list_chats"
}

func (r *ListChatsResponse) TimeCost() float64 {
	return r.Duration
}

func (r *ListChatsResponse) SetOutputFormat(format OutputFormat) {
	r.OutputFormat = format
}

func (r *ListChatsResponse) PrintOut() {
	if r.Code == 0 {
		total := r.Data["total"].(float64)
		fmt.Printf("Total: %0.0f\n", total)
		docs := r.Data["chats"].([]interface{})
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

type ListSearchesResponse struct {
	Code         int                    `json:"code"`
	Data         map[string]interface{} `json:"data"`
	Message      string                 `json:"message"`
	Duration     float64
	OutputFormat OutputFormat
}

func (r *ListSearchesResponse) Type() string {
	return "list_searches"
}

func (r *ListSearchesResponse) TimeCost() float64 {
	return r.Duration
}

func (r *ListSearchesResponse) SetOutputFormat(format OutputFormat) {
	r.OutputFormat = format
}

func (r *ListSearchesResponse) PrintOut() {
	if r.Code == 0 {
		total := r.Data["total"].(float64)
		fmt.Printf("Total: %0.0f\n", total)
		docs := r.Data["search_apps"].([]interface{})
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

type ListMemoriesResponse struct {
	Code         int                    `json:"code"`
	Data         map[string]interface{} `json:"data"`
	Message      string                 `json:"message"`
	Duration     float64
	OutputFormat OutputFormat
}

func (r *ListMemoriesResponse) Type() string {
	return "list_memories"
}

func (r *ListMemoriesResponse) TimeCost() float64 {
	return r.Duration
}

func (r *ListMemoriesResponse) SetOutputFormat(format OutputFormat) {
	r.OutputFormat = format
}

func (r *ListMemoriesResponse) PrintOut() {
	if r.Code == 0 {
		total := r.Data["total_count"].(float64)
		fmt.Printf("Total: %0.0f\n", total)
		docs := r.Data["memory_list"].([]interface{})
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

func HandleSimpleResponse(response *Response, command string) (ResponseIf, error) {
	if response.StatusCode != 200 {
		return nil, fmt.Errorf("failed to %s: HTTP %d, body: %s", command, response.StatusCode, string(response.Body))
	}

	var result SimpleResponse
	if err := json.Unmarshal(response.Body, &result); err != nil {
		return nil, fmt.Errorf("%s failed: invalid JSON (%w)", command, err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}

	result.Duration = response.Duration
	return &result, nil
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
		id := getChunkID(chunk)
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

func getChunkID(c map[string]interface{}) string {
	for _, key := range []string{"chunk_id", "id"} {
		if v, ok := c[key]; ok {
			return fmt.Sprint(v)
		}
	}
	return "-"
}

func chunkContent(c map[string]interface{}) string {
	for _, key := range []string{"content_with_weight", "content"} {
		if v, ok := c[key]; ok {
			s := fmt.Sprint(v)
			return strings.TrimSpace(s)
		}
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

type UserIndexResponse struct {
	CommonDataResponse
}

func (r *UserIndexResponse) Type() string {
	return "user_index"
}

func (r *UserIndexResponse) PrintOut() {
	if r.Code != 0 {
		fmt.Println("ERROR")
		fmt.Printf("%d, %s\n", r.Code, r.Message)
		return
	}

	summaryTable := r.orderedMetricTable()
	indexTable := make([]map[string]interface{}, 0)
	indicesRaw, hasIndices := r.Data["indices"]
	if hasIndices {
		if indices, ok := indicesRaw.([]interface{}); ok {
			for _, idx := range indices {
				if m, ok := idx.(map[string]interface{}); ok {
					indexTable = append(indexTable, m)
				}
			}
		}
	}

	if r.OutputFormat == OutputFormatJSON {
		payload := make(map[string]interface{})
		if len(summaryTable) > 0 {
			payload["summary"] = summaryTable
		}
		if len(indexTable) > 0 {
			payload["indices"] = indexTable
		}
		jsonData, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			fmt.Printf("Error marshaling JSON: %v\n", err)
			return
		}
		fmt.Println(string(jsonData))
		return
	}

	if len(summaryTable) > 0 {
		PrintTableSimpleByFormat(summaryTable, r.OutputFormat)
	}

	if len(indexTable) > 0 {
		fmt.Println()
		fmt.Println("Index Details:")
		PrintTableSimpleByFormat(indexTable, r.OutputFormat)
	} else if hasIndices {
		fmt.Println()
		fmt.Println("No indices found for this user.")
	}
}

type UserStorageResponse struct {
	CommonDataResponse
}

func (r *UserStorageResponse) Type() string {
	return "user_storage"
}

func (r *UserStorageResponse) PrintOut() {
	if r.Code != 0 {
		fmt.Println("ERROR")
		fmt.Printf("%d, %s\n", r.Code, r.Message)
		return
	}

	summaryTable := r.orderedMetricTable()
	fileTable := make([]map[string]interface{}, 0)
	filesRaw, hasFiles := r.Data["files"]
	if hasFiles {
		if files, ok := filesRaw.([]interface{}); ok {
			for _, f := range files {
				if m, ok := f.(map[string]interface{}); ok {
					fileTable = append(fileTable, m)
				}
			}
		}
	}

	if r.OutputFormat == OutputFormatJSON {
		payload := make(map[string]interface{})
		if len(summaryTable) > 0 {
			payload["summary"] = summaryTable
		}
		if len(fileTable) > 0 {
			payload["files"] = fileTable
		}
		jsonData, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			fmt.Printf("Error marshaling JSON: %v\n", err)
			return
		}
		fmt.Println(string(jsonData))
		return
	}

	if len(summaryTable) > 0 {
		PrintTableSimpleByFormat(summaryTable, r.OutputFormat)
	}

	if len(fileTable) > 0 {
		fmt.Println()
		fmt.Println("Files（Top 10）:")
		PrintTableSimpleByFormat(fileTable, r.OutputFormat)
	} else if hasFiles {
		fmt.Println()
		fmt.Println("No files found for this user.")
	}
}

func truncateStr(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}

type UserQuotaResponse struct {
	CommonDataResponse
}

func (r *UserQuotaResponse) Type() string {
	return "user_quota"
}

func (r *UserQuotaResponse) PrintOut() {
	if r.Code != 0 {
		fmt.Println("ERROR")
		fmt.Printf("%d, %s\n", r.Code, r.Message)
		return
	}

	summaryTable := make([]map[string]interface{}, 0)
	if rowsRaw, ok := r.Data["rows"]; ok {
		if rows, ok := rowsRaw.([]interface{}); ok {
			for _, row := range rows {
				if m, ok := row.(map[string]interface{}); ok {
					summaryTable = append(summaryTable, m)
				}
			}
		}
	}
	if len(summaryTable) > 0 {
		PrintTableSimpleByFormat(summaryTable, r.OutputFormat)
	}
}

type OrderedCommonResponse struct {
	CommonResponse
}

func (r *OrderedCommonResponse) PrintOut() {
	if r.Code == 0 {
		PrintTableSimpleByFormat(r.Data, r.OutputFormat)
	} else {
		fmt.Println("ERROR")
		fmt.Printf("%d, %s\n", r.Code, r.Message)
	}
}

type OrderedCommonDataResponse struct {
	CommonDataResponse
}

func (r *OrderedCommonDataResponse) PrintOut() {
	if r.Code == 0 {
		table := r.orderedMetricTable()
		if len(table) > 0 {
			PrintTableSimpleByFormat(table, r.OutputFormat)
		}
	} else {
		fmt.Println("ERROR")
		fmt.Printf("%d, %s\n", r.Code, r.Message)
	}
}

type QuotaSummaryResponse struct {
	CommonDataResponse
}

func (r *QuotaSummaryResponse) Type() string {
	return "quota_summary"
}

func (r *QuotaSummaryResponse) TimeCost() float64 {
	return r.Duration
}

func (r *QuotaSummaryResponse) SetOutputFormat(format OutputFormat) {
	r.OutputFormat = format
}

func (r *QuotaSummaryResponse) PrintOut() {
	if r.Code != 0 {
		fmt.Println("ERROR")
		fmt.Printf("%d, %s\n", r.Code, r.Message)
		return
	}

	sections := []string{"storage", "apps", "api"}

	for i, key := range sections {
		if i > 0 {
			fmt.Println()
		}

		rowsRaw, ok := r.Data[key]
		if !ok {
			fmt.Println("No data")
			continue
		}

		rows, ok := rowsRaw.([]interface{})
		if !ok || len(rows) == 0 {
			fmt.Println("No data")
			continue
		}

		table := make([]map[string]interface{}, 0, len(rows))
		for _, row := range rows {
			if m, ok := row.(map[string]interface{}); ok {
				table = append(table, m)
			}
		}

		PrintTableSimpleByFormat(table, r.OutputFormat)
	}
}

// ChatCompletionsResponse represents the RAGFlow-internal response from
// POST /api/v1/chat/completions (non-OpenAI format).
//
// JSON shape:
//
//	{"code":0,"data":{"answer":"...","reference":...},"message":""}
type ChatCompletionsResponse struct {
	Code         int                 `json:"code"`
	Data         *chatCompletionData `json:"data"`
	Message      string              `json:"message"`
	Duration     float64             `json:"-"`
	OutputFormat OutputFormat        `json:"-"`
	// raw HTTP body for "raw" output.
	raw []byte
	// streamed skips the "Answer:" line in PrintOut to avoid duplication
	// (used by the streaming path which prints chunk-by-chunk).
	streamed bool
}

type chatCompletionData struct {
	Answer    string          `json:"answer"`
	Reference json.RawMessage `json:"reference,omitempty"`
	ID        string          `json:"id,omitempty"`
	SessionID string          `json:"session_id,omitempty"`
	ChatID    string          `json:"chat_id,omitempty"`
}

func (r *ChatCompletionsResponse) Type() string                   { return "chat_completions" }
func (r *ChatCompletionsResponse) TimeCost() float64              { return r.Duration }
func (r *ChatCompletionsResponse) SetOutputFormat(f OutputFormat) { r.OutputFormat = f }

func (r *ChatCompletionsResponse) PrintOut() {
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
		if r.Data.Answer != "" {
			fmt.Printf("Answer: %s\n", r.Data.Answer)
		}
	}
	if r.Data != nil && len(r.Data.Reference) > 0 {
		printReferenceChunks(r.Data.Reference)
	}
	fmt.Printf("Time: %f\n", r.Duration)
}
