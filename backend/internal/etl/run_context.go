package etl

import (
	"fmt"
	"log"
	"time"
	"zenmind/internal/db"
	"zenmind/internal/models"
)

// TriggerSource identifies who started an ETL job.
type TriggerSource string

const (
	TriggerAuto      TriggerSource = "auto"
	TriggerManual    TriggerSource = "manual"
	TriggerScheduled TriggerSource = "scheduled"
)

// RunContext carries trigger metadata for sync run logs.
type RunContext struct {
	Source TriggerSource
	Actor  string // admin username for manual triggers
}

func (ctx RunContext) trigger() string {
	if ctx.Source == "" {
		return db.SyncTriggerAuto
	}
	return string(ctx.Source)
}

func logSyncSkipped(jobType string, ctx RunContext, busyKind string) {
	now := time.Now()
	_ = db.InsertSyncRunLog(db.SyncRunLogInput{
		JobType:       jobType,
		TriggerSource: ctx.trigger(),
		Status:        db.SyncStatusSkipped,
		Message:       db.SkippedSyncMessage(busyKind),
		ActorUsername: ctx.Actor,
		StartedAt:     now,
		FinishedAt:    &now,
		DurationMs:    ptrInt64(0),
	})
}

func runWithSyncLog(jobType string, lockKind RunKind, ctx RunContext, work func() (message string, meta models.JSONB)) {
	if !TryAcquire(lockKind) {
		busy := BusyKindFor(lockKind)
		log.Printf("[etl] %s skipped: %s is running", jobType, busy)
		logSyncSkipped(jobType, ctx, busy)
		return
	}
	defer Release(lockKind)

	logID, err := db.BeginSyncRunLog(jobType, ctx.trigger(), ctx.Actor, nil)
	if err != nil {
		log.Printf("[etl] sync run log begin failed: %v", err)
	}

	status := db.SyncStatusSuccess
	var message string
	var meta models.JSONB

	defer func() {
		if r := recover(); r != nil {
			status = db.SyncStatusFailed
			message = fmt.Sprintf("%v", r)
			finishSyncRunLog(logID, jobType, ctx, status, message, meta)
			panic(r)
		}
		finishSyncRunLog(logID, jobType, ctx, status, message, meta)
	}()

	message, meta = work()
}

func finishSyncRunLog(logID int64, jobType string, ctx RunContext, status, message string, meta models.JSONB) {
	if logID > 0 {
		if err := db.FinishSyncRunLog(logID, status, message, meta); err != nil {
			log.Printf("[etl] sync run log finish failed: %v", err)
		}
		return
	}
	now := time.Now()
	_ = db.InsertSyncRunLog(db.SyncRunLogInput{
		JobType:       jobType,
		TriggerSource: ctx.trigger(),
		Status:        status,
		Message:       message,
		ActorUsername: ctx.Actor,
		Metadata:      meta,
		StartedAt:     now,
		FinishedAt:    &now,
	})
}

func ptrInt64(n int64) *int64 { return &n }
