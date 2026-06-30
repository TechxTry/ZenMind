package db

import (
	"fmt"
	"time"
	"zenmind/internal/models"
)

const (
	SyncJobIncremental     = "incremental"
	SyncJobEffortReconcile = "effort_reconcile"
	SyncTriggerAuto        = "auto"
	SyncTriggerManual      = "manual"
	SyncTriggerScheduled   = "scheduled"
	SyncStatusRunning      = "running"
	SyncStatusSuccess      = "success"
	SyncStatusSkipped      = "skipped"
	SyncStatusFailed       = "failed"
)

// SyncRunLogInput starts or completes a sync run log row.
type SyncRunLogInput struct {
	JobType       string
	TriggerSource string
	Status        string
	Message       string
	ActorUsername string
	Metadata      models.JSONB
	StartedAt     time.Time
	FinishedAt    *time.Time
	DurationMs    *int64
}

// InsertSyncRunLog writes one sync run record.
func InsertSyncRunLog(in SyncRunLogInput) error {
	if PG == nil {
		return nil
	}
	row := models.SyncRunLog{
		JobType:       in.JobType,
		TriggerSource: in.TriggerSource,
		Status:        in.Status,
		Message:       in.Message,
		ActorUsername: in.ActorUsername,
		Metadata:      in.Metadata,
		StartedAt:     in.StartedAt,
		FinishedAt:    in.FinishedAt,
		DurationMs:    in.DurationMs,
	}
	if row.Metadata == nil {
		row.Metadata = models.JSONB{}
	}
	if row.StartedAt.IsZero() {
		row.StartedAt = time.Now()
	}
	return PG.Create(&row).Error
}

// BeginSyncRunLog inserts a running row and returns its id.
func BeginSyncRunLog(jobType, triggerSource, actor string, metadata models.JSONB) (int64, error) {
	if PG == nil {
		return 0, nil
	}
	row := models.SyncRunLog{
		JobType:       jobType,
		TriggerSource: triggerSource,
		Status:        SyncStatusRunning,
		ActorUsername: actor,
		Metadata:      metadata,
		StartedAt:     time.Now(),
	}
	if row.Metadata == nil {
		row.Metadata = models.JSONB{}
	}
	if err := PG.Create(&row).Error; err != nil {
		return 0, err
	}
	return row.ID, nil
}

// FinishSyncRunLog updates a running row to a terminal status.
func FinishSyncRunLog(id int64, status, message string, metadata models.JSONB) error {
	if PG == nil || id <= 0 {
		return nil
	}
	finished := time.Now()
	var row models.SyncRunLog
	if err := PG.First(&row, id).Error; err != nil {
		return err
	}
	dur := finished.Sub(row.StartedAt).Milliseconds()
	updates := map[string]any{
		"status":      status,
		"message":     message,
		"finished_at": finished,
		"duration_ms": dur,
	}
	if metadata != nil {
		merged := models.JSONB{}
		for k, v := range row.Metadata {
			merged[k] = v
		}
		for k, v := range metadata {
			merged[k] = v
		}
		updates["metadata"] = merged
	}
	return PG.Model(&models.SyncRunLog{}).Where("id = ?", id).Updates(updates).Error
}

// RepairStaleSyncRunLogs marks long-stuck "running" rows as failed (e.g. after process crash).
func RepairStaleSyncRunLogs(maxAge time.Duration) (int64, error) {
	if PG == nil {
		return 0, nil
	}
	cutoff := time.Now().Add(-maxAge)
	res := PG.Model(&models.SyncRunLog{}).
		Where("status = ? AND started_at < ?", SyncStatusRunning, cutoff).
		Updates(map[string]any{
			"status":      SyncStatusFailed,
			"message":     "执行超时或进程中断（已自动标记失败）",
			"finished_at": time.Now(),
		})
	return res.RowsAffected, res.Error
}

// ListSyncRunLogs returns recent sync logs newest first.
func ListSyncRunLogs(offset, limit int) ([]models.SyncRunLog, int64, error) {
	if PG == nil {
		return nil, 0, nil
	}
	var total int64
	if err := PG.Model(&models.SyncRunLog{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []models.SyncRunLog
	q := PG.Order("started_at DESC, id DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	if offset > 0 {
		q = q.Offset(offset)
	}
	if err := q.Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	for i := range rows {
		EnrichSyncRunLog(&rows[i])
	}
	return rows, total, nil
}

// EnrichSyncRunLog fills display fields for API responses.
func EnrichSyncRunLog(row *models.SyncRunLog) {
	row.DisplayName = SyncRunDisplayName(row.JobType, row.TriggerSource)
	row.StatusLabel = SyncRunStatusLabel(row.Status)
}

func SyncRunDisplayName(jobType, trigger string) string {
	switch jobType {
	case SyncJobIncremental:
		switch trigger {
		case SyncTriggerManual:
			return "手动增量同步"
		default:
			return "自动增量同步"
		}
	case SyncJobEffortReconcile:
		switch trigger {
		case SyncTriggerManual:
			return "手动回刷报工"
		case SyncTriggerScheduled:
			return "定时回刷报工"
		default:
			return "回刷报工"
		}
	default:
		return jobType
	}
}

func SyncRunStatusLabel(status string) string {
	switch status {
	case SyncStatusSuccess:
		return "成功"
	case SyncStatusSkipped:
		return "已跳过"
	case SyncStatusFailed:
		return "失败"
	case SyncStatusRunning:
		return "进行中"
	default:
		return status
	}
}

// SkippedSyncMessage formats the lock-contention skip reason.
func SkippedSyncMessage(busyKind string) string {
	busy := SyncRunDisplayNameFromActiveKind(busyKind)
	if busy == "" {
		busy = busyKind
	}
	return fmt.Sprintf("另一同步任务正在执行：%s", busy)
}

func SyncRunDisplayNameFromActiveKind(activeKind string) string {
	switch activeKind {
	case "incremental":
		return "增量同步"
	case "effort_daily_reconcile":
		return "回刷报工"
	default:
		return activeKind
	}
}
