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

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"ragflow/internal/utility"
	goruntime "runtime"
	"strings"
	"sync"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine"
	"ragflow/internal/entity"
	taskpkg "ragflow/internal/ingestion/task"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"gorm.io/gorm"
)

type Ingestor struct {
	id          string
	name        string
	serverAddr  string
	conn        *grpc.ClientConn
	ctx         context.Context
	cancel      context.CancelFunc
	reconnectMu sync.Mutex

	// Configuration
	maxConcurrency    int32
	supportedDocTypes []string
	version           string

	// Runtime state
	currentTasks map[string]*taskpkg.TaskContext
	tasksMu      sync.RWMutex

	// Shutdown channel - receive on this to trigger graceful shutdown
	ShutdownCh chan struct{}

	// Worker pool
	taskChan  chan *taskpkg.TaskContext
	workerWg  sync.WaitGroup
	startOnce sync.Once

	ingestionTaskDAO       *dao.IngestionTaskDAO
	ingestionTaskLogDAO    *dao.IngestionTaskLogDAO
	ingestionTaskletDAO    *dao.IngestionTaskletDAO
	ingestionTaskletLogDAO *dao.IngestionTaskletLogDAO

	// runDocumentTask dispatches to the migrated task handler path.
	// Tests may override this to verify branch routing without invoking
	// the full downstream stack.
	runDocumentTask func(ctx context.Context, ingestionTask *entity.IngestionTask) error
}

func NewIngestor(name string, maxConcurrency int32, supportedTypes []string) *Ingestor {
	ctx, cancel := context.WithCancel(context.Background())
	id := utility.GenerateUUID()
	ingestor := &Ingestor{
		id:                     id,
		name:                   name,
		ctx:                    ctx,
		cancel:                 cancel,
		maxConcurrency:         maxConcurrency,
		supportedDocTypes:      supportedTypes,
		version:                "1.0.0",
		currentTasks:           make(map[string]*taskpkg.TaskContext),
		taskChan:               make(chan *taskpkg.TaskContext, maxConcurrency*2),
		ShutdownCh:             make(chan struct{}, 1),
		ingestionTaskDAO:       dao.NewIngestionTaskDAO(),
		ingestionTaskLogDAO:    dao.NewIngestionTaskLogDAO(),
		ingestionTaskletDAO:    dao.NewIngestionTaskletDAO(),
		ingestionTaskletLogDAO: dao.NewIngestionTaskletLogDAO(),
	}
	ingestor.runDocumentTask = ingestor.defaultRunDocumentTask
	return ingestor
}

func (e *Ingestor) ID() string {
	return e.id
}

func (e *Ingestor) Start() error {
	common.Info(fmt.Sprintf("Ingestor %s initialized", e.id))
	msgQueueEngine := engine.GetMessageQueueEngine()
	err := msgQueueEngine.InitConsumer("tasks.RAGFLOW")
	if err != nil {
		return err
	}

	// Ensure worker pool is started on first task
	go e.startWorkerPool()

	for {
		var taskHandles []common.TaskHandle
		taskHandles, err = msgQueueEngine.GetMessages(4)
		if err != nil {
			common.Error("error consuming message", err)
			continue
		}
		for _, taskHandle := range taskHandles {
			taskMessage := taskHandle.GetMessage()
			common.Info(fmt.Sprintf("Received task id: %s, type: %s", taskMessage.TaskID, taskMessage.TaskType))
			if taskMessage.TaskType != common.TaskTypeIngestionTask {
				common.Info(fmt.Sprintf("task %s is not an ingestion task", taskMessage.TaskID))
				err = taskHandle.Ack()
				if err != nil {
					common.Error(fmt.Sprintf("error ack task %s", taskMessage.TaskID), err)
					return err
				}
				continue
			}
			var task *entity.IngestionTask
			task, err = e.ingestionTaskDAO.SetRunningByIngestor(taskMessage.TaskID)
			if err != nil {
				if errors.Is(err, common.ErrTaskNotFound) {
					common.Warn(fmt.Sprintf("task %s not found, skipping", taskMessage.TaskID))
					err = taskHandle.Ack()
					if err != nil {
						common.Error(fmt.Sprintf("error ack task %s", taskMessage.TaskID), err)
						return err
					}
					continue
				} else {
					common.Error(fmt.Sprintf("error setting task %s to running", taskMessage.TaskID), err)
					return err
				}
			}
			if task == nil {
				common.Info(fmt.Sprintf("task %s is already removed", taskMessage.TaskID))
				err = taskHandle.Ack()
				if err != nil {
					return err
				}
				continue
			}

			switch task.Status {
			case common.COMPLETED, common.STOPPED, common.FAILED:
				common.Info(fmt.Sprintf("task %s is already %s", taskMessage.TaskID, task.Status))
				err = taskHandle.Ack()
				if err != nil {
					common.Error(fmt.Sprintf("error nack task %s", taskMessage.TaskID), err)
					return err
				}
				continue
			case common.STOPPING, common.CREATED:
				err = fmt.Errorf("task %s is in unexpected status %s", taskMessage.TaskID, task.Status)
				return err
			case common.RUNNING:
			}

			// Construct TaskContext with parent context
			taskCtx := taskpkg.NewTaskContextForScheduling(e.ctx, task, taskHandle)

			// Push to task channel; if full, reject the task (backpressure)
			select {
			case e.taskChan <- taskCtx:
				common.Info(fmt.Sprintf("Task %s queued (channel: %d/%d)", task.ID, len(e.taskChan), cap(e.taskChan)))
			default:
				common.Info(fmt.Sprintf("No available slot for task %s, failed", task.ID))

				err = taskHandle.Nack()
				if err != nil {
					common.Error(fmt.Sprintf("error nack task %s", taskMessage.TaskID), err)
					return err
				}
			}
		}
	}
}

func (e *Ingestor) startWorkerPool() {
	e.startOnce.Do(func() {
		for i := int32(0); i < e.maxConcurrency; i++ {
			e.workerWg.Add(1)
			go e.workerLoop(i)
		}
		common.Info(fmt.Sprintf("Worker pool started with %d workers", e.maxConcurrency))
	})
}

func (e *Ingestor) workerLoop(id int32) {
	defer e.workerWg.Done()
	common.Info(fmt.Sprintf("Worker %d started", id))
	for {
		select {
		case <-e.ctx.Done():
			return
		case taskCtx := <-e.taskChan:
			common.Info("task context:" + taskCtx.IngestionTask.ID)
			e.executeTask(taskCtx)
		}
	}
}

func (e *Ingestor) executeTask(taskCtx *taskpkg.TaskContext) {
	ctx := taskCtx.Ctx
	task := taskCtx.IngestionTask
	common.Info(fmt.Sprintf("Starting task %s", task.ID))

	latestLog, err := e.ingestionTaskLogDAO.LatestLogByTaskID(task.ID)
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			common.Error(fmt.Sprintf("Failed to get latest task log for task %s", task.ID), err)
			if uErr := e.ingestionTaskDAO.UpdateStatus(task.ID, common.FAILED); uErr != nil {
				common.Error(fmt.Sprintf("Failed to set task %s to FAILED", task.ID), uErr)
			}
			return
		}
		latestLog = &entity.IngestionTaskLog{
			ID:     0,
			TaskID: task.ID,
			Checkpoint: entity.JSONMap{
				"current_step": 1,
				"total_step":   5,
			},
		}
		err = e.ingestionTaskLogDAO.Create(latestLog)
		if err != nil {
			common.Error(fmt.Sprintf("Failed to create task log for task %s", task.ID), err)
			if uErr := e.ingestionTaskDAO.UpdateStatus(task.ID, common.FAILED); uErr != nil {
				common.Error(fmt.Sprintf("Failed to set task %s to FAILED", task.ID), uErr)
			}
			return
		}
	}

	var checkpointMap map[string]interface{}
	checkpointMap = latestLog.Checkpoint
	currentStep, ok := common.GetInt(checkpointMap["current_step"])
	if !ok {
		common.Error(fmt.Sprintf("Failed to get current step from task log for task %s", task.ID), nil)
		if uErr := e.ingestionTaskDAO.UpdateStatus(task.ID, common.FAILED); uErr != nil {
			common.Error(fmt.Sprintf("Failed to set task %s to FAILED", task.ID), uErr)
		}
		return
	}
	totalStep, ok := common.GetInt(checkpointMap["total_step"])
	if !ok {
		common.Error(fmt.Sprintf("Failed to get total step from task log for task %s", task.ID), nil)
		if uErr := e.ingestionTaskDAO.UpdateStatus(task.ID, common.FAILED); uErr != nil {
			common.Error(fmt.Sprintf("Failed to set task %s to FAILED", task.ID), uErr)
		}
		return
	}
	// Bump checkpoint to signal we started processing
	checkpointMap["current_step"] = currentStep + 1
	latestLog.Checkpoint = checkpointMap
	if err := e.ingestionTaskLogDAO.Update(latestLog); err != nil {
		common.Error(fmt.Sprintf("Failed to persist checkpoint for task %s", task.ID), err)
	}
	_ = totalStep // unused in this temporary integration

	select {
	case <-ctx.Done():
		common.Info(fmt.Sprintf("Task %s cancelled", task.ID))
		return
	default:
	}
	if err := e.runDocumentTask(ctx, task); err != nil {
		common.Error(fmt.Sprintf("Task %s failed", task.ID), err)
		if uErr := e.ingestionTaskDAO.UpdateStatus(task.ID, common.FAILED); uErr != nil {
			common.Error(fmt.Sprintf("Failed to set task %s to FAILED", task.ID), uErr)
		}
		return
	}

	err = e.ingestionTaskDAO.UpdateStatus(task.ID, common.COMPLETED)
	if err != nil {
		common.Error(fmt.Sprintf("Task %s update status failed", task.ID), err)
		return
	}

	common.Info(fmt.Sprintf("Task %s completed", task.ID))
}

// FIXME: should remove
func (e *Ingestor) getPipelineID(tenantID string) (string, error) {
	_, file, _, ok := goruntime.Caller(0)
	if !ok {
		return "", fmt.Errorf("failed to get runtime base dir")
	}
	base := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))

	// FIXME: now use mocked pipeline, need to change later
	templatePath := filepath.Join(base, "agent", "templates", "ingestion_pipeline_general.json")
	common.Warn(fmt.Sprintf("use mocked DSL, templatePath: %s", templatePath))
	templateBytes, err := os.ReadFile(templatePath)
	if err != nil {
		return "", err
	}

	var templateDSL entity.JSONMap
	if err := json.Unmarshal(templateBytes, &templateDSL); err != nil {
		return "", err
	}

	des := "mock up DSL for integration test"
	title := "mock up DSL"
	ID := strings.ReplaceAll(uuid.New().String(), "-", "")[:32]
	userCanvas := entity.UserCanvas{UserID: tenantID, DSL: templateDSL,
		Description: &des, Title: &title, ID: ID, Permission: "me"}

	if err := dao.NewUserCanvasDAO().Create(&userCanvas); err != nil {
		return "", err
	}

	return ID, nil
}

func (e *Ingestor) defaultRunDocumentTask(ctx context.Context, ingestionTask *entity.IngestionTask) error {
	docTaskCtx, err := taskpkg.LoadFromIngestionTask(ingestionTask)
	if err != nil {
		return fmt.Errorf("load task context for %s: %w", ingestionTask.ID, err)
	}

	if docTaskCtx.PipelineID, err = e.getPipelineID(docTaskCtx.Tenant.ID); err != nil {
		return fmt.Errorf("get pipeline ID for %s: %w", ingestionTask.ID, err)
	}

	return taskpkg.NewTaskHandler(docTaskCtx).Handle()
}

// Stop gracefully shuts down the ingestor
func (e *Ingestor) Stop() {
	common.Info(fmt.Sprintf("Stopping ingestor %s", e.id))
	e.cancel()

	// Wait for all workers to finish (they exit on ctx.Done())
	e.workerWg.Wait()
	common.Info("All tasks completed")
}
