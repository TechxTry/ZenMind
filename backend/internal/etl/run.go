package etl

import (
	"log"
	"zenmind/internal/config"
)

// RunAll runs the incremental ETL pipeline. Skips when daily effort reconcile is in progress.
func RunAll() {
	if !TryAcquire(RunIncremental) {
		log.Printf("[etl] RunAll skipped: %s is running", ActiveRunKind())
		return
	}
	defer Release(RunIncremental)
	runAllPipeline()
}

func runAllPipeline() {
	log.Println("[etl] starting full sync pipeline")
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
	SyncActions()
	SyncHistories()
	log.Println("[etl] full sync pipeline complete")
}

// RunEffortsDailyReconcile re-syncs efforts in a calendar window (see SyncEffortsDailyReconcile).
// Skips when the incremental pipeline is running.
func RunEffortsDailyReconcile() {
	if !TryAcquire(RunEffortDailyReconcile) {
		log.Printf("[etl] daily effort reconcile skipped: %s is running", ActiveRunKind())
		return
	}
	defer Release(RunEffortDailyReconcile)
	SyncEffortsDailyReconcile(config.Global.EffortReconcileDays)
}
