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

// memory_extractor.go — async memory extraction worker.
//
// Port of the Python task-executor memory path:
//
//	rag/svr/task_executor.py:handle_task (task_type == "memory")
//	api/db/joint_services/memory_message_service.py:
//	    handle_save_to_memory_task / save_extracted_to_memory_only / extract_by_llm
//
// QueueSaveToMemoryTask persists the raw message and enqueues a
// task_type="memory" message on the Redis stream te.<priority>.common.
// StartTaskConsumer drains that stream (consumer group
// "rag_flow_svr_task_broker", same as the Python executor), runs LLM
// extraction for the non-raw memory types configured on the memory, and
// persists the extracted messages with source_id pointing at the raw
// message so listMemoryMessages can aggregate them under `extract`.
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	redisengine "ragflow/internal/engine/redis"
	"ragflow/internal/entity"
	models "ragflow/internal/entity/models"
	"ragflow/internal/utility"

	"go.uber.org/zap"
)

// memoryTaskConsumerGroup matches Python common.constants.SVR_CONSUMER_GROUP_NAME
// so Go and Python executors never double-process the same stream entry.
const memoryTaskConsumerGroup = "rag_flow_svr_task_broker"

// memoryTimeLayout is the storage format for valid_at / invalid_at,
// matching timestamp_to_date on the Python side.
const memoryTimeLayout = "2006-01-02 15:04:05"

// extractedMemory is one LLM-extracted memory item ready for persistence.
type extractedMemory struct {
	MessageType string
	Content     string
	ValidAt     string
	InvalidAt   string // empty means still valid
}

// StartTaskConsumer is the long-running loop that drains memory
// extraction tasks from the Redis stream. It returns when ctx is
// cancelled. Per-message failures are logged and acked so one bad
// message cannot stall the stream.
func (s *MemoryMessageService) StartTaskConsumer(ctx context.Context) {
	redisClient := redisengine.Get()
	if redisClient == nil {
		common.Error("memory task consumer: Redis is not available", nil)
		return
	}
	consumerName := fmt.Sprintf("go_memory_extractor_%s", utility.GenerateUUID())
	queueName := memoryTaskQueueName(0)
	common.Info(fmt.Sprintf("Memory task consumer %s started on queue %s", consumerName, queueName))

	for {
		if ctx.Err() != nil {
			return
		}
		msg, err := redisClient.QueueConsumer(ctx, queueName, memoryTaskConsumerGroup, consumerName, ">")
		if err != nil {
			common.Error("memory task consumer: consume error", err)
			select {
			case <-time.After(time.Second):
			case <-ctx.Done():
				return
			}
			continue
		}
		if msg == nil {
			continue
		}
		payload := msg.GetMessage()
		if taskType, _ := payload["task_type"].(string); taskType != "memory" {
			common.Warn(fmt.Sprintf("memory task consumer: skip task_type %q", taskType))
			msg.Ack(ctx)
			continue
		}
		if err := s.HandleSaveToMemoryTask(ctx, payload); err != nil {
			common.Error("memory task consumer: handle task failed", err)
		}
		msg.Ack(ctx)
	}
}

// HandleSaveToMemoryTask processes one queued memory task. Mirrors
// Python handle_save_to_memory_task: validate the task row, then
// extract + persist, settling task progress on the way out.
func (s *MemoryMessageService) HandleSaveToMemoryTask(ctx context.Context, payload map[string]any) error {
	taskID, _ := payload["id"].(string)
	if taskID == "" {
		taskID, _ = payload["task_id"].(string)
	}
	memoryID, _ := payload["memory_id"].(string)
	sourceID := payloadInt64(payload["source_id"])
	msgDict, _ := payload["message_dict"].(map[string]any)
	msg := MemoryMessage{
		UserID:        payloadString(msgDict["user_id"]),
		AgentID:       payloadString(msgDict["agent_id"]),
		SessionID:     payloadString(msgDict["session_id"]),
		UserInput:     payloadString(msgDict["user_input"]),
		AgentResponse: payloadString(msgDict["agent_response"]),
	}

	task, err := s.taskDAO.GetByID(ctx, dao.DB, taskID)
	if err != nil {
		return fmt.Errorf("memory: task %s is not found", taskID)
	}
	if task.Progress == -1 {
		return fmt.Errorf("memory: task %s is already failed", taskID)
	}

	if err := s.saveExtractedToMemory(ctx, memoryID, msg, sourceID, taskID); err != nil {
		s.updateTaskProgress(taskID, -1, err.Error())
		return err
	}
	return nil
}

// saveExtractedToMemory mirrors Python save_extracted_to_memory_only:
// skip raw-only memories, run LLM extraction, embed and persist the
// extracted messages under the raw message's id.
func (s *MemoryMessageService) saveExtractedToMemory(ctx context.Context, memoryID string, msg MemoryMessage, sourceID int64, taskID string) error {
	mem, err := s.memories.getMemoryConfig(ctx, memoryID)
	if err != nil {
		return err
	}
	memoryTypes := mem.MemoryType
	if len(memoryTypes) == 0 {
		memoryTypes = dao.GetMemoryTypeHuman(mem.Memory.MemoryType)
	}
	extractTypes := getTypesToExtract(memoryTypes)
	if len(extractTypes) == 0 {
		s.updateTaskProgress(taskID, 1.0, fmt.Sprintf("Memory '%s' don't need to extract.", memoryID))
		return nil
	}

	extracted, err := s.extractByLLM(ctx, mem, extractTypes, msg, taskID)
	if err != nil {
		return err
	}
	if len(extracted) == 0 {
		s.updateTaskProgress(taskID, 1.0, "No memory extracted from raw message.")
		return nil
	}
	s.updateTaskProgress(taskID, 0.5, fmt.Sprintf("Extracted %d messages from raw dialogue.", len(extracted)))

	now := time.Now().UTC()
	messages := make([]map[string]any, 0, len(extracted))
	for _, item := range extracted {
		messages = append(messages, buildExtractedMessage(generateRawMessageID(ctx), sourceID, memoryID, msg, item, now))
	}
	if err := s.embedAndSaveMessages(ctx, mem, messages); err != nil {
		return err
	}
	s.updateTaskProgress(taskID, 1.0, "Message saved successfully.")
	return nil
}

// extractByLLM mirrors Python extract_by_llm: build the system/user
// prompts from the memory config, chat with the configured model, and
// parse the JSON result into per-type extracted items.
func (s *MemoryMessageService) extractByLLM(ctx context.Context, mem *CreateMemoryResponse, extractTypes []string, msg MemoryMessage, taskID string) ([]extractedMemory, error) {
	systemPrompt := ""
	if mem.SystemPrompt != nil {
		systemPrompt = *mem.SystemPrompt
	}
	if strings.TrimSpace(systemPrompt) == "" {
		systemPrompt = PromptAssembler{}.AssembleSystemPrompt(extractTypes)
	}

	conversation := fmt.Sprintf("User Input: %s\nAgent Response: %s", msg.UserInput, msg.AgentResponse)
	now := time.Now().UTC().Format(memoryTimeLayout)
	messages := []models.Message{{Role: "system", Content: systemPrompt}}
	if mem.UserPrompt != nil && strings.TrimSpace(*mem.UserPrompt) != "" {
		messages = append(messages,
			models.Message{Role: "user", Content: *mem.UserPrompt},
			models.Message{Role: "user", Content: fmt.Sprintf("Conversation: %s\nConversation Time: %s\nCurrent Time: %s", conversation, now, now)},
		)
	} else {
		messages = append(messages, models.Message{Role: "user", Content: PromptAssembler{}.AssembleUserPrompt(conversation, now, now)})
	}

	// Python prefers tenant_llm_id and falls back to llm_id;
	// ResolveModelConfig accepts both tenant-model ids and model names.
	llmRef := mem.LLMID
	if mem.TenantLLMID != nil && *mem.TenantLLMID != "" {
		llmRef = *mem.TenantLLMID
	}
	driver, modelName, apiConfig, _, err := NewModelProviderService().ResolveModelConfig(ctx, mem.TenantID, entity.ModelTypeChat, llmRef)
	if err != nil {
		return nil, fmt.Errorf("resolve chat model: %w", err)
	}
	chatModel := models.NewChatModel(driver, &modelName, apiConfig)

	s.updateTaskProgress(taskID, 0.15, "Prepared prompts and LLM.")
	temperature := mem.Temperature
	resp, err := chatModel.ModelDriver.ChatWithMessages(ctx, modelName, messages, apiConfig, &models.ChatConfig{Temperature: &temperature}, nil)
	if err != nil {
		return nil, fmt.Errorf("chat model: %w", err)
	}
	if resp == nil || resp.Answer == nil {
		return nil, errors.New("empty response from chat model")
	}
	s.updateTaskProgress(taskID, 0.35, "Get extracted result from LLM.")

	return parseMemoryExtraction(*resp.Answer, extractTypes), nil
}

// buildExtractedMessage builds the persisted envelope for one extracted
// memory item. Field set matches buildRawMessage except message_type and
// source_id, which listMemoryMessages uses to aggregate extracts. Only
// logical message fields are set here; the doc engine maps them to
// storage fields (including tokenization) at insert time.
func buildExtractedMessage(messageID, sourceID int64, memoryID string, msg MemoryMessage, item extractedMemory, now time.Time) map[string]any {
	var invalidAt any
	if strings.TrimSpace(item.InvalidAt) != "" {
		invalidAt = formatMemoryTime(item.InvalidAt, now)
	}
	return map[string]any{
		"message_id":   messageID,
		"message_type": item.MessageType,
		"source_id":    sourceID,
		"memory_id":    memoryID,
		"user_id":      msg.UserID,
		"agent_id":     msg.AgentID,
		"session_id":   msg.SessionID,
		"content":      item.Content,
		"valid_at":     formatMemoryTime(item.ValidAt, now),
		"invalid_at":   invalidAt,
		"forget_at":    nil,
		"status":       true,
	}
}

// parseMemoryExtraction ports memory.utils.msg_util.get_json_result_from_llm_response
// plus the per-type flattening in extract_by_llm. Only the configured
// extract types are collected; unparseable responses yield an empty list.
func parseMemoryExtraction(answer string, extractTypes []string) []extractedMemory {
	clean := strings.TrimSpace(answer)
	clean = strings.TrimPrefix(clean, "```json")
	clean = strings.TrimPrefix(clean, "```")
	clean = strings.TrimSuffix(clean, "```")
	if start := strings.Index(clean, "{"); start >= 0 {
		if end := strings.LastIndex(clean, "}"); end > start {
			clean = clean[start : end+1]
		}
	}

	var parsed map[string][]struct {
		Content   string `json:"content"`
		ValidAt   string `json:"valid_at"`
		InvalidAt string `json:"invalid_at"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(clean)), &parsed); err != nil {
		common.Warn("memory: failed to parse LLM extraction result", zap.Error(err))
		return nil
	}

	var out []extractedMemory
	for _, memoryType := range extractTypes {
		for _, item := range parsed[memoryType] {
			if strings.TrimSpace(item.Content) == "" {
				continue
			}
			out = append(out, extractedMemory{
				MessageType: memoryType,
				Content:     item.Content,
				ValidAt:     item.ValidAt,
				InvalidAt:   item.InvalidAt,
			})
		}
	}
	return out
}

// formatMemoryTime normalizes an LLM-supplied timestamp (ISO 8601 or
// already-formatted) into memoryTimeLayout. Unparseable or empty input
// falls back to the supplied time.
func formatMemoryTime(value string, fallback time.Time) string {
	v := strings.TrimSpace(value)
	if v != "" {
		for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02"} {
			if t, err := time.Parse(layout, v); err == nil {
				return t.UTC().Format(memoryTimeLayout)
			}
		}
	}
	return fallback.UTC().Format(memoryTimeLayout)
}

// updateTaskProgress stamps and persists task progress, mirroring
// Python TaskService.update_progress call sites. Failures are logged
// and swallowed so progress reporting never breaks extraction.
func (s *MemoryMessageService) updateTaskProgress(taskID string, progress float64, msg string) {
	if s == nil || s.taskDAO == nil || taskID == "" {
		return
	}
	stamped := time.Now().Format(memoryTimeLayout) + " " + msg
	if err := s.taskDAO.UpdateProgress(taskID, progress, stamped); err != nil {
		common.Warn("memory: update task progress failed", zap.Error(err))
	}
}

func payloadInt64(v any) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case int64:
		return n
	case int:
		return int64(n)
	case json.Number:
		i, _ := n.Int64()
		return i
	case string:
		var i int64
		fmt.Sscanf(n, "%d", &i)
		return i
	}
	return 0
}

func payloadString(v any) string {
	s, _ := v.(string)
	return s
}
