// Package etl implements the ETL pipeline that pulls data from Zentao MySQL
// and upserts it into local PostgreSQL.
//
// Table names, watermark strategies, and extra filters are driven by
// config/etl_tables.yaml — no recompile needed when table names change.
package etl

import (
	"fmt"
	"log"
	"strings"
	"time"
	"zenmind/internal/config"
	"zenmind/internal/db"
	"zenmind/internal/models"
	"zenmind/internal/source"
)

// ---- watermark helpers ----

func getWatermark(targetTable string) time.Time {
	var wm models.SyncWatermark
	db.PG.Where("table_name = ?", targetTable).First(&wm)
	return wm.Watermark
}

func setWatermark(targetTable string, t time.Time, count int64) {
	db.PG.Save(&models.SyncWatermark{
		Table:     targetTable,
		Watermark: t,
		LastCount: count,
		UpdatedAt: time.Now(),
	})
}

// tableConfig returns the ETLTableConfig for a given target table name.
// Logs a warning and returns a zero value if not found or disabled.
func tableConfig(name string) (config.ETLTableConfig, bool) {
	cfg, ok := config.ETLTableMap[name]
	if !ok {
		log.Printf("[etl] WARNING: no config found for table %q in etl_tables.yaml", name)
		return config.ETLTableConfig{}, false
	}
	if !cfg.Enabled {
		log.Printf("[etl] table %q is disabled in etl_tables.yaml, skipping", name)
		return config.ETLTableConfig{}, false
	}
	return cfg, true
}

// buildSourceQuery builds a GORM query on the Zentao DB for a given table config.
func buildSourceQuery(cfg config.ETLTableConfig, watermark interface{}) interface{} {
	return cfg // query building is done inline per sync function for type safety
}

// whereClause builds the incremental WHERE clause based on watermark type.
func whereClause(cfg config.ETLTableConfig, watermark interface{}) (clause string, arg interface{}) {
	switch cfg.Watermark.Type {
	case config.WatermarkTime:
		return fmt.Sprintf("%s > ?", cfg.Watermark.Field), watermark
	case config.WatermarkID:
		return fmt.Sprintf("%s > ?", cfg.Watermark.Field), watermark
	default:
		return "", nil // full sync
	}
}

// ---- Sync functions ----
// Each function uses cfg.Source as the MySQL table name, making it
// configurable without recompilation.

// SyncUsers pulls zt_user (or configured source) and upserts into local_users.
func SyncUsers() {
	cfg, ok := tableConfig("local_users")
	if !ok {
		return
	}
	ztDB := db.GetZentao()
	if ztDB == nil {
		log.Println("[etl] SyncUsers: Zentao DB not connected, skipping")
		return
	}

	var rows []source.ZtUser
	q := ztDB.Table(cfg.Source)
	if cfg.ExtraFilter != "" {
		q = q.Where(cfg.ExtraFilter)
	}
	if err := q.Find(&rows).Error; err != nil {
		log.Printf("[etl] SyncUsers(%s) query error: %v", cfg.Source, err)
		return
	}

	now := time.Now()
	for _, r := range rows {
		m := models.LocalUser{
			ID:       r.ID,
			Account:  r.Account,
			Realname: r.Realname,
			Role:     r.Role,
			Deleted:  r.Deleted == "1",
			RawData:  db.RowToJSONB(r),
			SyncedAt: now,
		}
		db.PG.Save(&m)
	}
	log.Printf("[etl] SyncUsers(%s→%s): upserted %d rows", cfg.Source, cfg.Name, len(rows))
	setWatermark(cfg.Name, now, int64(len(rows)))
}

// ztTaskWatermarkTime returns a monotonic "change time" for watermark advancement.
// Some Zentao rows keep lastEditedDate NULL until the task is edited; openedDate is set on create.
func ztTaskWatermarkTime(r *source.ZtTask) *time.Time {
	if r == nil {
		return nil
	}
	if r.LastEditedDate != nil {
		return r.LastEditedDate
	}
	return r.OpenedDate
}

const taskNullLastEditedReconcileWindow = 30 * 24 * time.Hour

// ztBugWatermarkTime returns a monotonic "change time" for watermark advancement.
// Some Zentao bug rows keep lastEditedDate NULL until the bug is edited; openedDate is set on create.
func ztBugWatermarkTime(r *source.ZtBug) *time.Time {
	if r == nil {
		return nil
	}
	if r.LastEditedDate != nil {
		return r.LastEditedDate
	}
	return r.OpenedDate
}

const bugNullLastEditedReconcileWindow = 30 * 24 * time.Hour

// SyncTasks pulls source task table (default: zt_task) incrementally.
func SyncTasks() {
	cfg, ok := tableConfig("local_tasks")
	if !ok {
		return
	}
	ztDB := db.GetZentao()
	if ztDB == nil {
		return
	}

	wm := getWatermark(cfg.Name)
	var rows []source.ZtTask
	q := ztDB.Table(cfg.Source)
	// Incremental by lastEditedDate alone misses rows where lastEditedDate IS NULL (common for brand-new 未开始 tasks):
	// in SQL, "NULL > watermark" is unknown and the row is excluded forever after the first sync advances the watermark.
	if cfg.Watermark.Type == config.WatermarkTime && strings.TrimSpace(cfg.Watermark.Field) == "lastEditedDate" {
		reconcileFrom := time.Now().Add(-taskNullLastEditedReconcileWindow)
		q = q.Where(
			"(COALESCE(lastEditedDate, openedDate) > ?) OR (lastEditedDate IS NULL AND openedDate >= ?)",
			wm,
			reconcileFrom,
		)
	} else if clause, arg := whereClause(cfg, wm); clause != "" {
		q = q.Where(clause, arg)
	}
	if cfg.ExtraFilter != "" {
		q = q.Where(cfg.ExtraFilter)
	}
	if err := q.Find(&rows).Error; err != nil {
		log.Printf("[etl] SyncTasks(%s) query error: %v", cfg.Source, err)
		return
	}

	now := time.Now()
	maxEdited := wm
	for _, r := range rows {
		m := models.LocalTask{
			ID: r.ID, Name: r.Name, Type: r.Type, Status: r.Status,
			AssignedTo: r.AssignedTo, FinishedBy: r.FinishedBy,
			Estimate: r.Estimate, Consumed: r.Consumed,
			ExecutionID:    r.Execution,
			StoryID:        r.Story,
			OpenedDate:     db.SafeTime(r.OpenedDate),
			StartedDate:    db.SafeTime(r.StartedDate),
			AssignedDate:   db.SafeTime(r.AssignedDate),
			DeadlineDate:   db.SafeTime(r.Deadline),
			FinishedDate:   db.SafeTime(r.FinishedDate),
			ClosedDate:     db.SafeTime(r.ClosedDate),
			LastEditedDate: db.SafeTime(r.LastEditedDate),
			Deleted:        r.Deleted == "1",
			RawData:        db.RowToJSONB(r),
			SyncedAt:       now,
		}
		db.PG.Save(&m)
		if t := ztTaskWatermarkTime(&r); t != nil && t.After(maxEdited) {
			maxEdited = *t
		}
	}
	log.Printf("[etl] SyncTasks(%s→%s): upserted %d rows since %v", cfg.Source, cfg.Name, len(rows), wm)
	if len(rows) > 0 {
		setWatermark(cfg.Name, maxEdited, int64(len(rows)))
	}
}

// SyncStories pulls source story table (default: zt_story) incrementally.
func SyncStories() {
	cfg, ok := tableConfig("local_stories")
	if !ok {
		return
	}
	ztDB := db.GetZentao()
	if ztDB == nil {
		return
	}

	wm := getWatermark(cfg.Name)
	var rows []source.ZtStory
	q := ztDB.Table(cfg.Source)
	if clause, arg := whereClause(cfg, wm); clause != "" {
		q = q.Where(clause, arg)
	}
	if cfg.ExtraFilter != "" {
		q = q.Where(cfg.ExtraFilter)
	}
	if err := q.Find(&rows).Error; err != nil {
		log.Printf("[etl] SyncStories(%s) query error: %v", cfg.Source, err)
		return
	}

	now := time.Now()
	maxEdited := wm
	for _, r := range rows {
		m := models.LocalStory{
			ID: r.ID, Title: r.Title, Status: r.Status,
			AssignedTo:     r.AssignedTo,
			Estimate:       r.Estimate,
			ProductID:      r.Product,
			OpenedDate:     db.SafeTime(r.OpenedDate),
			ClosedDate:     db.SafeTime(r.ClosedDate),
			LastEditedDate: db.SafeTime(r.LastEditedDate),
			Deleted:        r.Deleted == "1",
			RawData:        db.RowToJSONB(r),
			SyncedAt:       now,
		}
		db.PG.Save(&m)
		if r.LastEditedDate != nil && r.LastEditedDate.After(maxEdited) {
			maxEdited = *r.LastEditedDate
		}
	}
	log.Printf("[etl] SyncStories(%s→%s): upserted %d rows", cfg.Source, cfg.Name, len(rows))
	if len(rows) > 0 {
		setWatermark(cfg.Name, maxEdited, int64(len(rows)))
	}
}

// SyncBugs pulls source bug table (default: zt_bug) incrementally.
func SyncBugs() {
	cfg, ok := tableConfig("local_bugs")
	if !ok {
		return
	}
	ztDB := db.GetZentao()
	if ztDB == nil {
		return
	}

	wm := getWatermark(cfg.Name)
	var rows []source.ZtBug
	q := ztDB.Table(cfg.Source)
	// Incremental by lastEditedDate alone misses rows where lastEditedDate IS NULL.
	// Use openedDate fallback plus a small reconcile window to avoid permanent misses.
	if cfg.Watermark.Type == config.WatermarkTime && strings.TrimSpace(cfg.Watermark.Field) == "lastEditedDate" {
		reconcileFrom := time.Now().Add(-bugNullLastEditedReconcileWindow)
		q = q.Where(
			"(COALESCE(lastEditedDate, openedDate) > ?) OR (lastEditedDate IS NULL AND openedDate >= ?)",
			wm,
			reconcileFrom,
		)
	} else if clause, arg := whereClause(cfg, wm); clause != "" {
		q = q.Where(clause, arg)
	}
	if cfg.ExtraFilter != "" {
		q = q.Where(cfg.ExtraFilter)
	}
	if err := q.Find(&rows).Error; err != nil {
		log.Printf("[etl] SyncBugs(%s) query error: %v", cfg.Source, err)
		return
	}

	now := time.Now()
	maxEdited := wm
	for _, r := range rows {
		m := models.LocalBug{
			ID: r.ID, Title: r.Title, Severity: r.Severity, Status: r.Status,
			AssignedTo: r.AssignedTo, ResolvedBy: r.ResolvedBy, Resolution: r.Resolution,
			ExecutionID:    r.Execution,
			StoryID:        r.Story,
			TaskID:         r.Task,
			OpenedDate:     db.SafeTime(r.OpenedDate),
			ResolvedDate:   db.SafeTime(r.ResolvedDate),
			ClosedDate:     db.SafeTime(r.ClosedDate),
			LastEditedDate: db.SafeTime(r.LastEditedDate),
			Deleted:        r.Deleted == "1",
			RawData:        db.RowToJSONB(r),
			SyncedAt:       now,
		}
		db.PG.Save(&m)
		if t := ztBugWatermarkTime(&r); t != nil && t.After(maxEdited) {
			maxEdited = *t
		}
	}
	log.Printf("[etl] SyncBugs(%s→%s): upserted %d rows", cfg.Source, cfg.Name, len(rows))
	if len(rows) > 0 {
		setWatermark(cfg.Name, maxEdited, int64(len(rows)))
	}
}

// SyncEfforts pulls source effort table (default: zt_effort) by ID watermark.
func SyncEfforts() {
	cfg, ok := tableConfig("local_efforts")
	if !ok {
		return
	}
	ztDB := db.GetZentao()
	if ztDB == nil {
		return
	}

	var lastID int64
	db.PG.Model(&models.LocalEffort{}).Select("COALESCE(MAX(id),0)").Scan(&lastID)

	// Effort 在禅道侧支持删除（deleted 字段/软删），但该表没有可靠的“最后修改时间”水位线字段。
	// 仅按 id 递增拉新会导致：历史报工被删后，本地 deleted 状态不会更新。
	//
	// 解决方案：两段式同步
	//  1) 仍按 id > lastID 拉取新增（保证性能）
	//  2) 对最近一段 id 窗口做“对账回刷”，把 deleted 变化同步回来（避免全表扫描）
	const reconcileIDWindow int64 = 5000
	reconcileFromID := lastID - reconcileIDWindow
	if reconcileFromID < 0 {
		reconcileFromID = 0
	}

	var newRows []source.ZtEffort
	qNew := ztDB.Table(cfg.Source).Where("id > ?", lastID)
	if cfg.ExtraFilter != "" {
		qNew = qNew.Where(cfg.ExtraFilter)
	}
	if err := qNew.Find(&newRows).Error; err != nil {
		log.Printf("[etl] SyncEfforts(%s) new-rows query error: %v", cfg.Source, err)
		return
	}

	var reconcileRows []source.ZtEffort
	// 注意：只回刷“已存在范围”的记录，避免重复拉新；但覆盖 lastID 附近的 deleted 变化。
	qRecon := ztDB.Table(cfg.Source).Where("id <= ? AND id > ?", lastID, reconcileFromID)
	if cfg.ExtraFilter != "" {
		qRecon = qRecon.Where(cfg.ExtraFilter)
	}
	if err := qRecon.Find(&reconcileRows).Error; err != nil {
		log.Printf("[etl] SyncEfforts(%s) reconcile query error: %v", cfg.Source, err)
		return
	}

	now := time.Now()
	all := append(append([]source.ZtEffort{}, newRows...), reconcileRows...)
	upsertEffortRows(all, now)

	log.Printf(
		"[etl] SyncEfforts(%s→%s): upserted new=%d reconcile=%d (id_window=%d, lastID=%d)",
		cfg.Source, cfg.Name, len(newRows), len(reconcileRows), reconcileIDWindow, lastID,
	)
	setWatermark(cfg.Name, now, int64(len(newRows)+len(reconcileRows)))
}

// SyncExecutions pulls source execution table (default: zt_project) incrementally by lastEditedDate.
func SyncExecutions() {
	cfg, ok := tableConfig("local_executions")
	if !ok {
		return
	}
	ztDB := db.GetZentao()
	if ztDB == nil {
		return
	}

	wm := getWatermark(cfg.Name)
	var rows []source.ZtExecution
	q := ztDB.Table(cfg.Source)
	if clause, arg := whereClause(cfg, wm); clause != "" {
		q = q.Where(clause, arg)
	}
	if cfg.ExtraFilter != "" {
		q = q.Where(cfg.ExtraFilter)
	}
	if err := q.Find(&rows).Error; err != nil {
		log.Printf("[etl] SyncExecutions(%s) query error: %v", cfg.Source, err)
		return
	}

	now := time.Now()
	maxEdited := wm
	for _, r := range rows {
		var parentPtr *int64
		if r.Parent > 0 {
			v := r.Parent
			parentPtr = &v
		}
		m := models.LocalExecution{
			ID:        r.ID,
			Name:      r.Name,
			Status:    r.Status,
			BeginDate: db.SafeTime(r.Begin),
			EndDate:   db.SafeTime(r.End),
			ParentID:  parentPtr,
			Type:      r.Type,
			Deleted:   r.Deleted == "1",
			RawData:   db.RowToJSONB(r),
			SyncedAt:  now,
		}
		db.PG.Save(&m)
		if r.LastEditedDate != nil && r.LastEditedDate.After(maxEdited) {
			maxEdited = *r.LastEditedDate
		}
	}
	log.Printf("[etl] SyncExecutions(%s→%s): upserted %d rows since %v", cfg.Source, cfg.Name, len(rows), wm)
	if len(rows) > 0 {
		setWatermark(cfg.Name, maxEdited, int64(len(rows)))
	}
}

// SyncPrograms pulls zt_project rows where type indicates a program/project-set.
// Since Zentao variants differ, the source table and filter are configurable in etl_tables.yaml.
func SyncPrograms() {
	cfg, ok := tableConfig("local_programs")
	if !ok {
		return
	}
	ztDB := db.GetZentao()
	if ztDB == nil {
		return
	}

	var rows []source.ZtProjectRow
	q := ztDB.Table(cfg.Source)
	if cfg.ExtraFilter != "" {
		q = q.Where(cfg.ExtraFilter)
	}
	if err := q.Find(&rows).Error; err != nil {
		log.Printf("[etl] SyncPrograms(%s) query error: %v", cfg.Source, err)
		return
	}

	now := time.Now()
	for _, r := range rows {
		var parentPtr *int64
		if r.Parent > 0 {
			v := r.Parent
			parentPtr = &v
		}
		var gradePtr *int
		if r.Grade != 0 {
			v := r.Grade
			gradePtr = &v
		}
		m := models.LocalProgram{
			ID:        r.ID,
			Name:      r.Name,
			Status:    r.Status,
			ParentID:  parentPtr,
			Path:      r.Path,
			Grade:     gradePtr,
			BeginDate: db.SafeTime(r.Begin),
			EndDate:   db.SafeTime(r.End),
			Deleted:   r.Deleted == "1",
			RawData:   db.RowToJSONB(r),
			SyncedAt:  now,
		}
		db.PG.Save(&m)
	}
	log.Printf("[etl] SyncPrograms(%s→%s): upserted %d rows", cfg.Source, cfg.Name, len(rows))
	setWatermark(cfg.Name, now, int64(len(rows)))
}

// SyncProjects pulls zt_project rows where type indicates a project.
func SyncProjects() {
	cfg, ok := tableConfig("local_projects")
	if !ok {
		return
	}
	ztDB := db.GetZentao()
	if ztDB == nil {
		return
	}

	var rows []source.ZtProjectRow
	q := ztDB.Table(cfg.Source)
	if cfg.ExtraFilter != "" {
		q = q.Where(cfg.ExtraFilter)
	}
	if err := q.Find(&rows).Error; err != nil {
		log.Printf("[etl] SyncProjects(%s) query error: %v", cfg.Source, err)
		return
	}

	now := time.Now()
	for _, r := range rows {
		var parentPtr *int64
		if r.Parent > 0 {
			v := r.Parent
			parentPtr = &v
		}
		var gradePtr *int
		if r.Grade != 0 {
			v := r.Grade
			gradePtr = &v
		}
		m := models.LocalProject{
			ID:        r.ID,
			Name:      r.Name,
			Status:    r.Status,
			ParentID:  parentPtr,
			Path:      r.Path,
			Grade:     gradePtr,
			BeginDate: db.SafeTime(r.Begin),
			EndDate:   db.SafeTime(r.End),
			Deleted:   r.Deleted == "1",
			RawData:   db.RowToJSONB(r),
			SyncedAt:  now,
		}
		db.PG.Save(&m)
	}
	log.Printf("[etl] SyncProjects(%s→%s): upserted %d rows", cfg.Source, cfg.Name, len(rows))
	setWatermark(cfg.Name, now, int64(len(rows)))
}

// SyncProductLines pulls product-line table (commonly zt_line) if enabled.
func SyncProductLines() {
	cfg, ok := tableConfig("local_product_lines")
	if !ok {
		return
	}
	ztDB := db.GetZentao()
	if ztDB == nil {
		return
	}

	var rows []source.ZtProductLine
	q := ztDB.Table(cfg.Source)
	if cfg.ExtraFilter != "" {
		q = q.Where(cfg.ExtraFilter)
	}
	if err := q.Find(&rows).Error; err != nil {
		log.Printf("[etl] SyncProductLines(%s) query error: %v", cfg.Source, err)
		return
	}

	now := time.Now()
	for _, r := range rows {
		var parentPtr *int64
		if r.Parent > 0 {
			v := r.Parent
			parentPtr = &v
		}
		var gradePtr *int
		if r.Grade != 0 {
			v := r.Grade
			gradePtr = &v
		}
		m := models.LocalProductLine{
			ID:       r.ID,
			Name:     r.Name,
			ParentID: parentPtr,
			Path:     r.Path,
			Grade:    gradePtr,
			Deleted:  r.Deleted == "1",
			RawData:  db.RowToJSONB(r),
			SyncedAt: now,
		}
		db.PG.Save(&m)
	}
	log.Printf("[etl] SyncProductLines(%s→%s): upserted %d rows", cfg.Source, cfg.Name, len(rows))
	setWatermark(cfg.Name, now, int64(len(rows)))
}

// SyncProducts pulls zt_product (or configured source) and upserts into local_products.
func SyncProducts() {
	cfg, ok := tableConfig("local_products")
	if !ok {
		return
	}
	ztDB := db.GetZentao()
	if ztDB == nil {
		return
	}

	var rows []source.ZtProduct
	q := ztDB.Table(cfg.Source)
	if cfg.ExtraFilter != "" {
		q = q.Where(cfg.ExtraFilter)
	}
	if err := q.Find(&rows).Error; err != nil {
		log.Printf("[etl] SyncProducts(%s) query error: %v", cfg.Source, err)
		return
	}

	now := time.Now()
	for _, r := range rows {
		var linePtr *int64
		if r.Line > 0 {
			v := r.Line
			linePtr = &v
		}
		m := models.LocalProduct{
			ID:       r.ID,
			Name:     r.Name,
			Code:     r.Code,
			Status:   r.Status,
			LineID:   linePtr,
			Deleted:  r.Deleted == "1",
			RawData:  db.RowToJSONB(r),
			SyncedAt: now,
		}
		db.PG.Save(&m)
	}
	log.Printf("[etl] SyncProducts(%s→%s): upserted %d rows", cfg.Source, cfg.Name, len(rows))
	setWatermark(cfg.Name, now, int64(len(rows)))
}

// SyncActions pulls source action table (default: zt_action) by ID watermark.
func SyncActions() {
	cfg, ok := tableConfig("local_actions")
	if !ok {
		return
	}
	ztDB := db.GetZentao()
	if ztDB == nil {
		return
	}

	var lastID int64
	db.PG.Model(&models.LocalAction{}).Select("COALESCE(MAX(id),0)").Scan(&lastID)

	// action 同样可能在禅道侧被删除/软删，但该表也缺少稳定的“最后修改时间”字段。
	// 仅按 id 递增拉新会漏掉 deleted 状态回写，因此与 efforts 一致做两段式同步：
	//  1) id > lastID 拉新增
	//  2) 回刷最近一段 id 窗口，更新 deleted 变化
	const reconcileIDWindow int64 = 10000
	reconcileFromID := lastID - reconcileIDWindow
	if reconcileFromID < 0 {
		reconcileFromID = 0
	}

	var newRows []source.ZtAction
	qNew := ztDB.Table(cfg.Source).Where("id > ?", lastID)
	if cfg.ExtraFilter != "" {
		qNew = qNew.Where(cfg.ExtraFilter)
	}
	if err := qNew.Find(&newRows).Error; err != nil {
		log.Printf("[etl] SyncActions(%s) new-rows query error: %v", cfg.Source, err)
		return
	}

	var reconcileRows []source.ZtAction
	qRecon := ztDB.Table(cfg.Source).Where("id <= ? AND id > ?", lastID, reconcileFromID)
	if cfg.ExtraFilter != "" {
		qRecon = qRecon.Where(cfg.ExtraFilter)
	}
	if err := qRecon.Find(&reconcileRows).Error; err != nil {
		log.Printf("[etl] SyncActions(%s) reconcile query error: %v", cfg.Source, err)
		return
	}

	now := time.Now()
	upsert := func(r source.ZtAction) {
		m := models.LocalAction{
			ID:         r.ID,
			ObjectType: r.ObjectType,
			ObjectID:   r.ObjectID,
			Actor:      r.Actor,
			Action:     r.Action,
			ActionDate: db.SafeTime(r.Date),
			Comment:    r.Comment,
			Extra:      r.Extra,
			Deleted:    r.Deleted == "1",
			RawData:    db.RowToJSONB(r),
			SyncedAt:   now,
		}
		db.PG.Save(&m)
	}

	for _, r := range newRows {
		upsert(r)
		applyActionSideEffects(r)
	}
	for _, r := range reconcileRows {
		upsert(r)
		applyActionSideEffects(r)
	}

	log.Printf(
		"[etl] SyncActions(%s→%s): upserted new=%d reconcile=%d (id_window=%d, lastID=%d)",
		cfg.Source, cfg.Name, len(newRows), len(reconcileRows), reconcileIDWindow, lastID,
	)
	setWatermark(cfg.Name, now, int64(len(newRows)+len(reconcileRows)))
}

func norm(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// isDeleteAction returns true if the zt_action.action represents a delete/erase action.
// Zentao instances vary: commonly "deleted", sometimes "erased".
func isDeleteAction(action string) bool {
	a := norm(action)
	return a == "deleted" || a == "erased"
}

// isRestoreAction returns true if the zt_action.action represents restoring a deleted record.
// Zentao instances vary: commonly "undeleted" / "restored".
func isRestoreAction(action string) bool {
	a := norm(action)
	return a == "undeleted" || a == "restored"
}

// applyActionSideEffects maps Zentao delete/restore actions into local table deleted flags.
// This is critical because some Zentao delete operations do NOT update lastEditedDate on the object,
// making watermark-by-lastEditedDate incremental sync miss the status change.
func applyActionSideEffects(a source.ZtAction) {
	ot := norm(a.ObjectType)
	act := a.Action
	id := a.ObjectID
	if id <= 0 {
		return
	}

	var (
		markDeleted *bool
	)
	if isDeleteAction(act) {
		v := true
		markDeleted = &v
	} else if isRestoreAction(act) {
		v := false
		markDeleted = &v
	} else {
		return
	}

	switch ot {
	case "task":
		db.PG.Model(&models.LocalTask{}).Where("id = ?", id).Update("deleted", *markDeleted)
	case "story":
		db.PG.Model(&models.LocalStory{}).Where("id = ?", id).Update("deleted", *markDeleted)
	case "bug":
		db.PG.Model(&models.LocalBug{}).Where("id = ?", id).Update("deleted", *markDeleted)
	case "execution", "project":
		// execution is stored in Zentao project table; some instances emit objectType=project.
		db.PG.Model(&models.LocalExecution{}).Where("id = ?", id).Update("deleted", *markDeleted)
	default:
		// ignore other object types
	}
}

// SyncHistories pulls source history table (default: zt_history) by ID watermark.
func SyncHistories() {
	cfg, ok := tableConfig("local_histories")
	if !ok {
		return
	}
	ztDB := db.GetZentao()
	if ztDB == nil {
		return
	}

	var lastID int64
	db.PG.Model(&models.LocalHistory{}).Select("COALESCE(MAX(id),0)").Scan(&lastID)

	// history 也可能被软删（看禅道版本/定制），同样缺少 lastEdited 水位线字段。
	// 做一个小窗口回刷，至少保证 deleted/修正能同步（如果源表不存在 deleted 字段，GORM 会忽略赋值）。
	const reconcileIDWindow int64 = 20000
	reconcileFromID := lastID - reconcileIDWindow
	if reconcileFromID < 0 {
		reconcileFromID = 0
	}

	var newRows []source.ZtHistory
	qNew := ztDB.Table(cfg.Source).Where("id > ?", lastID)
	if cfg.ExtraFilter != "" {
		qNew = qNew.Where(cfg.ExtraFilter)
	}
	if err := qNew.Find(&newRows).Error; err != nil {
		log.Printf("[etl] SyncHistories(%s) new-rows query error: %v", cfg.Source, err)
		return
	}

	var reconcileRows []source.ZtHistory
	qRecon := ztDB.Table(cfg.Source).Where("id <= ? AND id > ?", lastID, reconcileFromID)
	if cfg.ExtraFilter != "" {
		qRecon = qRecon.Where(cfg.ExtraFilter)
	}
	if err := qRecon.Find(&reconcileRows).Error; err != nil {
		log.Printf("[etl] SyncHistories(%s) reconcile query error: %v", cfg.Source, err)
		return
	}

	now := time.Now()
	upsert := func(r source.ZtHistory) {
		m := models.LocalHistory{
			ID:       r.ID,
			ActionID: r.Action,
			Field:    r.Field,
			Old:      r.Old,
			New:      r.New,
			Diff:     r.Diff,
			RawData:  db.RowToJSONB(r),
			SyncedAt: now,
		}
		db.PG.Save(&m)
	}

	for _, r := range newRows {
		upsert(r)
	}
	for _, r := range reconcileRows {
		upsert(r)
	}

	log.Printf(
		"[etl] SyncHistories(%s→%s): upserted new=%d reconcile=%d (id_window=%d, lastID=%d)",
		cfg.Source, cfg.Name, len(newRows), len(reconcileRows), reconcileIDWindow, lastID,
	)
	setWatermark(cfg.Name, now, int64(len(newRows)+len(reconcileRows)))
}
