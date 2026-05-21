package handlers

import (
	"net/http"
	"sort"
	"time"
	"zenmind/internal/db"

	"github.com/gin-gonic/gin"
)

type peopleReq struct {
	GroupID  int
	DateFrom time.Time
	DateTo   time.Time
}

func parsePeopleReq(c *gin.Context) (peopleReq, error) {
	groupID := queryInt(c, "group_id")
	from := queryDate(c, "date_from")
	to := queryDate(c, "date_to")
	if from.IsZero() || to.IsZero() {
		// also accept start/end like existing analytics endpoints
		from = queryDate(c, "start")
		to = queryDate(c, "end")
	}
	if from.IsZero() || to.IsZero() {
		return peopleReq{}, errBadRequest("date_from/date_to (or start/end) are required")
	}
	if to.Before(from) {
		return peopleReq{}, errBadRequest("date_to must be >= date_from")
	}
	if !enforceMaxRange(from, to) {
		return peopleReq{}, errBadRequest("date range must not exceed 6 months")
	}
	// group_id is optional:
	// - group_id > 0: apply group isolation
	// - group_id <= 0: no group filter (all data)
	return peopleReq{GroupID: groupID, DateFrom: from, DateTo: to}, nil
}

func peopleAccounts(req peopleReq, forced []string) (accounts []string, showNone bool, err error) {
	if forced != nil {
		if len(forced) == 0 {
			return nil, true, nil
		}
		return forced, false, nil
	}
	if req.GroupID > 0 {
		acc, sn := groupFilter(req.GroupID)
		if sn {
			return nil, true, nil
		}
		if len(acc) == 0 {
			return nil, false, errBadRequest("group has no members")
		}
		return acc, false, nil
	}

	// No group filter: derive accounts from existing data (tasks/efforts).
	set := map[string]struct{}{}
	var a1 []string
	db.PG.Table("local_tasks").
		Select("DISTINCT assigned_to").
		Where("deleted = false AND COALESCE(TRIM(assigned_to),'') <> ''").
		Pluck("assigned_to", &a1)
	for _, a := range a1 {
		set[a] = struct{}{}
	}
	var a2 []string
	db.PG.Table("local_efforts").
		Select("DISTINCT account").
		Where("deleted = false AND COALESCE(TRIM(account),'') <> ''").
		Pluck("account", &a2)
	for _, a := range a2 {
		set[a] = struct{}{}
	}

	accounts = make([]string, 0, len(set))
	for a := range set {
		accounts = append(accounts, a)
	}
	sort.Strings(accounts)
	return accounts, false, nil
}

// PeopleOverview GET /api/analytics/people/overview
func PeopleOverview(c *gin.Context) {
	req, err := parsePeopleReq(c)
	if err != nil {
		if _, ok := err.(badRequestErr); ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Scope enforcement:
	// - SELF: restrict to bound zentao account
	// - GROUP: force to default group (even if group_id not provided)
	// - ALL: allow (group optional)
	var forced []string
	if cu := GetCurrentUser(c); cu == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	} else {
		switch normalizeScope(cu.User.DataScope) {
		case scopeSelf:
			acc, ok := requireBinding(c)
			if !ok {
				return
			}
			forced = []string{acc}
			req.GroupID = 0
		case scopeGroup:
			gid, ok := effectiveGroupID(c, req.GroupID)
			if !ok {
				return
			}
			req.GroupID = gid
		}
	}

	accounts, showNone, err := peopleAccounts(req, forced)
	if err != nil {
		if _, ok := err.(badRequestErr); ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if showNone {
		c.JSON(http.StatusOK, gin.H{"rows": []gin.H{}})
		return
	}

	// names
	nameOf := map[string]string{}
	if req.GroupID > 0 {
		var memberRows []groupMemberRow
		db.PG.Table("group_members").
			Select("group_members.account, COALESCE(local_users.realname, '') AS realname").
			Joins("LEFT JOIN local_users ON local_users.account = group_members.account").
			Where("group_members.group_id = ?", req.GroupID).
			Order("group_members.account").
			Scan(&memberRows)
		for _, m := range memberRows {
			nameOf[m.Account] = m.Realname
		}
	} else {
		type row struct {
			Account  string `gorm:"column:account"`
			Realname string `gorm:"column:realname"`
		}
		var rows []row
		db.PG.Table("local_users").
			Select("account, COALESCE(realname,'') AS realname").
			Where("account IN ?", accounts).
			Scan(&rows)
		for _, r := range rows {
			nameOf[r.Account] = r.Realname
		}
	}

	// current open tasks + wip (started but not finished)
	type taskAgg struct {
		Account    string  `gorm:"column:account"`
		OpenCount  int64   `gorm:"column:open_count"`
		OpenEstSum float64 `gorm:"column:open_est_sum"`
		WIPCount   int64   `gorm:"column:wip_count"`
	}
	var tas []taskAgg
	openStatuses := []string{"wait", "doing", "active", "pause"}
	qTask := db.PG.Table("local_tasks").
		Select(`
			assigned_to AS account,
			COUNT(1) FILTER (WHERE status IN ?) AS open_count,
			COALESCE(SUM(CASE WHEN status IN ? THEN estimate ELSE 0 END),0) AS open_est_sum,
			COUNT(1) FILTER (WHERE started_date IS NOT NULL AND COALESCE(finished_date, closed_date) IS NULL) AS wip_count
		`, openStatuses, openStatuses).
		Where("deleted = false").
		Group("assigned_to")
	qTask = qTask.Where("assigned_to IN ?", accounts)
	qTask.Scan(&tas)
	byAcc := map[string]taskAgg{}
	for _, r := range tas {
		byAcc[r.Account] = r
	}

	// throughput in range (tasks finished_by)
	type doneAgg struct {
		Account string `gorm:"column:account"`
		DoneCnt int64  `gorm:"column:done_cnt"`
	}
	var das []doneAgg
	qDone := db.PG.Table("local_tasks").
		Select("finished_by AS account, COUNT(1) AS done_cnt").
		Where("deleted = false").
		Where("COALESCE(finished_date, closed_date) BETWEEN ? AND ?", req.DateFrom, req.DateTo).
		Group("finished_by")
	qDone = qDone.Where("finished_by IN ?", accounts)
	qDone.Scan(&das)
	doneBy := map[string]int64{}
	for _, r := range das {
		doneBy[r.Account] = r.DoneCnt
	}

	// effort in range
	type effortAgg struct {
		Account string  `gorm:"column:account"`
		Total   float64 `gorm:"column:total"`
		Bug     float64 `gorm:"column:bug"`
	}
	var eas []effortAgg
	qEff := db.PG.Table("local_efforts").
		Select(`
			account,
			COALESCE(SUM(consumed),0) AS total,
			COALESCE(SUM(CASE WHEN LOWER(TRIM(object_type))='bug' THEN consumed ELSE 0 END),0) AS bug
		`).
		Where("deleted = false").
		Where("work_date BETWEEN ? AND ?", req.DateFrom, req.DateTo).
		Group("account")
	qEff = qEff.Where("account IN ?", accounts)
	qEff.Scan(&eas)
	effBy := map[string]effortAgg{}
	for _, r := range eas {
		effBy[r.Account] = r
	}

	rows := make([]gin.H, 0, len(accounts))
	for _, acc := range accounts {
		t := byAcc[acc]
		e := effBy[acc]
		bugPct := safeRateFloat(e.Bug, e.Total)
		rows = append(rows, gin.H{
			"account":          acc,
			"realname":         nameOf[acc],
			"open_task_count":  t.OpenCount,
			"wip_count":        t.WIPCount,
			"open_estimate":    round1(t.OpenEstSum),
			"done_count_range": doneBy[acc],
			"effort_hours":     round1(e.Total),
			"bug_hours":        round1(e.Bug),
			"bug_percent":      round1(bugPct * 100),
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"group_id":  req.GroupID,
		"date_from": req.DateFrom.Format("2006-01-02"),
		"date_to":   req.DateTo.Format("2006-01-02"),
		"rows":      rows,
	})
}

// PeopleWIPTrend GET /api/analytics/people/wip-trend
func PeopleWIPTrend(c *gin.Context) {
	req, err := parsePeopleReq(c)
	if err != nil {
		if _, ok := err.(badRequestErr); ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var forced []string
	if cu := GetCurrentUser(c); cu == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	} else {
		switch normalizeScope(cu.User.DataScope) {
		case scopeSelf:
			acc, ok := requireBinding(c)
			if !ok {
				return
			}
			forced = []string{acc}
			req.GroupID = 0
		case scopeGroup:
			gid, ok := effectiveGroupID(c, req.GroupID)
			if !ok {
				return
			}
			req.GroupID = gid
		}
	}

	accounts, showNone, err := peopleAccounts(req, forced)
	if err != nil {
		if _, ok := err.(badRequestErr); ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if showNone {
		c.JSON(http.StatusOK, gin.H{"series": []gin.H{}})
		return
	}

	days := dateKeys(req.DateFrom, req.DateTo, false)
	series := make([]gin.H, 0, len(days))
	for _, d := range days {
		dayEnd, _ := time.ParseInLocation("2006-01-02 15:04:05", d+" 23:59:59", time.Local)
		var total int64
		q := db.PG.Table("local_tasks").
			Select("COUNT(1)").
			Where("deleted = false").
			Where("opened_date IS NOT NULL AND opened_date <= ?", dayEnd).
			Where("(COALESCE(finished_date, closed_date) IS NULL OR COALESCE(finished_date, closed_date) > ?)", dayEnd).
			Where("started_date IS NOT NULL AND started_date <= ?", dayEnd)
		q = q.Where("assigned_to IN ?", accounts)
		q.Scan(&total)
		series = append(series, gin.H{"day": d, "wip": total})
	}

	c.JSON(http.StatusOK, gin.H{
		"group_id":  req.GroupID,
		"date_from": req.DateFrom.Format("2006-01-02"),
		"date_to":   req.DateTo.Format("2006-01-02"),
		"series":    series,
	})
}

// PeopleThroughput GET /api/analytics/people/throughput
func PeopleThroughput(c *gin.Context) {
	req, err := parsePeopleReq(c)
	if err != nil {
		if _, ok := err.(badRequestErr); ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var forced []string
	if cu := GetCurrentUser(c); cu == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	} else {
		switch normalizeScope(cu.User.DataScope) {
		case scopeSelf:
			acc, ok := requireBinding(c)
			if !ok {
				return
			}
			forced = []string{acc}
			req.GroupID = 0
		case scopeGroup:
			gid, ok := effectiveGroupID(c, req.GroupID)
			if !ok {
				return
			}
			req.GroupID = gid
		}
	}
	accounts, showNone, err := peopleAccounts(req, forced)
	if err != nil {
		if _, ok := err.(badRequestErr); ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if showNone {
		c.JSON(http.StatusOK, gin.H{"series": []gin.H{}})
		return
	}

	type row struct {
		Day     time.Time `gorm:"column:day"`
		Account string    `gorm:"column:account"`
		Cnt     int64     `gorm:"column:cnt"`
	}
	var rows []row
	q := db.PG.Table("local_tasks").
		Select("DATE(COALESCE(finished_date, closed_date)) AS day, finished_by AS account, COUNT(1) AS cnt").
		Where("deleted = false").
		Where("COALESCE(finished_date, closed_date) BETWEEN ? AND ?", req.DateFrom, req.DateTo).
		Where("finished_by IN ?", accounts).
		Group("DATE(COALESCE(finished_date, closed_date)), finished_by")
	q.Scan(&rows)

	series := make([]gin.H, 0, len(rows))
	for _, r := range rows {
		series = append(series, gin.H{
			"day":     r.Day.Format("2006-01-02"),
			"account": r.Account,
			"done":    r.Cnt,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"group_id":  req.GroupID,
		"date_from": req.DateFrom.Format("2006-01-02"),
		"date_to":   req.DateTo.Format("2006-01-02"),
		"series":    series,
	})
}

// PeopleBottleneck GET /api/analytics/people/bottleneck
// MVP for bottleneck: list top long-running tasks per member based on started_date.
func PeopleBottleneck(c *gin.Context) {
	req, err := parsePeopleReq(c)
	if err != nil {
		if _, ok := err.(badRequestErr); ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var forced []string
	if cu := GetCurrentUser(c); cu == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	} else {
		switch normalizeScope(cu.User.DataScope) {
		case scopeSelf:
			acc, ok := requireBinding(c)
			if !ok {
				return
			}
			forced = []string{acc}
			req.GroupID = 0
		case scopeGroup:
			gid, ok := effectiveGroupID(c, req.GroupID)
			if !ok {
				return
			}
			req.GroupID = gid
		}
	}
	accounts, showNone, err := peopleAccounts(req, forced)
	if err != nil {
		if _, ok := err.(badRequestErr); ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if showNone {
		c.JSON(http.StatusOK, gin.H{"items": []gin.H{}})
		return
	}

	dayEnd := time.Date(req.DateTo.Year(), req.DateTo.Month(), req.DateTo.Day(), 23, 59, 59, 0, time.Local)

	type row struct {
		ID         int64      `gorm:"column:id"`
		Name       string     `gorm:"column:name"`
		AssignedTo string     `gorm:"column:assigned_to"`
		Started    *time.Time `gorm:"column:started_date"`
		Finished   *time.Time `gorm:"column:finished"`
		Status     string     `gorm:"column:status"`
	}
	var rows []row
	q := db.PG.Table("local_tasks").
		Select("id, name, assigned_to, started_date, COALESCE(finished_date, closed_date) AS finished, status").
		Where("deleted = false").
		Where("started_date IS NOT NULL AND started_date <= ?", dayEnd).
		Where("COALESCE(finished_date, closed_date) IS NULL OR COALESCE(finished_date, closed_date) > ?", dayEnd).
		Order("started_date ASC").
		Limit(100)
	q = q.Where("assigned_to IN ?", accounts)
	q.Scan(&rows)

	items := make([]gin.H, 0, len(rows))
	for _, r := range rows {
		ageHours := 0.0
		if r.Started != nil {
			ageHours = dayEnd.Sub(*r.Started).Hours()
		}
		items = append(items, gin.H{
			"id":          r.ID,
			"name":        r.Name,
			"assigned_to": r.AssignedTo,
			"status":      r.Status,
			"started_at":  r.Started,
			"age_hours":   round1(ageHours),
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"group_id":  req.GroupID,
		"date_from": req.DateFrom.Format("2006-01-02"),
		"date_to":   req.DateTo.Format("2006-01-02"),
		"items":     items,
		"note":      "瓶颈列表当前基于 started_date 的在制时长近似；后续可用 action/history 重建更精确的状态停留。",
	})
}
