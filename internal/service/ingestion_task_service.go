package service

import (
	"errors"
	"fmt"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"

	"gorm.io/gorm"
)

type InvalidTaskTransitionError struct {
	TaskID string
	From   string
	To     string
}

func (e *InvalidTaskTransitionError) Error() string {
	return fmt.Sprintf("task %s status cannot transition from %s to %s", e.TaskID, e.From, e.To)
}

type TaskStatusConflictError struct {
	TaskID        string
	ExpectedFrom  string
	AttemptedTo   string
	ActualCurrent string
}

func (e *TaskStatusConflictError) Error() string {
	return fmt.Sprintf("task %s status conflict: expected %s -> %s, actual current %s", e.TaskID, e.ExpectedFrom, e.AttemptedTo, e.ActualCurrent)
}

type IngestionTaskService struct {
	documentDAO         *dao.DocumentDAO
	userDAO             *dao.UserDAO
	ingestionTaskDAO    *dao.IngestionTaskDAO
	ingestionTaskLogDAO *dao.IngestionTaskLogDAO
	taskPublisher       TaskPublisher
}

func NewIngestionTaskService() *IngestionTaskService {
	return &IngestionTaskService{
		documentDAO:         dao.NewDocumentDAO(),
		userDAO:             dao.NewUserDAO(),
		ingestionTaskDAO:    dao.NewIngestionTaskDAO(),
		ingestionTaskLogDAO: dao.NewIngestionTaskLogDAO(),
		taskPublisher:       NewMessageQueueTaskPublisher(),
	}
}

func (s *IngestionTaskService) SetTaskPublisher(taskPublisher TaskPublisher) {
	if taskPublisher == nil {
		return
	}
	s.taskPublisher = taskPublisher
}

func (s *IngestionTaskService) ListByUser(userID string, datasetID *string, page, pageSize int) ([]*entity.IngestionTask, error) {
	if datasetID == nil {
		return s.ingestionTaskDAO.ListByUserID(userID, page, pageSize)
	}
	return s.ingestionTaskDAO.ListByUserIDAndDatasetID(userID, *datasetID, page, pageSize)
}

func (s *IngestionTaskService) CreateForDocuments(datasetID, userID string, docIDs []string) ([]*ParseDocumentResponse, error) {
	uniqueDocIDs := common.Deduplicate(docIDs)
	if len(uniqueDocIDs) == 0 {
		return nil, fmt.Errorf("no documents to parse")
	}

	responses := make([]*ParseDocumentResponse, 0, len(uniqueDocIDs))
	for _, docID := range uniqueDocIDs {
		doc, err := s.documentDAO.GetByID(docID)
		if err != nil {
			responses = append(responses, &ParseDocumentResponse{
				DocumentID: docID,
				Result:     err.Error(),
			})
			continue
		}
		if doc == nil {
			responses = append(responses, &ParseDocumentResponse{
				DocumentID: docID,
				Result:     "no such document",
			})
			continue
		}

		task := &entity.IngestionTask{
			DocumentID: docID,
			UserID:     userID,
			DatasetID:  datasetID,
			Schema:     nil,
			Status:     common.CREATED,
		}
		task, err = s.CreateAndEnqueue(task)
		if err != nil {
			responses = append(responses, &ParseDocumentResponse{
				DocumentID: docID,
				Result:     err.Error(),
			})
			continue
		}

		responses = append(responses, &ParseDocumentResponse{
			DocumentID: docID,
			Result:     fmt.Sprintf("task_id: %s", task.ID),
		})
	}
	return responses, nil
}

func (s *IngestionTaskService) RequestStopMany(tasks []string, ownerUserID *string) ([]*entity.IngestionTask, error) {
	taskResponses := make([]*entity.IngestionTask, 0, len(tasks))
	for _, taskID := range tasks {
		if ownerUserID != nil {
			task, err := s.getTask(taskID)
			if err != nil {
				return nil, err
			}
			if task.UserID != *ownerUserID {
				return nil, errors.New("task does not belong to the user")
			}
		}
		task, err := s.RequestStop(taskID)
		if err != nil {
			return nil, err
		}
		taskResponses = append(taskResponses, task)
	}
	return taskResponses, nil
}

func (s *IngestionTaskService) RemoveMany(tasks []string, ownerUserID *string) ([]map[string]string, error) {
	deletedTasks := make([]map[string]string, 0, len(tasks))
	for _, taskID := range tasks {
		taskRecord := map[string]string{"task_id": taskID}
		if _, err := s.Remove(taskID, ownerUserID); err != nil {
			taskRecord["remove"] = fmt.Sprintf("fail: %s", err.Error())
		} else {
			taskRecord["remove"] = "success"
		}
		deletedTasks = append(deletedTasks, taskRecord)
	}
	return deletedTasks, nil
}

func (s *IngestionTaskService) ListAllForAdmin() ([]map[string]interface{}, error) {
	ingestionTasks, err := s.ingestionTaskDAO.GetAllTasks(0, 0)
	if err != nil {
		return nil, err
	}

	showTasks := make([]map[string]interface{}, 0, len(ingestionTasks))
	for _, task := range ingestionTasks {
		user, err := s.userDAO.GetByTenantID(task.UserID)
		if err != nil {
			return nil, err
		}

		showTask := map[string]interface{}{
			"id":          task.ID,
			"user_id":     task.UserID,
			"user":        user.Email,
			"document_id": task.DocumentID,
			"status":      task.Status,
		}

		latestLog, err := s.ingestionTaskLogDAO.LatestLogByTaskID(task.ID)
		if err == nil && latestLog != nil && latestLog.Checkpoint != nil {
			if step, ok := latestLog.Checkpoint["current_step"].(float64); ok {
				showTask["step"] = int(step)
			}
		}

		showTasks = append(showTasks, showTask)
	}
	return showTasks, nil
}

func (s *IngestionTaskService) StartRunning(taskID string) (*entity.IngestionTask, error) {
	task, err := s.getTask(taskID)
	if err != nil {
		return nil, err
	}
	switch task.Status {
	case common.CREATED:
		return s.transition(taskID, common.RUNNING)
	case common.STOPPING:
		return s.transition(taskID, common.STOPPED)
	case common.RUNNING, common.COMPLETED, common.STOPPED, common.FAILED:
		return task, nil
	default:
		return task, fmt.Errorf("task %s has unsupported status %s", taskID, task.Status)
	}
}

func (s *IngestionTaskService) RequestStop(taskID string) (*entity.IngestionTask, error) {
	task, err := s.getTask(taskID)
	if err != nil {
		return nil, err
	}
	switch task.Status {
	case common.CREATED:
		return s.transition(taskID, common.STOPPED)
	case common.RUNNING:
		return s.transition(taskID, common.STOPPING)
	default:
		return task, nil
	}
}

func (s *IngestionTaskService) MarkCompleted(taskID string) error {
	_, err := s.transition(taskID, common.COMPLETED)
	return err
}

func (s *IngestionTaskService) MarkFailed(taskID string) error {
	_, err := s.transition(taskID, common.FAILED)
	return err
}

func (s *IngestionTaskService) Remove(taskID string, userID *string) (*dao.TaskInfo, error) {
	return s.ingestionTaskDAO.Delete(taskID, userID)
}

func (s *IngestionTaskService) getTask(taskID string) (*entity.IngestionTask, error) {
	task, err := s.ingestionTaskDAO.GetByID(taskID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, common.ErrTaskNotFound
		}
		return nil, err
	}
	return task, nil
}

func validateTransition(from, to string) error {
	switch from {
	case common.CREATED:
		if to == common.RUNNING || to == common.STOPPED {
			return nil
		}
	case common.RUNNING:
		if to == common.STOPPING || to == common.COMPLETED || to == common.FAILED {
			return nil
		}
	case common.STOPPING:
		if to == common.STOPPED {
			return nil
		}
	case common.FAILED, common.STOPPED:
		if to == common.CREATED {
			return nil
		}
	}
	return &InvalidTaskTransitionError{From: from, To: to}
}

func (s *IngestionTaskService) newTaskStatusConflictError(taskID, expectedFrom, attemptedTo string) error {
	current, err := s.getTask(taskID)
	if err != nil {
		return err
	}
	return &TaskStatusConflictError{
		TaskID:        taskID,
		ExpectedFrom:  expectedFrom,
		AttemptedTo:   attemptedTo,
		ActualCurrent: current.Status,
	}
}

func (s *IngestionTaskService) transition(taskID string, to string) (*entity.IngestionTask, error) {
	task, err := s.getTask(taskID)
	if err != nil {
		return nil, err
	}
	if err := validateTransition(task.Status, to); err != nil {
		var transitionErr *InvalidTaskTransitionError
		if errors.As(err, &transitionErr) {
			return task, &InvalidTaskTransitionError{TaskID: taskID, From: transitionErr.From, To: transitionErr.To}
		}
		return task, err
	}
	updated, err := s.ingestionTaskDAO.UpdateStatusIfCurrent(taskID, task.Status, to)
	if err != nil {
		return nil, err
	}
	if !updated {
		return nil, s.newTaskStatusConflictError(taskID, task.Status, to)
	}
	task.Status = to
	return task, nil
}

func (s *IngestionTaskService) CreateAndEnqueue(task *entity.IngestionTask) (*entity.IngestionTask, error) {
	existing, err := s.ingestionTaskDAO.GetByDocumentID(task.DocumentID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		switch existing.Status {
		case common.FAILED, common.STOPPED:
			originalStatus := existing.Status
			existing, err = s.transition(existing.ID, common.CREATED)
			if err != nil {
				return nil, err
			}
			if err := s.enqueueTask(existing.ID); err != nil {
				if rollbackErr := s.rollbackRetriedTask(existing.ID, originalStatus); rollbackErr != nil {
					return nil, fmt.Errorf("enqueue task %s: %w (rollback failed: %v)", existing.ID, err, rollbackErr)
				}
				return nil, err
			}
			return existing, nil
		default:
			return nil, fmt.Errorf("document id %s already exists, status: %s, task id: %s", task.DocumentID, existing.Status, existing.ID)
		}
	}
	created, err := s.ingestionTaskDAO.Create(task)
	if err != nil {
		return nil, err
	}
	if err := s.enqueueTask(created.ID); err != nil {
		if rollbackErr := s.rollbackCreatedTask(created.ID); rollbackErr != nil {
			return nil, fmt.Errorf("enqueue task %s: %w (rollback failed: %v)", created.ID, err, rollbackErr)
		}
		return nil, err
	}
	return created, nil
}

func (s *IngestionTaskService) rollbackRetriedTask(taskID, status string) error {
	updated, err := s.ingestionTaskDAO.UpdateStatusIfCurrent(taskID, common.CREATED, status)
	if err != nil {
		return err
	}
	if !updated {
		return s.newTaskStatusConflictError(taskID, common.CREATED, status)
	}
	return nil
}

func (s *IngestionTaskService) rollbackCreatedTask(taskID string) error {
	_, err := s.ingestionTaskDAO.Delete(taskID, nil)
	return err
}

func (s *IngestionTaskService) enqueueTask(taskID string) error {
	taskMessage := common.TaskMessage{
		TaskID:   taskID,
		TaskType: common.TaskTypeIngestionTask,
	}
	return s.taskPublisher.PublishTaskMessage("tasks.RAGFLOW", taskMessage)
}
