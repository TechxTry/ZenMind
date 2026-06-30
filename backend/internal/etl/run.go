package etl

import (
	"fmt"
	"log"
	"sync/atomic"
	"zenmind/internal/config"
	"zenmind/internal/db"
	"zenmind/internal/models"
)

// historiesActionsEveryNthRun: SyncActions/SyncHistories are heavy; run them every N incremental cycles.
const historiesActionsEveryNthRun uint64 = 4

var incrementalPipelineSeq uint64

// RunAll runs the incremental ETL pipeline (automatic trigger).
func RunAll() {
	RunAllWith(RunContext{Source: TriggerAuto})
}

// RunAllWith runs incremental ETL and records a sync run log entry.
func RunAllWith(ctx RunContext) {
	runWithSyncLog(db.SyncJobIncremental, RunIncremental, ctx, func() (string, models.JSONB) {
		runAllPipeline()
		return "增量同步完成", nil
	})
}

func runAllPipeline() {
	seq := atomic.AddUint64(&incrementalPipelineSeq, 1)
	runHeavy := seq == 1 || seq%historiesActionsEveryNthRun == 0
	log.Printf("[etl] starting sync pipeline (seq=%d, actions/histories=%v)", seq, runHeavy)

	SyncUsers()
	SyncTasks()
	SyncStories()
	SyncBugs()
	SyncEfforts()
	SyncPrograms()
	SyncProjects()
	SyncProductLines()
	SyncProducts()
	SyncExecutions()
	if runHeavy {
		SyncActions()
		SyncHistories()
	}
	log.Printf("[etl] sync pipeline complete (seq=%d)", seq)
}

// RunEffortsDailyReconcile runs effort date-window reconcile (scheduled trigger).
func RunEffortsDailyReconcile() {
	RunEffortsDailyReconcileWith(RunContext{Source: TriggerScheduled})
}

// RunEffortsDailyReconcileWith runs effort reconcile and records a sync run log entry.
func RunEffortsDailyReconcileWith(ctx RunContext) {
	days := config.ClampEffortReconcileDays(config.Global.EffortReconcileDays)
	runWithSyncLog(db.SyncJobEffortReconcile, RunEffortDailyReconcile, ctx, func() (string, models.JSONB) {
		n := SyncEffortsDailyReconcile(days)
		meta := models.JSONB{
			"days":     days,
			"upserted": n,
		}
		return fmt.Sprintf("回刷报工完成，写入 %d 条", n), meta
	})
}
