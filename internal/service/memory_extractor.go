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
// QueueSaveToMemoryTask persists the raw message and publishes a
// task_type="memory" TaskMessage on the NATS tasks.RAGFLOW subject. The
// Ingestor's shared consumer + worker pool dispatches it by TaskType to
// HandleSaveToMemoryTask (see internal/ingestion/service/handleAndExecute and
// executeMemoryTask), which runs LLM extraction for the non-raw memory types
// configured on the memory and persists the extracted messages with source_id
// pointing at the raw message so listMemoryMessages can aggregate them under
// `extract`. Publishing over NATS (instead of the Python te.*.common Redis
// stream) keeps Go out of the Python executor's queue and removes the
// cross-consumer contention that previously stole Python dataflow tasks.
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	models "ragflow/internal/entity/models"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// memoryTimeLayout is the storage format for valid_at / invalid_at: a
// server-local wall-clock string ("YYYY-MM-DD HH:MM:SS"), never UTC-shifted.
const memoryTimeLayout = "2006-01-02 15:04:05"

const (
	memoryTaskLeaseTTL            = 2 * time.Minute
	memoryTaskRetryInitialDelay   = 5 * time.Second
	memoryTaskRetryMaxDelay       = 5 * time.Minute
	memoryTaskFailureWriteTimeout = 5 * time.Second
)

var (
	memoryTaskLeaseRenewInterval = 30 * time.Second
	memoryTaskLeaseRenewTimeout  = 10 * time.Second
)

var errPermanentMemoryTask = errors.New("memory: permanent task failure")

// classifyMemoryTaskDependencyError marks confirmed missing or unusable
// dependencies as permanent while preserving transient lookup failures.
func classifyMemoryTaskDependencyError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, errModelConfigUnavailable) {
		return fmt.Errorf("%w: %w", errPermanentMemoryTask, err)
	}
	return err
}

// memoryNow is the wall clock behind every memory timestamp. Tests pin it
// to a fixed instant in a fixed location so the server-local assertions are
// deterministic on any host, including UTC CI runners.
var memoryNow = time.Now

// MemoryTaskDisposition tells the broker consumer whether a durable memory
// task delivery can be acknowledged. An unsettled delivery is recovered by
// broker redelivery or the database reconciler.
type MemoryTaskDisposition uint8

const (
	// MemoryTaskLeaveUnsettled preserves the delivery for durable recovery.
	MemoryTaskLeaveUnsettled MemoryTaskDisposition = iota
	// MemoryTaskAcknowledge permits the consumer to settle the delivery.
	MemoryTaskAcknowledge
)

// extractedMemory is one LLM-extracted memory item ready for persistence.
type extractedMemory struct {
	MessageID   int64  `json:"message_id"`
	MessageType string `json:"message_type"`
	Content     string `json:"content"`
	ValidAt     string `json:"valid_at"`
	InvalidAt   string `json:"invalid_at,omitempty"` // empty means still valid
}

type memoryExtractionCheckpoint struct {
	MessageID   string `json:"message_id"`
	MessageType string `json:"message_type"`
	Content     string `json:"content"`
	ValidAt     string `json:"valid_at"`
	InvalidAt   string `json:"invalid_at,omitempty"`
}

// HandleSaveToMemoryTask claims and resumes one durable memory task. The NATS
// delivery is only a wake-up; task input and checkpoints always come from the
// memory_task row identified by taskID.
func (s *MemoryMessageService) HandleSaveToMemoryTask(ctx context.Context, taskID, leaseOwner string) (MemoryTaskDisposition, error) {
	if s == nil {
		return MemoryTaskAcknowledge, errors.New("memory: nil MemoryMessageService")
	}
	if taskID == "" || leaseOwner == "" {
		return MemoryTaskLeaveUnsettled, errors.New("memory: task id and lease owner are required")
	}
	if s.memoryTaskDAO == nil {
		s.memoryTaskDAO = dao.NewMemoryTaskDAO()
	}

	task, acquired, err := s.memoryTaskDAO.Claim(ctx, dao.DB, taskID, leaseOwner, memoryNow(), memoryTaskLeaseTTL)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return MemoryTaskAcknowledge, fmt.Errorf("memory: task %s is not found", taskID)
		}
		return MemoryTaskLeaveUnsettled, fmt.Errorf("memory: claim task %s: %w", taskID, err)
	}
	if !acquired {
		switch task.State {
		case entity.MemoryTaskStateCompleted, entity.MemoryTaskStateFailed:
			return MemoryTaskAcknowledge, nil
		case entity.MemoryTaskStatePending, entity.MemoryTaskStateExtracted, entity.MemoryTaskStateStored:
			return MemoryTaskAcknowledge, nil
		default:
			return MemoryTaskAcknowledge, fmt.Errorf("memory: task %s has unknown state %q", taskID, task.State)
		}
	}

	return s.runClaimedMemoryTask(ctx, task, leaseOwner)
}

// runClaimedMemoryTask renews the DB lease while the state machine advances.
func (s *MemoryMessageService) runClaimedMemoryTask(ctx context.Context, task *entity.MemoryTask, leaseOwner string) (MemoryTaskDisposition, error) {
	if task.LeaseExpiresAt == nil {
		return MemoryTaskLeaveUnsettled, fmt.Errorf("memory: claimed task %s has no lease expiration", task.TaskID)
	}
	leaseExpiresAt := *task.LeaseExpiresAt
	runCtx, cancel := context.WithCancel(ctx)
	renewErr := make(chan error, 1)
	renewDone := make(chan struct{})
	go func() {
		defer close(renewDone)
		s.renewMemoryTaskLease(runCtx, cancel, task.TaskID, leaseOwner, leaseExpiresAt, renewErr)
	}()
	defer func() {
		cancel()
		<-renewDone
	}()

	resumeTask := s.resumeMemoryTask
	if s.resumeTask != nil {
		resumeTask = s.resumeTask
	}
	err := resumeTask(runCtx, task, leaseOwner)
	if err == nil {
		return MemoryTaskAcknowledge, nil
	}
	select {
	case leaseErr := <-renewErr:
		if leaseErr != nil {
			err = leaseErr
		}
	default:
	}
	return s.persistMemoryTaskFailure(ctx, task, leaseOwner, err)
}

// persistMemoryTaskFailure records the recovery decision before allowing the
// broker delivery to be acknowledged. If the write cannot be proven, the
// delivery remains unsettled and the expired lease is recovered later.
func (s *MemoryMessageService) persistMemoryTaskFailure(ctx context.Context, task *entity.MemoryTask, leaseOwner string, runErr error) (MemoryTaskDisposition, error) {
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), memoryTaskFailureWriteTimeout)
	defer cancel()

	var (
		updated bool
		err     error
		action  string
	)
	now := memoryNow()
	if errors.Is(runErr, errPermanentMemoryTask) {
		action = "mark failed"
		updated, err = s.memoryTaskDAO.MarkFailed(persistCtx, dao.DB, task.TaskID, leaseOwner, runErr.Error(), now)
	} else {
		action = "schedule retry"
		nextRetryAt := now.Add(memoryTaskRetryDelay(task.AttemptCount))
		updated, err = s.memoryTaskDAO.ScheduleRetry(persistCtx, dao.DB, task.TaskID, leaseOwner, now, nextRetryAt, runErr.Error())
	}
	if err != nil {
		return MemoryTaskLeaveUnsettled, errors.Join(runErr, fmt.Errorf("memory: %s for task %s: %w", action, task.TaskID, err))
	}
	if !updated {
		return MemoryTaskLeaveUnsettled, errors.Join(runErr, fmt.Errorf("memory: %s for task %s: lease is no longer owned by this worker", action, task.TaskID))
	}
	return MemoryTaskAcknowledge, runErr
}

// memoryTaskRetryDelay applies bounded exponential backoff using the attempt
// count incremented when the task lease was claimed.
func memoryTaskRetryDelay(attemptCount int) time.Duration {
	delay := memoryTaskRetryInitialDelay
	for attempt := 1; attempt < attemptCount && delay < memoryTaskRetryMaxDelay; attempt++ {
		delay *= 2
		if delay >= memoryTaskRetryMaxDelay {
			return memoryTaskRetryMaxDelay
		}
	}
	return delay
}

// renewMemoryTaskLease retries transient renewal failures while the last
// confirmed lease has time for another attempt, and cancels execution when
// ownership is lost or the lease is too close to expiry.
func (s *MemoryMessageService) renewMemoryTaskLease(ctx context.Context, cancel context.CancelFunc, taskID, leaseOwner string, leaseExpiresAt time.Time, errCh chan<- error) {
	ticker := time.NewTicker(memoryTaskLeaseRenewInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			renewedAt := memoryNow()
			renewCtx, renewCancel := context.WithTimeout(ctx, memoryTaskLeaseRenewTimeout)
			renewed, err := s.memoryTaskDAO.RenewLease(renewCtx, dao.DB, taskID, leaseOwner, renewedAt, memoryTaskLeaseTTL)
			renewCancel()
			if err == nil && renewed {
				leaseExpiresAt = renewedAt.Add(memoryTaskLeaseTTL)
				continue
			}
			if err != nil && memoryNow().Add(memoryTaskLeaseRenewInterval).Before(leaseExpiresAt) {
				common.Warn(fmt.Sprintf("memory: renew task %s lease failed, will retry", taskID), zap.Error(err))
				continue
			}
			if err == nil {
				err = errors.New("lease is no longer owned by this worker")
			}
			select {
			case errCh <- fmt.Errorf("memory: renew task %s lease: %w", taskID, err):
			default:
			}
			cancel()
			return
		}
	}
}

// resumeMemoryTask advances from the last durable checkpoint through
// completion without consulting the generic task progress projection.
func (s *MemoryMessageService) resumeMemoryTask(ctx context.Context, task *entity.MemoryTask, leaseOwner string) error {
	var (
		msg         MemoryMessage
		inputLoaded bool
	)
	loadInput := func() error {
		if inputLoaded {
			return nil
		}
		var err error
		msg, err = memoryMessageFromTaskInput(task.Input)
		if err != nil {
			return fmt.Errorf("%w: decode task %s input: %v", errPermanentMemoryTask, task.TaskID, err)
		}
		inputLoaded = true
		return nil
	}

	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		switch task.State {
		case entity.MemoryTaskStatePending:
			if err := loadInput(); err != nil {
				return err
			}
			extraction, extractErr := s.extractMemoryTask(ctx, task, msg)
			if extractErr != nil {
				return extractErr
			}
			encoded, encodeErr := encodeMemoryExtraction(extraction)
			if encodeErr != nil {
				return fmt.Errorf("memory: encode task %s extraction: %w", task.TaskID, encodeErr)
			}
			updated, persistErr := s.memoryTaskDAO.PersistExtraction(ctx, dao.DB, task.TaskID, leaseOwner, memoryNow(), encoded)
			if persistErr != nil {
				return fmt.Errorf("memory: persist task %s extraction: %w", task.TaskID, persistErr)
			}
			if !updated {
				return fmt.Errorf("memory: task %s lost its lease before extraction checkpoint", task.TaskID)
			}
			task.Extraction = encoded
			task.State = entity.MemoryTaskStateExtracted

		case entity.MemoryTaskStateExtracted:
			if err := loadInput(); err != nil {
				return err
			}
			if err := s.storeMemoryTaskExtraction(ctx, task, msg); err != nil {
				return err
			}
			updated, storeErr := s.memoryTaskDAO.MarkStored(ctx, dao.DB, task.TaskID, leaseOwner, memoryNow())
			if storeErr != nil {
				return fmt.Errorf("memory: mark task %s stored: %w", task.TaskID, storeErr)
			}
			if !updated {
				return fmt.Errorf("memory: task %s lost its lease before storage checkpoint", task.TaskID)
			}
			task.State = entity.MemoryTaskStateStored

		case entity.MemoryTaskStateStored:
			progressMsg := "Message saved successfully."
			if len(task.Extraction) == 0 {
				progressMsg = "No memory extracted from raw message."
			}
			completed, completeErr := s.memoryTaskDAO.Complete(ctx, dao.DB, task.TaskID, leaseOwner, progressMsg, memoryNow())
			if completeErr != nil {
				return fmt.Errorf("memory: complete task %s: %w", task.TaskID, completeErr)
			}
			if !completed {
				return fmt.Errorf("memory: task %s lost its lease before completion", task.TaskID)
			}
			return nil

		case entity.MemoryTaskStateCompleted, entity.MemoryTaskStateFailed:
			return nil
		default:
			return fmt.Errorf("%w: task %s has unknown state %q", errPermanentMemoryTask, task.TaskID, task.State)
		}
	}
}

// extractMemoryTask executes the LLM stage and materializes retry-stable output.
func (s *MemoryMessageService) extractMemoryTask(ctx context.Context, task *entity.MemoryTask, msg MemoryMessage) ([]extractedMemory, error) {
	if s.memories == nil {
		return nil, errors.New("memory: memory service is not initialized")
	}
	mem, err := s.memories.getMemoryConfig(ctx, task.MemoryID)
	if err != nil {
		return nil, classifyMemoryTaskDependencyError(err)
	}
	memoryTypes := mem.MemoryType
	if len(memoryTypes) == 0 {
		memoryTypes = dao.GetMemoryTypeHuman(mem.Memory.MemoryType)
	}
	extractTypes := getTypesToExtract(memoryTypes)
	if len(extractTypes) == 0 {
		return []extractedMemory{}, nil
	}

	extracted, err := s.extractByLLM(ctx, mem, extractTypes, msg, task.TaskID)
	if err != nil {
		return nil, err
	}
	materialized := materializeMemoryExtraction(ctx, extracted, memoryNow())
	_ = s.updateTaskProgress(ctx, task.TaskID, 0.5, fmt.Sprintf("Extracted %d messages from raw dialogue.", len(materialized)))
	return materialized, nil
}

// storeMemoryTaskExtraction embeds and stores a previously checkpointed result.
func (s *MemoryMessageService) storeMemoryTaskExtraction(ctx context.Context, task *entity.MemoryTask, msg MemoryMessage) error {
	extracted, err := decodeMemoryExtraction(task.Extraction)
	if err != nil {
		return fmt.Errorf("%w: decode task %s extraction: %v", errPermanentMemoryTask, task.TaskID, err)
	}
	if len(extracted) == 0 {
		return nil
	}
	if s.memories == nil {
		return errors.New("memory: memory service is not initialized")
	}
	mem, err := s.memories.getMemoryConfig(ctx, task.MemoryID)
	if err != nil {
		return classifyMemoryTaskDependencyError(err)
	}
	messages := make([]map[string]any, 0, len(extracted))
	for _, item := range extracted {
		messages = append(messages, buildExtractedMessage(task.SourceID, task.MemoryID, msg, item))
	}
	if err = s.embedAndSaveMessages(ctx, mem, messages); err != nil {
		return classifyMemoryTaskDependencyError(err)
	}
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
	now := memoryNow().Format(memoryTimeLayout)
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
		return nil, fmt.Errorf("resolve chat model: %w", classifyMemoryTaskDependencyError(err))
	}
	chatModel := models.NewChatModel(driver, &modelName, apiConfig)

	_ = s.updateTaskProgress(ctx, taskID, 0.15, "Prepared prompts and LLM.")
	temperature := mem.Temperature
	resp, err := chatModel.ModelDriver.ChatWithMessages(ctx, modelName, messages, apiConfig, &models.ChatConfig{Temperature: &temperature}, nil)
	if err != nil {
		return nil, fmt.Errorf("chat model: %w", err)
	}
	if resp == nil || resp.Answer == nil {
		return nil, errors.New("empty response from chat model")
	}
	_ = s.updateTaskProgress(ctx, taskID, 0.35, "Get extracted result from LLM.")

	return parseMemoryExtraction(*resp.Answer, extractTypes), nil
}

// materializeMemoryExtraction assigns stable message ids and timestamp
// fallbacks before the extraction checkpoint is persisted.
func materializeMemoryExtraction(ctx context.Context, extracted []extractedMemory, now time.Time) []extractedMemory {
	materialized := make([]extractedMemory, len(extracted))
	for i, item := range extracted {
		item.MessageID = generateRawMessageID(ctx)
		item.ValidAt = formatMemoryTime(item.ValidAt, now)
		item.InvalidAt = strings.TrimSpace(item.InvalidAt)
		if item.InvalidAt != "" {
			item.InvalidAt = formatMemoryTime(item.InvalidAt, now)
		}
		materialized[i] = item
	}
	return materialized
}

// encodeMemoryExtraction converts typed extraction output to the JSON column type.
func encodeMemoryExtraction(extracted []extractedMemory) (entity.JSONSlice, error) {
	checkpoint := make([]memoryExtractionCheckpoint, len(extracted))
	for i, item := range extracted {
		checkpoint[i] = memoryExtractionCheckpoint{
			MessageID:   strconv.FormatInt(item.MessageID, 10),
			MessageType: item.MessageType,
			Content:     item.Content,
			ValidAt:     item.ValidAt,
			InvalidAt:   item.InvalidAt,
		}
	}
	data, err := json.Marshal(checkpoint)
	if err != nil {
		return nil, err
	}
	var encoded entity.JSONSlice
	if err = json.Unmarshal(data, &encoded); err != nil {
		return nil, err
	}
	return encoded, nil
}

// decodeMemoryExtraction restores typed extraction output from its checkpoint.
func decodeMemoryExtraction(encoded entity.JSONSlice) ([]extractedMemory, error) {
	data, err := json.Marshal(encoded)
	if err != nil {
		return nil, err
	}
	var checkpoint []memoryExtractionCheckpoint
	if err = json.Unmarshal(data, &checkpoint); err != nil {
		return nil, err
	}
	extracted := make([]extractedMemory, len(checkpoint))
	for i, item := range checkpoint {
		messageID, parseErr := strconv.ParseInt(item.MessageID, 10, 64)
		if parseErr != nil {
			return nil, fmt.Errorf("parse message id %q: %w", item.MessageID, parseErr)
		}
		extracted[i] = extractedMemory{
			MessageID:   messageID,
			MessageType: item.MessageType,
			Content:     item.Content,
			ValidAt:     item.ValidAt,
			InvalidAt:   item.InvalidAt,
		}
	}
	return extracted, nil
}

// memoryMessageFromTaskInput reads the dialogue persisted with the durable task.
func memoryMessageFromTaskInput(input entity.JSONMap) (MemoryMessage, error) {
	msg := MemoryMessage{
		UserID:        stringFromJSONMap(input, "user_id"),
		AgentID:       stringFromJSONMap(input, "agent_id"),
		SessionID:     stringFromJSONMap(input, "session_id"),
		UserInput:     stringFromJSONMap(input, "user_input"),
		AgentResponse: stringFromJSONMap(input, "agent_response"),
	}
	if msg.AgentID == "" {
		return MemoryMessage{}, errors.New("agent_id is required")
	}
	return msg, nil
}

// stringFromJSONMap returns the named string value or an empty string.
func stringFromJSONMap(values entity.JSONMap, key string) string {
	value, _ := values[key].(string)
	return value
}

// buildExtractedMessage builds the persisted envelope for one materialized
// memory item. Field set matches buildRawMessage except message_type and
// source_id, which listMemoryMessages uses to aggregate extracts.
func buildExtractedMessage(sourceID int64, memoryID string, msg MemoryMessage, item extractedMemory) map[string]any {
	var storedInvalidAt any
	if item.InvalidAt != "" {
		storedInvalidAt = item.InvalidAt
	}
	return map[string]any{
		"id":           fmt.Sprintf("%s_%d", memoryID, item.MessageID),
		"message_id":   item.MessageID,
		"message_type": item.MessageType,
		"source_id":    sourceID,
		"memory_id":    memoryID,
		"user_id":      msg.UserID,
		"agent_id":     msg.AgentID,
		"session_id":   msg.SessionID,
		"content":      item.Content,
		"valid_at":     item.ValidAt,
		"invalid_at":   storedInvalidAt,
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
// already-formatted) into memoryTimeLayout. The parsed timestamp keeps its
// own wall clock (no zone conversion). Unparseable or empty input falls
// back to the supplied time formatted in its own location (server-local
// when callers pass time.Now()).
func formatMemoryTime(value string, fallback time.Time) string {
	if normalized, ok := normalizeMemoryTime(value); ok {
		return normalized
	}
	return fallback.Format(memoryTimeLayout)
}

// normalizeMemoryTime parses an explicit LLM-supplied timestamp without a
// runtime fallback. Its result is suitable for a stable document fingerprint.
func normalizeMemoryTime(value string) (string, bool) {
	v := strings.TrimSpace(value)
	if v == "" {
		return "", false
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, v); err == nil {
			return t.Format(memoryTimeLayout), true
		}
	}
	return "", false
}

// updateTaskProgress stamps and persists the UI progress projection. Durable
// execution decisions never read this value.
func (s *MemoryMessageService) updateTaskProgress(ctx context.Context, taskID string, progress float64, msg string) error {
	if s == nil || s.taskDAO == nil {
		return errors.New("memory: nil task DAO")
	}
	if taskID == "" {
		return errors.New("memory: empty task id")
	}
	stamped := time.Now().Format(memoryTimeLayout) + " " + msg
	if err := s.taskDAO.UpdateProgress(ctx, dao.DB, taskID, progress, stamped); err != nil {
		common.Warn("memory: update task progress failed", zap.Error(err))
		return err
	}
	return nil
}
