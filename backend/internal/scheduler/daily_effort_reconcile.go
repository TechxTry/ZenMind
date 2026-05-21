package scheduler

import (
	"context"
	"log"
	"time"
	"zenmind/internal/config"
	"zenmind/internal/etl"
)

// StartDailyEffortReconcile runs a wide date-window effort sync once per day at EffortReconcileHour (local).
// It does not run concurrently with the incremental RunAll pipeline.
func StartDailyEffortReconcile(ctx context.Context) {
	if !config.Global.EffortReconcileEnabled {
		log.Printf("[scheduler] daily effort reconcile disabled (EFFORT_RECONCILE_ENABLED=false)")
		return
	}
	hour := config.ClampEffortReconcileHour(config.Global.EffortReconcileHour)
	days := config.ClampEffortReconcileDays(config.Global.EffortReconcileDays)
	log.Printf("[scheduler] daily effort reconcile enabled: at %02d:00 local, window=%d days", hour, days)

	go func() {
		for {
			wait := durationUntilLocalHour(hour, 0)
			timer := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				return
			case <-timer.C:
				log.Printf("[scheduler] daily effort reconcile starting (window=%d days)", days)
				etl.RunEffortsDailyReconcile()
			}
		}
	}()
}

func durationUntilLocalHour(hour, minute int) time.Duration {
	now := time.Now()
	loc := now.Location()
	next := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, loc)
	if !next.After(now) {
		next = next.Add(24 * time.Hour)
	}
	return next.Sub(now)
}
