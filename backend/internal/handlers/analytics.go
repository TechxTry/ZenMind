package handlers

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	"zenmind/internal/db"

	"github.com/gin-gonic/gin"
)

type effortHeatmapReq struct {
	GroupID        int
	DateFrom       time.Time
	DateTo         time.Time
	ExcludeWeekend bool
	TargetHours    float64
	OverloadHours  float64
	OverloadStreak int
}

type heatCell struct {
	ConsumedSum  float64 `json:"consumed_sum"`
	EntriesCount int     `json:"entries_count"`
}

type heatmapMember struct {
	Account  string `json:"account"`
	Realname string `json:"realname"`
}

type heatmapSummary struct {
	MemberDaysTotal      int `json:"member_days_total"`
	ComplianceMemberDays int `json:"compliance_member_days"`
	TargetMemberDays     int `json:"target_member_days"`
	OverloadMemberDays   int `json:"overload_member_days"`

	ComplianceRate float64 `json:"compliance_rate"`
	TargetRate     float64 `json:"target_rate"`
	OverloadRate   float64 `json:"overload_rate"`

	TopOverloadUsers []overloadUser `json:"top_overload_users"`
}

type overloadUser struct {
	Account        string `json:"account"`
	Realname       string `json:"realname"`
	MaxStreakDays  int    `json:"max_streak_days"`
	OverloadDays   int    `json:"overload_days"`
	TotalMemberDay int    `json:"total_member_day"`
}

func parseHeatmapReq(c *gin.Context) (effortHeatmapReq, error) {
	groupID := queryInt(c, "group_id")
	if groupID <= 0 {
		return effortHeatmapReq{}, errBadRequest("group_id is required")
	}

	// Accept both {start,end} and {date_from,date_to}
	dateFrom := queryDate(c, "start")
	dateTo := queryDate(c, "end")
	if dateFrom.IsZero() {
		dateFrom = queryDate(c, "date_from")
	}
	if dateTo.IsZero() {
		dateTo = queryDate(c, "date_to")
	}
	if dateFrom.IsZero() || dateTo.IsZero() {
		return effortHeatmapReq{}, errBadRequest("start/end (or date_from/date_to) are required")
	}
	if dateTo.Before(dateFrom) {
		return effortHeatmapReq{}, errBadRequest("end must be >= start")
	}
	if !enforceMaxRange(dateFrom, dateTo) {
		return effortHeatmapReq{}, errBadRequest("date range must not exceed 6 months")
	}

	excludeWeekend := strings.ToLower(strings.TrimSpace(c.DefaultQuery("exclude_weekend", "true"))) != "false"

	target := queryFloat(c, "target_hours", 8)
	overload := queryFloat(c, "overload_hours", 12)
	streak := queryIntDefault(c, "overload_streak", 3)
	if streak < 1 {
		streak = 1
	}

	return effortHeatmapReq{
		GroupID:        groupID,
		DateFrom:       dateFrom,
		DateTo:         dateTo,
		ExcludeWeekend: excludeWeekend,
		TargetHours:    target,
		OverloadHours:  overload,
		OverloadStreak: streak,
	}, nil
}

type badRequestErr struct{ msg string }

func (e badRequestErr) Error() string { return e.msg }

func errBadRequest(msg string) error { return badRequestErr{msg: msg} }

func queryIntDefault(c *gin.Context, key string, def int) int {
	if strings.TrimSpace(c.Query(key)) == "" {
		return def
	}
	return queryInt(c, key)
}

func queryFloat(c *gin.Context, key string, def float64) float64 {
	s := strings.TrimSpace(c.Query(key))
	if s == "" {
		return def
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return def
	}
	return v
}

// dateKeys returns a sorted date string slice within [from,to] (inclusive).
func dateKeys(from, to time.Time, excludeWeekend bool) []string {
	start := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, time.Local)
	end := time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, time.Local)
	var out []string
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		if excludeWeekend {
			if d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
				continue
			}
		}
		out = append(out, d.Format("2006-01-02"))
	}
	return out
}

// EffortHeatmap GET /api/analytics/effort-heatmap
func EffortHeatmap(c *gin.Context) {
	// Scope enforcement (group-based endpoint)
	if reqGID, ok := effectiveGroupID(c, queryInt(c, "group_id")); ok {
		_ = reqGID
	} else {
		return
	}

	req, err := parseHeatmapReq(c)
	if err != nil {
		if _, ok := err.(badRequestErr); ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Override group_id for GROUP scope; forbid for SELF already handled in effectiveGroupID.
	if gid, ok := effectiveGroupID(c, req.GroupID); ok {
		req.GroupID = gid
	} else {
		return
	}

	accounts, showNone := groupFilter(req.GroupID)
	if showNone {
		c.JSON(http.StatusOK, gin.H{
			"group_id": req.GroupID,
			"members":  []heatmapMember{},
			"dates":    []string{},
			"matrix":   gin.H{},
			"summary":  heatmapSummary{},
		})
		return
	}
	if len(accounts) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "group has no members"})
		return
	}

	// Load member display names (stable order)
	var memberRows []groupMemberRow
	db.PG.Table("group_members").
		Select("group_members.account, COALESCE(local_users.realname, '') AS realname").
		Joins("LEFT JOIN local_users ON local_users.account = group_members.account").
		Where("group_members.group_id = ?", req.GroupID).
		Order("group_members.account").
		Scan(&memberRows)
	members := make([]heatmapMember, 0, len(memberRows))
	realnameByAcc := map[string]string{}
	for _, r := range memberRows {
		members = append(members, heatmapMember{Account: r.Account, Realname: r.Realname})
		realnameByAcc[r.Account] = r.Realname
	}

	dates := dateKeys(req.DateFrom, req.DateTo, req.ExcludeWeekend)
	if len(dates) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"group_id": req.GroupID,
			"members":  members,
			"dates":    []string{},
			"matrix":   gin.H{},
			"summary":  heatmapSummary{},
		})
		return
	}

	// Aggregate efforts by account+date.
	type aggRow struct {
		Account string    `json:"account"`
		Day     time.Time `json:"day"`
		Sum     float64   `json:"sum"`
		Cnt     int       `json:"cnt"`
	}
	var agg []aggRow
	if err := db.PG.Table("local_efforts").
		Select("account, DATE(work_date) AS day, SUM(consumed) AS sum, COUNT(1) AS cnt").
		Where("deleted = false").
		Where("account IN ?", accounts).
		Where("work_date BETWEEN ? AND ?", req.DateFrom, req.DateTo).
		Group("account, DATE(work_date)").
		Scan(&agg).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	matrix := map[string]map[string]heatCell{}
	for _, acc := range accounts {
		matrix[acc] = map[string]heatCell{}
	}
	for _, r := range agg {
		dayKey := r.Day.Format("2006-01-02")
		if req.ExcludeWeekend {
			wd := r.Day.Weekday()
			if wd == time.Saturday || wd == time.Sunday {
				continue
			}
		}
		if _, ok := matrix[r.Account]; !ok {
			matrix[r.Account] = map[string]heatCell{}
		}
		matrix[r.Account][dayKey] = heatCell{ConsumedSum: r.Sum, EntriesCount: r.Cnt}
	}

	// Summary + overload streak scan.
	totalMemberDays := len(accounts) * len(dates)
	var complianceDays, targetDays, overloadDays int

	overloadUsers := make([]overloadUser, 0, len(accounts))
	for _, acc := range accounts {
		var streak, maxStreak, userOverloadDays int
		for _, d := range dates {
			cell := matrix[acc][d]
			if cell.ConsumedSum > 0 {
				complianceDays++
			}
			if cell.ConsumedSum >= req.TargetHours {
				targetDays++
			}
			if cell.ConsumedSum > req.OverloadHours {
				overloadDays++
				userOverloadDays++
				streak++
				if streak > maxStreak {
					maxStreak = streak
				}
			} else {
				streak = 0
			}
		}
		overloadUsers = append(overloadUsers, overloadUser{
			Account:        acc,
			Realname:       realnameByAcc[acc],
			MaxStreakDays:  maxStreak,
			OverloadDays:   userOverloadDays,
			TotalMemberDay: len(dates),
		})
	}

	sort.Slice(overloadUsers, func(i, j int) bool {
		if overloadUsers[i].MaxStreakDays != overloadUsers[j].MaxStreakDays {
			return overloadUsers[i].MaxStreakDays > overloadUsers[j].MaxStreakDays
		}
		return overloadUsers[i].OverloadDays > overloadUsers[j].OverloadDays
	})
	if len(overloadUsers) > 10 {
		overloadUsers = overloadUsers[:10]
	}

	summary := heatmapSummary{
		MemberDaysTotal:      totalMemberDays,
		ComplianceMemberDays: complianceDays,
		TargetMemberDays:     targetDays,
		OverloadMemberDays:   overloadDays,
		ComplianceRate:       safeRate(complianceDays, totalMemberDays),
		TargetRate:           safeRate(targetDays, totalMemberDays),
		OverloadRate:         safeRate(overloadDays, totalMemberDays),
		TopOverloadUsers:     overloadUsers,
	}

	c.JSON(http.StatusOK, gin.H{
		"group_id": req.GroupID,
		"members":  members,
		"dates":    dates,
		"matrix":   matrix,
		"config": gin.H{
			"exclude_weekend": req.ExcludeWeekend,
			"target_hours":    req.TargetHours,
			"overload_hours":  req.OverloadHours,
			"overload_streak": req.OverloadStreak,
		},
		"summary": summary,
	})
}

func safeRate(n, d int) float64 {
	if d <= 0 {
		return 0
	}
	return float64(n) / float64(d)
}

// UserLoad GET /api/analytics/user-load
func UserLoad(c *gin.Context) {
	groupID := queryInt(c, "group_id")
	if groupID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "group_id is required"})
		return
	}

	if gid, ok := effectiveGroupID(c, groupID); ok {
		groupID = gid
	} else {
		return
	}

	accounts, showNone := groupFilter(groupID)
	if showNone {
		c.JSON(http.StatusOK, gin.H{"group_id": groupID, "rows": []gin.H{}})
		return
	}
	if len(accounts) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "group has no members"})
		return
	}

	type aggRow struct {
		AssignedTo string  `json:"assigned_to"`
		TaskCount  int     `json:"task_count"`
		EstSum     float64 `json:"estimate_sum"`
	}
	var agg []aggRow

	openStatuses := []string{"wait", "doing", "active", "pause"}
	q := db.PG.Table("local_tasks").
		Select("assigned_to, COUNT(1) AS task_count, COALESCE(SUM(estimate), 0) AS estimate_sum").
		Where("deleted = false").
		Where("assigned_to IN ?", accounts).
		Where("status IN ?", openStatuses)

	if execID := int64(queryInt(c, "execution_id")); execID > 0 {
		q = q.Where("execution_id = ?", execID)
	}

	if err := q.Group("assigned_to").Scan(&agg).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Load member names
	var memberRows []groupMemberRow
	db.PG.Table("group_members").
		Select("group_members.account, COALESCE(local_users.realname, '') AS realname").
		Joins("LEFT JOIN local_users ON local_users.account = group_members.account").
		Where("group_members.group_id = ?", groupID).
		Order("group_members.account").
		Scan(&memberRows)
	nameOf := map[string]string{}
	for _, m := range memberRows {
		nameOf[m.Account] = m.Realname
	}

	byAcc := map[string]aggRow{}
	for _, r := range agg {
		byAcc[r.AssignedTo] = r
	}

	rows := make([]gin.H, 0, len(accounts))
	for _, acc := range accounts {
		r := byAcc[acc]
		rows = append(rows, gin.H{
			"account":           acc,
			"realname":          nameOf[acc],
			"open_task_count":   r.TaskCount,
			"estimate_sum_open": r.EstSum,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"group_id": groupID,
		"rows":     rows,
	})
}

// WorkloadDistribution GET /api/analytics/workload-distribution
func WorkloadDistribution(c *gin.Context) {
	groupID := queryInt(c, "group_id")
	if groupID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "group_id is required"})
		return
	}

	if gid, ok := effectiveGroupID(c, groupID); ok {
		groupID = gid
	} else {
		return
	}

	start := queryDate(c, "start")
	end := queryDate(c, "end")
	if start.IsZero() {
		start = queryDate(c, "date_from")
	}
	if end.IsZero() {
		end = queryDate(c, "date_to")
	}
	if start.IsZero() || end.IsZero() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "start/end (or date_from/date_to) are required"})
		return
	}
	if end.Before(start) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "end must be >= start"})
		return
	}
	if !enforceMaxRange(start, end) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "date range must not exceed 6 months"})
		return
	}

	accounts, showNone := groupFilter(groupID)
	if showNone {
		c.JSON(http.StatusOK, gin.H{"group_id": groupID, "items": []gin.H{}, "total_consumed": 0})
		return
	}
	if len(accounts) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "group has no members"})
		return
	}

	account := strings.TrimSpace(c.Query("account"))
	if account != "" {
		visible := false
		for _, a := range accounts {
			if a == account {
				visible = true
				break
			}
		}
		if !visible {
			c.JSON(http.StatusBadRequest, gin.H{"error": "account not in group"})
			return
		}
	}

	type aggRow struct {
		ObjectType string  `json:"object_type"`
		Sum        float64 `json:"sum"`
	}
	var agg []aggRow
	q := db.PG.Table("local_efforts").
		Select("LOWER(TRIM(object_type)) AS object_type, COALESCE(SUM(consumed), 0) AS sum").
		Where("deleted = false").
		Where("work_date BETWEEN ? AND ?", start, end)
	if account != "" {
		q = q.Where("account = ?", account)
	} else {
		q = q.Where("account IN ?", accounts)
	}
	if err := q.Group("LOWER(TRIM(object_type))").Scan(&agg).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var total float64
	for _, r := range agg {
		total += r.Sum
	}

	items := make([]gin.H, 0, len(agg))
	for _, r := range agg {
		ot := strings.TrimSpace(strings.ToLower(r.ObjectType))
		if ot == "" {
			ot = "other"
		}
		items = append(items, gin.H{
			"object_type":  ot,
			"category":     mapObjectTypeToCategory(ot),
			"consumed_sum": r.Sum,
			"percent":      safeRateFloat(r.Sum, total),
			"drilldown": gin.H{
				"object_type": ot,
			},
		})
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i]["consumed_sum"].(float64) > items[j]["consumed_sum"].(float64)
	})

	bugPercent := 0.0
	for _, it := range items {
		if it["object_type"].(string) == "bug" {
			bugPercent = it["percent"].(float64)
			break
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"group_id":       groupID,
		"start":          start.Format("2006-01-02"),
		"end":            end.Format("2006-01-02"),
		"account":        account,
		"items":          items,
		"total_consumed": total,
		"alerts": gin.H{
			"bug_percent": bugPercent,
		},
	})
}

func safeRateFloat(n, d float64) float64 {
	if d <= 0 {
		return 0
	}
	return n / d
}

func mapObjectTypeToCategory(ot string) string {
	switch ot {
	case "bug":
		return "修Bug"
	case "task":
		return "任务/开发"
	case "story":
		return "需求"
	default:
		return "其它"
	}
}
