package etl

import (
	"fmt"
	"sync"
	"time"
)

// RunKind identifies which ETL job holds its own lock (incremental vs effort reconcile can overlap).
type RunKind int

const (
	RunIncremental RunKind = iota
	RunEffortDailyReconcile
)

// ActiveRun describes a job currently holding its lock.
type ActiveRun struct {
	Kind      string    `json:"kind"`
	Label     string    `json:"label"`
	StartedAt time.Time `json:"started_at"`
}

var (
	coordMu              sync.Mutex
	incrementalHeld      bool
	effortReconcileHeld  bool
	incrementalStartedAt time.Time
	effortStartedAt      time.Time
)

// TryAcquire returns true when this job kind is not already running.
func TryAcquire(kind RunKind) bool {
	coordMu.Lock()
	defer coordMu.Unlock()
	now := time.Now()
	switch kind {
	case RunIncremental:
		if incrementalHeld {
			return false
		}
		incrementalHeld = true
		incrementalStartedAt = now
	case RunEffortDailyReconcile:
		if effortReconcileHeld {
			return false
		}
		effortReconcileHeld = true
		effortStartedAt = now
	default:
		return false
	}
	return true
}

// Release clears the lock for the given kind.
func Release(kind RunKind) {
	coordMu.Lock()
	defer coordMu.Unlock()
	switch kind {
	case RunIncremental:
		incrementalHeld = false
	case RunEffortDailyReconcile:
		effortReconcileHeld = false
	}
}

// ActiveRunKind returns a label for backward compatibility (incremental preferred if both).
func ActiveRunKind() string {
	runs := ActiveRuns()
	if len(runs) == 0 {
		return ""
	}
	return runs[0].Kind
}

// ActiveRuns lists all in-flight ETL jobs.
func ActiveRuns() []ActiveRun {
	coordMu.Lock()
	defer coordMu.Unlock()
	var out []ActiveRun
	if incrementalHeld {
		out = append(out, ActiveRun{
			Kind:      "incremental",
			Label:     "增量同步",
			StartedAt: incrementalStartedAt,
		})
	}
	if effortReconcileHeld {
		out = append(out, ActiveRun{
			Kind:      "effort_daily_reconcile",
			Label:     "回刷报工",
			StartedAt: effortStartedAt,
		})
	}
	return out
}

// IsEffortReconcileRunning reports whether an effort reconcile job holds the lock.
func IsEffortReconcileRunning() bool {
	coordMu.Lock()
	defer coordMu.Unlock()
	return effortReconcileHeld
}

// BusyKindFor blocks acquiring kind — only the same kind blocks itself now.
func BusyKindFor(kind RunKind) string {
	coordMu.Lock()
	defer coordMu.Unlock()
	switch kind {
	case RunIncremental:
		if incrementalHeld {
			return "incremental"
		}
	case RunEffortDailyReconcile:
		if effortReconcileHeld {
			return "effort_daily_reconcile"
		}
	}
	return ""
}

// ActiveRunKindLabel returns a human label for API errors.
func ActiveRunKindLabel(kind string) string {
	switch kind {
	case "incremental":
		return "增量同步"
	case "effort_daily_reconcile":
		return "回刷报工"
	default:
		return kind
	}
}

// FormatBusyKinds joins active run labels for error messages.
func FormatBusyKinds(kinds []string) string {
	if len(kinds) == 0 {
		return ""
	}
	labels := make([]string, 0, len(kinds))
	for _, k := range kinds {
		labels = append(labels, ActiveRunKindLabel(k))
	}
	if len(labels) == 1 {
		return labels[0]
	}
	return fmt.Sprintf("%v", labels)
}
