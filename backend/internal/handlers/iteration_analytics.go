package handlers

import (
	"math"
	"net/http"
	"sort"
	"strings"
	"time"
	"zenmind/internal/db"

	"github.com/gin-gonic/gin"
)

type iterationReq struct {
	GroupID     int
	ExecutionID int64
	DateFrom    time.Time
	DateTo      time.Time
}

func parseIterationReq(c *gin.Context) (iterationReq, error) {
	groupID := queryInt(c, "group_id")
	execID := int64(queryInt(c, "execution_id"))
	if execID <= 0 {
		return iterationReq{}, errBadRequest("execution_id is required")
	}
	from := queryDate(c, "date_from")
	to := queryDate(c, "date_to")
	if from.IsZero() || to.IsZero() {
		// fallback: use execution date range (if available)
		var row struct {
			Begin *time.Time `gorm:"column:begin_date"`
			End   *time.Time `gorm:"column:end_date"`
		}
		db.PG.Table("local_executions").Select("begin_date, end_date").
			Where("deleted = false AND id = ?", execID).Scan(&row)
		if from.IsZero() && row.Begin != nil {
			from = time.Date(row.Begin.Year(), row.Begin.Month(), row.Begin.Day(), 0, 0, 0, 0, time.Local)
		}
		if to.IsZero() && row.End != nil {
			to = time.Date(row.End.Year(), row.End.Month(), row.End.Day(), 0, 0, 0, 0, time.Local)
		}
	}
	if from.IsZero() || to.IsZero() {
		return iterationReq{}, errBadRequest("date_from/date_to are required (or execution must have begin/end)")
	}
	if to.Before(from) {
		return iterationReq{}, errBadRequest("date_to must be >= date_from")
	}
	if !enforceMaxRange(from, to) {
		return iterationReq{}, errBadRequest("date range must not exceed 6 months")
	}
	// group_id is optional:
	// - group_id > 0: apply group isolation
	// - group_id <= 0: no group filter (all data)
	return iterationReq{GroupID: groupID, ExecutionID: execID, DateFrom: from, DateTo: to}, nil
}

// IterationOverview GET /api/analytics/iteration/overview
func IterationOverview(c *gin.Context) {
	req, err := parseIterationReq(c)
	if err != nil {
		if _, ok := err.(badRequestErr); ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	accounts, showNone := groupFilter(req.GroupID)
	if showNone {
		c.JSON(http.StatusOK, gin.H{"group_id": req.GroupID, "execution_id": req.ExecutionID})
		return
	}
	// Only error on empty members when group_id is explicitly provided.
	if req.GroupID > 0 && len(accounts) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "group has no members"})
		return
	}

	// Execution meta
	var exec struct {
		ID     int64      `gorm:"column:id"`
		Name   string     `gorm:"column:name"`
		Status string     `gorm:"column:status"`
		Begin  *time.Time `gorm:"column:begin_date"`
		End    *time.Time `gorm:"column:end_date"`
	}
	db.PG.Table("local_executions").
		Select("id, name, status, begin_date, end_date").
		Where("deleted = false AND id = ?", req.ExecutionID).
		Scan(&exec)

	// Task stats
	type taskAgg struct {
		Total           int64   `gorm:"column:total"`
		OpenCount       int64   `gorm:"column:open_count"`
		DoneCount       int64   `gorm:"column:done_count"`
		EstimateSum     float64 `gorm:"column:estimate_sum"`
		EstimateSumOpen float64 `gorm:"column:estimate_sum_open"`
		ConsumedSum     float64 `gorm:"column:consumed_sum"`
	}
	var ta taskAgg
	openStatuses := []string{"wait", "doing", "active", "pause"}
	doneStatuses := []string{"done", "closed"}

	qTask := db.PG.Table("local_tasks").
		Select(`
			COUNT(1) AS total,
			SUM(CASE WHEN status IN ? THEN 1 ELSE 0 END) AS open_count,
			SUM(CASE WHEN status IN ? THEN 1 ELSE 0 END) AS done_count,
			COALESCE(SUM(estimate),0) AS estimate_sum,
			COALESCE(SUM(CASE WHEN status IN ? THEN estimate ELSE 0 END),0) AS estimate_sum_open,
			COALESCE(SUM(consumed),0) AS consumed_sum
		`, openStatuses, doneStatuses, openStatuses).
		Where("deleted = false").
		Where("execution_id = ?", req.ExecutionID)
	if accounts != nil {
		qTask = qTask.Where("assigned_to IN ?", accounts)
	}
	qTask.Scan(&ta)

	// Bug stats
	type bugAgg struct {
		Total     int64 `gorm:"column:total"`
		OpenCount int64 `gorm:"column:open_count"`
		Resolved  int64 `gorm:"column:resolved_count"`
		Sev1Or2   int64 `gorm:"column:sev12"`
		Sev3      int64 `gorm:"column:sev3"`
		Sev4Plus  int64 `gorm:"column:sev4p"`
	}
	var ba bugAgg
	qBug := db.PG.Table("local_bugs").
		Select(`
			COUNT(1) AS total,
			SUM(CASE WHEN status NOT IN ('closed','resolved') THEN 1 ELSE 0 END) AS open_count,
			SUM(CASE WHEN status IN ('resolved','closed') THEN 1 ELSE 0 END) AS resolved_count,
			SUM(CASE WHEN severity <= 2 THEN 1 ELSE 0 END) AS sev12,
			SUM(CASE WHEN severity = 3 THEN 1 ELSE 0 END) AS sev3,
			SUM(CASE WHEN severity >= 4 THEN 1 ELSE 0 END) AS sev4p
		`).
		Where("deleted = false").
		Where("execution_id = ?", req.ExecutionID)
	if accounts != nil {
		qBug = qBug.Where("assigned_to IN ?", accounts)
	}
	qBug.Scan(&ba)

	// Effort total in range for group members and (optionally) scoped to iteration objects
	type effortAgg struct {
		Total float64 `gorm:"column:total"`
		Bug   float64 `gorm:"column:bug"`
		Task  float64 `gorm:"column:task"`
		Story float64 `gorm:"column:story"`
	}
	var ea effortAgg
	qEff := db.PG.Table("local_efforts").
		Select(`
			COALESCE(SUM(consumed),0) AS total,
			COALESCE(SUM(CASE WHEN LOWER(TRIM(object_type))='bug' THEN consumed ELSE 0 END),0) AS bug,
			COALESCE(SUM(CASE WHEN LOWER(TRIM(object_type))='task' THEN consumed ELSE 0 END),0) AS task,
			COALESCE(SUM(CASE WHEN LOWER(TRIM(object_type))='story' THEN consumed ELSE 0 END),0) AS story
		`).
		Where("deleted = false").
		Where("work_date BETWEEN ? AND ?", req.DateFrom, req.DateTo)
	if accounts != nil {
		qEff = qEff.Where("account IN ?", accounts)
	}
	qEff.Scan(&ea)

	// crude health score (configurable later)
	progress := safeRateFloat(float64(ta.DoneCount), float64(maxI64(ta.Total, 1)))
	qualityPenalty := safeRateFloat(float64(ba.Sev1Or2), float64(maxI64(ba.Total, 1)))
	health := clamp01(progress*0.7 + (1-qualityPenalty)*0.3)

	c.JSON(http.StatusOK, gin.H{
		"group_id":     req.GroupID,
		"execution_id": req.ExecutionID,
		"date_from":    req.DateFrom.Format("2006-01-02"),
		"date_to":      req.DateTo.Format("2006-01-02"),
		"execution": gin.H{
			"id":     exec.ID,
			"name":   exec.Name,
			"status": exec.Status,
			"begin":  formatDatePtr(exec.Begin),
			"end":    formatDatePtr(exec.End),
		},
		"tasks": gin.H{
			"total":             ta.Total,
			"open":              ta.OpenCount,
			"done":              ta.DoneCount,
			"estimate_sum":      round1(ta.EstimateSum),
			"estimate_sum_open": round1(ta.EstimateSumOpen),
			"consumed_sum":      round1(ta.ConsumedSum),
		},
		"bugs": gin.H{
			"total":         ba.Total,
			"open":          ba.OpenCount,
			"resolved":      ba.Resolved,
			"severity_1_2":  ba.Sev1Or2,
			"severity_3":    ba.Sev3,
			"severity_4_up": ba.Sev4Plus,
		},
		"efforts": gin.H{
			"total_hours": round1(ea.Total),
			"bug_hours":   round1(ea.Bug),
			"task_hours":  round1(ea.Task),
			"story_hours": round1(ea.Story),
		},
		"health": gin.H{
			"score": round1(health * 100),
		},
	})
}

// IterationBurndown GET /api/analytics/iteration/burndown
func IterationBurndown(c *gin.Context) {
	req, err := parseIterationReq(c)
	if err != nil {
		if _, ok := err.(badRequestErr); ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	accounts, showNone := groupFilter(req.GroupID)
	if showNone {
		c.JSON(http.StatusOK, gin.H{"series": []gin.H{}})
		return
	}
	if req.GroupID > 0 && len(accounts) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "group has no members"})
		return
	}

	days := dateKeys(req.DateFrom, req.DateTo, false)
	type row struct {
		Day          string  `json:"day"`
		OpenEstimate float64 `json:"open_estimate"`
		DoneCount    int64   `json:"done_count"`
	}
	out := make([]row, 0, len(days))
	for _, d := range days {
		// Try cache (fresh within 10 minutes)
		var cached struct {
			OpenEstimate float64   `gorm:"column:open_estimate"`
			DoneTotal    int64     `gorm:"column:done_total"`
			UpdatedAt    time.Time `gorm:"column:updated_at"`
		}
		db.PG.Table("iteration_daily_metrics").
			Select("open_estimate, done_total, updated_at").
			Where("group_id = ? AND execution_id = ? AND day = ?", req.GroupID, req.ExecutionID, d).
			Scan(&cached)
		if !cached.UpdatedAt.IsZero() && time.Since(cached.UpdatedAt) < 10*time.Minute {
			out = append(out, row{Day: d, OpenEstimate: round1(cached.OpenEstimate), DoneCount: cached.DoneTotal})
			continue
		}

		dayEnd, _ := time.ParseInLocation("2006-01-02 15:04:05", d+" 23:59:59", time.Local)
		var r struct {
			OpenEstimate float64 `gorm:"column:open_estimate"`
			DoneCount    int64   `gorm:"column:done_count"`
		}
		qDay := db.PG.Table("local_tasks").
			Select(`
				COALESCE(SUM(CASE
					WHEN opened_date IS NOT NULL AND opened_date <= ?
					  AND COALESCE(finished_date, closed_date) IS NULL THEN estimate
					WHEN opened_date IS NOT NULL AND opened_date <= ?
					  AND COALESCE(finished_date, closed_date) > ? THEN estimate
					ELSE 0 END),0) AS open_estimate,
				COALESCE(SUM(CASE WHEN COALESCE(finished_date, closed_date) <= ? THEN 1 ELSE 0 END),0) AS done_count
			`, dayEnd, dayEnd, dayEnd, dayEnd).
			Where("deleted = false").
			Where("execution_id = ?", req.ExecutionID)
		if accounts != nil {
			qDay = qDay.Where("assigned_to IN ?", accounts)
		}
		qDay.Scan(&r)

		// upsert cache
		db.PG.Exec(`
			INSERT INTO iteration_daily_metrics (group_id, execution_id, day, open_estimate, done_total, updated_at)
			VALUES (?, ?, ?, ?, ?, NOW())
			ON CONFLICT (group_id, execution_id, day)
			DO UPDATE SET open_estimate = EXCLUDED.open_estimate, done_total = EXCLUDED.done_total, updated_at = NOW()
		`, req.GroupID, req.ExecutionID, d, r.OpenEstimate, r.DoneCount)

		out = append(out, row{Day: d, OpenEstimate: round1(r.OpenEstimate), DoneCount: r.DoneCount})
	}

	c.JSON(http.StatusOK, gin.H{
		"group_id":     req.GroupID,
		"execution_id": req.ExecutionID,
		"date_from":    req.DateFrom.Format("2006-01-02"),
		"date_to":      req.DateTo.Format("2006-01-02"),
		"series":       out,
	})
}

// IterationCFD GET /api/analytics/iteration/cfd
func IterationCFD(c *gin.Context) {
	req, err := parseIterationReq(c)
	if err != nil {
		if _, ok := err.(badRequestErr); ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	accounts, showNone := groupFilter(req.GroupID)
	if showNone {
		c.JSON(http.StatusOK, gin.H{"series": []gin.H{}})
		return
	}
	if req.GroupID > 0 && len(accounts) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "group has no members"})
		return
	}

	days := dateKeys(req.DateFrom, req.DateTo, false)
	type point struct {
		Day   string `json:"day"`
		Todo  int64  `json:"todo"`
		Doing int64  `json:"doing"`
		Done  int64  `json:"done"`
	}
	out := make([]point, 0, len(days))
	for _, d := range days {
		// Try cache (fresh within 10 minutes)
		var cached struct {
			Todo      int64     `gorm:"column:todo_count"`
			Doing     int64     `gorm:"column:doing_count"`
			Done      int64     `gorm:"column:done_count"`
			UpdatedAt time.Time `gorm:"column:updated_at"`
		}
		db.PG.Table("iteration_daily_metrics").
			Select("todo_count, doing_count, done_count, updated_at").
			Where("group_id = ? AND execution_id = ? AND day = ?", req.GroupID, req.ExecutionID, d).
			Scan(&cached)
		if !cached.UpdatedAt.IsZero() && time.Since(cached.UpdatedAt) < 10*time.Minute {
			out = append(out, point{Day: d, Todo: cached.Todo, Doing: cached.Doing, Done: cached.Done})
			continue
		}

		dayEnd, _ := time.ParseInLocation("2006-01-02 15:04:05", d+" 23:59:59", time.Local)
		var r struct {
			Todo  int64 `gorm:"column:todo"`
			Doing int64 `gorm:"column:doing"`
			Done  int64 `gorm:"column:done"`
		}
		qCfd := db.PG.Table("local_tasks").
			Select(`
				COALESCE(SUM(CASE
					WHEN opened_date IS NOT NULL AND opened_date <= ?
					  AND COALESCE(finished_date, closed_date) IS NOT NULL
					  AND COALESCE(finished_date, closed_date) <= ? THEN 1 ELSE 0 END),0) AS done,
				COALESCE(SUM(CASE
					WHEN opened_date IS NOT NULL AND opened_date <= ?
					  AND (COALESCE(finished_date, closed_date) IS NULL OR COALESCE(finished_date, closed_date) > ?)
					  AND started_date IS NOT NULL AND started_date <= ? THEN 1 ELSE 0 END),0) AS doing,
				COALESCE(SUM(CASE
					WHEN opened_date IS NOT NULL AND opened_date <= ?
					  AND (COALESCE(finished_date, closed_date) IS NULL OR COALESCE(finished_date, closed_date) > ?)
					  AND (started_date IS NULL OR started_date > ?) THEN 1 ELSE 0 END),0) AS todo
			`, dayEnd, dayEnd, dayEnd, dayEnd, dayEnd, dayEnd, dayEnd, dayEnd).
			Where("deleted = false").
			Where("execution_id = ?", req.ExecutionID)
		if accounts != nil {
			qCfd = qCfd.Where("assigned_to IN ?", accounts)
		}
		qCfd.Scan(&r)

		db.PG.Exec(`
			INSERT INTO iteration_daily_metrics (group_id, execution_id, day, todo_count, doing_count, done_count, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, NOW())
			ON CONFLICT (group_id, execution_id, day)
			DO UPDATE SET todo_count = EXCLUDED.todo_count, doing_count = EXCLUDED.doing_count, done_count = EXCLUDED.done_count, updated_at = NOW()
		`, req.GroupID, req.ExecutionID, d, r.Todo, r.Doing, r.Done)

		out = append(out, point{Day: d, Todo: r.Todo, Doing: r.Doing, Done: r.Done})
	}
	c.JSON(http.StatusOK, gin.H{
		"group_id":     req.GroupID,
		"execution_id": req.ExecutionID,
		"date_from":    req.DateFrom.Format("2006-01-02"),
		"date_to":      req.DateTo.Format("2006-01-02"),
		"series":       out,
		"note":         "CFD基于 opened/started/finished/closed 时间戳近似计算；若禅道未维护对应字段，数据会偏空。",
	})
}

// IterationCycleTime GET /api/analytics/iteration/cycle-time
func IterationCycleTime(c *gin.Context) {
	req, err := parseIterationReq(c)
	if err != nil {
		if _, ok := err.(badRequestErr); ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	accounts, showNone := groupFilter(req.GroupID)
	if showNone {
		c.JSON(http.StatusOK, gin.H{"items": []gin.H{}})
		return
	}
	if req.GroupID > 0 && len(accounts) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "group has no members"})
		return
	}

	type item struct {
		ID         int64      `gorm:"column:id"`
		AssignedTo string     `gorm:"column:assigned_to"`
		Opened     *time.Time `gorm:"column:opened_date"`
		Started    *time.Time `gorm:"column:started_date"`
		Finished   *time.Time `gorm:"column:finished"`
	}
	var rows []item
	qCT := db.PG.Table("local_tasks").
		Select("id, assigned_to, opened_date, started_date, COALESCE(finished_date, closed_date) AS finished").
		Where("deleted = false").
		Where("execution_id = ?", req.ExecutionID).
		Where("COALESCE(finished_date, closed_date) BETWEEN ? AND ?", req.DateFrom, req.DateTo).
		Where("opened_date IS NOT NULL")
	if accounts != nil {
		qCT = qCT.Where("assigned_to IN ?", accounts)
	}
	qCT.Find(&rows)

	var cycleHours []float64
	var leadHours []float64
	for _, r := range rows {
		if r.Finished == nil || r.Opened == nil {
			continue
		}
		if r.Started != nil && !r.Finished.Before(*r.Started) {
			cycleHours = append(cycleHours, r.Finished.Sub(*r.Started).Hours())
		}
		if !r.Finished.Before(*r.Opened) {
			leadHours = append(leadHours, r.Finished.Sub(*r.Opened).Hours())
		}
	}
	sort.Float64s(cycleHours)
	sort.Float64s(leadHours)

	c.JSON(http.StatusOK, gin.H{
		"group_id":     req.GroupID,
		"execution_id": req.ExecutionID,
		"date_from":    req.DateFrom.Format("2006-01-02"),
		"date_to":      req.DateTo.Format("2006-01-02"),
		"count":        len(rows),
		"cycle_time_hours": gin.H{
			"p50": round1(percentile(cycleHours, 0.50)),
			"p85": round1(percentile(cycleHours, 0.85)),
			"p95": round1(percentile(cycleHours, 0.95)),
		},
		"lead_time_hours": gin.H{
			"p50": round1(percentile(leadHours, 0.50)),
			"p85": round1(percentile(leadHours, 0.85)),
			"p95": round1(percentile(leadHours, 0.95)),
		},
		"note": "周期时间=finished-started；前置时间=finished-opened。单位小时。",
	})
}

// IterationScopeChange GET /api/analytics/iteration/scope-change
func IterationScopeChange(c *gin.Context) {
	req, err := parseIterationReq(c)
	if err != nil {
		if _, ok := err.(badRequestErr); ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	accounts, showNone := groupFilter(req.GroupID)
	if showNone {
		c.JSON(http.StatusOK, gin.H{"items": []gin.H{}})
		return
	}
	if req.GroupID > 0 && len(accounts) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "group has no members"})
		return
	}

	// Identify execution changes via history, joined to action timestamp.
	type row struct {
		ActionDate time.Time `gorm:"column:action_date"`
		ObjectType string    `gorm:"column:object_type"`
		ObjectID   int64     `gorm:"column:object_id"`
		Field      string    `gorm:"column:field"`
		Old        string    `gorm:"column:old"`
		New        string    `gorm:"column:new"`
		Actor      string    `gorm:"column:actor"`
	}
	var rows []row
	db.PG.Table("local_histories h").
		Select(`
			COALESCE(a.action_date, NOW()) AS action_date,
			a.object_type, a.object_id, h.field, h.old, h.new, COALESCE(a.actor,'') AS actor
		`).
		Joins("JOIN local_actions a ON a.id = h.action_id").
		Where("a.deleted = false").
		Where("LOWER(TRIM(a.object_type)) IN ('task','story','bug')").
		Where("a.action_date BETWEEN ? AND ?", req.DateFrom, req.DateTo).
		Where("LOWER(TRIM(h.field)) LIKE 'execution%' OR LOWER(TRIM(h.field)) IN ('execution','iteration','sprint')").
		Order("a.action_date DESC").
		Limit(500).
		Scan(&rows)

	// Optional: keep only visible objects in group by checking current ownership (cheap filter).
	visibleTask := map[int64]bool{}
	{
		var ids []int64
		qVis := db.PG.Table("local_tasks").Select("id").Where("deleted = false AND execution_id = ?", req.ExecutionID)
		if accounts != nil {
			qVis = qVis.Where("assigned_to IN ?", accounts)
		}
		qVis.Pluck("id", &ids)
		for _, id := range ids {
			visibleTask[id] = true
		}
	}

	items := make([]gin.H, 0, len(rows))
	for _, r := range rows {
		ot := stringsLowerTrim(r.ObjectType)
		if ot == "task" && !visibleTask[r.ObjectID] {
			continue
		}
		items = append(items, gin.H{
			"time":        r.ActionDate.Format(time.RFC3339),
			"object_type": ot,
			"object_id":   r.ObjectID,
			"field":       r.Field,
			"old":         r.Old,
			"new":         r.New,
			"actor":       r.Actor,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"group_id":     req.GroupID,
		"execution_id": req.ExecutionID,
		"date_from":    req.DateFrom.Format("2006-01-02"),
		"date_to":      req.DateTo.Format("2006-01-02"),
		"items":        items,
		"note":         "范围变更依赖 zt_action/zt_history（local_actions/local_histories）同步；若缺失则列表为空。",
	})
}

func maxI64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func clamp01(x float64) float64 {
	if x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}

func round1(x float64) float64 { return math.Round(x*10) / 10 }

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if p <= 0 {
		return sorted[0]
	}
	if p >= 1 {
		return sorted[len(sorted)-1]
	}
	pos := p * float64(len(sorted)-1)
	lo := int(math.Floor(pos))
	hi := int(math.Ceil(pos))
	if lo == hi {
		return sorted[lo]
	}
	w := pos - float64(lo)
	return sorted[lo]*(1-w) + sorted[hi]*w
}

func formatDatePtr(t *time.Time) interface{} {
	if t == nil {
		return nil
	}
	return t.Format("2006-01-02")
}

func stringsLowerTrim(s string) string {
	s = strings.TrimSpace(s)
	return strings.ToLower(s)
}
