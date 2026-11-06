package syncer

import (
	"testing"
	"time"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

func testTaskDelayTask(taskType string, freq int64, updateDate *time.Time) dao.ScheduledSyncTask {
	task := dao.ScheduledSyncTask{ConnectorConfig: entity.JSONMap{}}
	task.TaskType = taskType
	task.ConnectorRefreshFreq = freq
	task.ConnectorPruneFreq = freq
	task.UpdateDate = updateDate
	return task
}

// TestTaskDelayNonPositiveFreqDoesNotReschedule pins the hot-loop fix: a
// task whose connector frequency is zero or negative must not be
// republished once it has run before. Only a never-run task runs once.
func TestTaskDelayNonPositiveFreqDoesNotReschedule(t *testing.T) {
	scheduler := NewScheduler(nil, nil)
	past := time.Now().Add(-time.Hour)

	for _, freq := range []int64{0, -5} {
		task := testTaskDelayTask(dao.TaskTypeSync, freq, &past)
		if delay, schedule := scheduler.taskDelay(task, time.Now()); schedule {
			t.Fatalf("sync freq=%d with past run: schedule=true (delay=%v), want false", freq, delay)
		}

		prune := testTaskDelayTask(dao.TaskTypePrune, freq, &past)
		prune.ConnectorConfig = entity.JSONMap{"sync_deleted_files": true}
		if delay, schedule := scheduler.taskDelay(prune, time.Now()); schedule {
			t.Fatalf("prune freq=%d with past run: schedule=true (delay=%v), want false", freq, delay)
		}
	}
}

// TestTaskDelayNeverRunPublishesOnce ensures a never-run task still runs
// exactly once, even with a non-positive frequency.
func TestTaskDelayNeverRunPublishesOnce(t *testing.T) {
	scheduler := NewScheduler(nil, nil)

	task := testTaskDelayTask(dao.TaskTypeSync, 0, nil)
	delay, schedule := scheduler.taskDelay(task, time.Now())
	if !schedule {
		t.Fatal("never-run sync task: schedule=false, want true")
	}
	if delay != 0 {
		t.Fatalf("never-run sync task: delay=%v, want 0", delay)
	}
}

// TestTaskDelayPositiveFreqSchedulesByUpdateDate guards the normal path:
// overdue tasks are due now, recently run tasks wait out their interval.
func TestTaskDelayPositiveFreqSchedulesByUpdateDate(t *testing.T) {
	scheduler := NewScheduler(nil, nil)
	now := time.Now()

	overdue := now.Add(-10 * time.Minute)
	task := testTaskDelayTask(dao.TaskTypeSync, 5, &overdue)
	delay, schedule := scheduler.taskDelay(task, now)
	if !schedule {
		t.Fatal("overdue sync task: schedule=false, want true")
	}
	if delay > 0 {
		t.Fatalf("overdue sync task: delay=%v, want <= 0", delay)
	}

	recent := now
	waiting := testTaskDelayTask(dao.TaskTypeSync, 60, &recent)
	delay, schedule = scheduler.taskDelay(waiting, now)
	if !schedule {
		t.Fatal("recent sync task: schedule=false, want true")
	}
	if delay <= 50*time.Minute {
		t.Fatalf("recent sync task: delay=%v, want > 50m", delay)
	}
}
