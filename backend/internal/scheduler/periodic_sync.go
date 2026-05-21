package scheduler

import (
	"context"
	"log"
	"time"
	"zenmind/internal/config"
	"zenmind/internal/etl"
)

var syncIntervalReset = make(chan struct{}, 1)

// StartPeriodicETL runs etl.RunAll on a timer; interval comes from config.Global.SyncIntervalMinutes.
// NotifySyncIntervalChanged restarts the wait so a new interval applies without restarting the process.
func StartPeriodicETL(ctx context.Context) {
	go func() {
		for {
			mins := config.ClampSyncIntervalMinutes(config.Global.SyncIntervalMinutes)
			dur := time.Duration(mins) * time.Minute
			timer := time.NewTimer(dur)
			select {
			case <-ctx.Done():
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				return
			case <-syncIntervalReset:
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				continue
			case <-timer.C:
				log.Printf("[scheduler] periodic ETL (every %dm)", mins)
				etl.RunAll()
			}
		}
	}()
}

// NotifySyncIntervalChanged wakes the periodic loop so it picks up the new interval.
func NotifySyncIntervalChanged() {
	select {
	case syncIntervalReset <- struct{}{}:
	default:
	}
}
