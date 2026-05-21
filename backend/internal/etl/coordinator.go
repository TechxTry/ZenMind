package etl

import (
	"fmt"
	"sync"
)

// RunKind identifies which ETL job holds the global lock.
type RunKind int

const (
	RunIncremental RunKind = iota
	RunEffortDailyReconcile
)

var (
	coordMu     sync.Mutex
	coordActive RunKind
	coordHeld   bool
)

// TryAcquire returns true when no other ETL job is running.
func TryAcquire(kind RunKind) bool {
	coordMu.Lock()
	defer coordMu.Unlock()
	if coordHeld {
		return false
	}
	coordHeld = true
	coordActive = kind
	return true
}

// Release clears the lock for the given kind (no-op if kind mismatches).
func Release(kind RunKind) {
	coordMu.Lock()
	defer coordMu.Unlock()
	if coordHeld && coordActive == kind {
		coordHeld = false
	}
}

// ActiveRunKind returns a label for the running job, or "" if idle.
func ActiveRunKind() string {
	coordMu.Lock()
	defer coordMu.Unlock()
	if !coordHeld {
		return ""
	}
	switch coordActive {
	case RunIncremental:
		return "incremental"
	case RunEffortDailyReconcile:
		return "effort_daily_reconcile"
	default:
		return fmt.Sprintf("unknown(%d)", coordActive)
	}
}
