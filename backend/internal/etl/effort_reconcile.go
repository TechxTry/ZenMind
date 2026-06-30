package etl

import (
	"log"
	"time"
	"zenmind/internal/config"
	"zenmind/internal/db"
	"zenmind/internal/models"
	"zenmind/internal/source"
)

const effortReconcileIDBatch = 500

// DefaultEffortReconcileDays matches workbench/analytics max range (6×30 days).
const DefaultEffortReconcileDays = 180

// effortReconcileDateRange returns [from, to] inclusive calendar bounds in local time.
func effortReconcileDateRange(dayWindow int) (time.Time, time.Time) {
	now := time.Now()
	to := time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, int(time.Second-time.Nanosecond), now.Location())
	fromDay := now.AddDate(0, 0, -dayWindow)
	from := time.Date(fromDay.Year(), fromDay.Month(), fromDay.Day(), 0, 0, 0, 0, now.Location())
	return from, to
}

func upsertEffortRows(rows []source.ZtEffort, now time.Time) int {
	for _, r := range rows {
		m := models.LocalEffort{
			ID: r.ID, Account: r.Account,
			WorkDate:   db.SafeTime(r.Date),
			Consumed:   r.Consumed,
			Work:       r.Work,
			ObjectType: r.ObjectType,
			ObjectID:   r.ObjectID,
			Deleted:    r.Deleted == "1",
			RawData:    db.RowToJSONB(r),
			SyncedAt:   now,
		}
		db.PG.Save(&m)
	}
	return len(rows)
}

// SyncEffortsDailyReconcile upserts all zt_effort rows whose work date falls in the window,
// plus any local_efforts in that window (covers date edits that moved a row out of the window).
// Returns the number of rows upserted.
func SyncEffortsDailyReconcile(dayWindow int) int {
	dayWindow = config.ClampEffortReconcileDays(dayWindow)
	cfg, ok := tableConfig("local_efforts")
	if !ok {
		return 0
	}
	ztDB := db.GetZentao()
	if ztDB == nil {
		log.Println("[etl] SyncEffortsDailyReconcile: Zentao DB not connected, skipping")
		return 0
	}

	from, to := effortReconcileDateRange(dayWindow)
	now := time.Now()

	var byDate []source.ZtEffort
	qDate := ztDB.Table(cfg.Source).Where("`date` >= ? AND `date` <= ?", from, to)
	if cfg.ExtraFilter != "" {
		qDate = qDate.Where(cfg.ExtraFilter)
	}
	if err := qDate.Find(&byDate).Error; err != nil {
		log.Printf("[etl] SyncEffortsDailyReconcile(%s) by-date query error: %v", cfg.Source, err)
		return 0
	}

	var localIDs []int64
	if err := db.PG.Model(&models.LocalEffort{}).
		Where("work_date >= ? AND work_date <= ?", from, to).
		Pluck("id", &localIDs).Error; err != nil {
		log.Printf("[etl] SyncEffortsDailyReconcile: local id list error: %v", err)
		return 0
	}

	byID := make(map[int64]source.ZtEffort, len(byDate)+len(localIDs))
	for _, r := range byDate {
		byID[r.ID] = r
	}

	for i := 0; i < len(localIDs); i += effortReconcileIDBatch {
		end := i + effortReconcileIDBatch
		if end > len(localIDs) {
			end = len(localIDs)
		}
		batch := localIDs[i:end]
		var rows []source.ZtEffort
		qID := ztDB.Table(cfg.Source).Where("id IN ?", batch)
		if cfg.ExtraFilter != "" {
			qID = qID.Where(cfg.ExtraFilter)
		}
		if err := qID.Find(&rows).Error; err != nil {
			log.Printf("[etl] SyncEffortsDailyReconcile(%s) by-id batch error: %v", cfg.Source, err)
			return 0
		}
		for _, r := range rows {
			byID[r.ID] = r
		}
	}

	merged := make([]source.ZtEffort, 0, len(byID))
	for _, r := range byID {
		merged = append(merged, r)
	}
	n := upsertEffortRows(merged, now)

	log.Printf(
		"[etl] SyncEffortsDailyReconcile(%s→%s): upserted=%d by_date=%d local_ids=%d window_days=%d range=%s..%s",
		cfg.Source, cfg.Name, n, len(byDate), len(localIDs), dayWindow,
		from.Format("2006-01-02"), to.Format("2006-01-02"),
	)
	return n
}
