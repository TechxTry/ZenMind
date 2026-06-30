package handlers

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
	"zenmind/internal/db"
	"zenmind/internal/extcal"
	"zenmind/internal/models"

	"github.com/gin-gonic/gin"
)

const maxCalendarAggregateDays = 120
const maxExternalCalendarFetchConcurrency = 4

func calendarTimeOK(t *time.Time) bool {
	if t == nil || t.IsZero() || t.Year() <= 1 {
		return false
	}
	return true
}

// taskEffortSpan returns inclusive local-midnight bounds from efforts linked to a task.
func taskEffortSpan(efforts []models.LocalEffort, taskID int64) (start, end time.Time, ok bool) {
	for _, e := range efforts {
		if strings.ToLower(strings.TrimSpace(e.ObjectType)) != "task" || e.ObjectID != taskID {
			continue
		}
		if !calendarTimeOK(e.WorkDate) {
			continue
		}
		d := e.WorkDate.In(time.Local)
		day := time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.Local)
		if !ok {
			start, end, ok = day, day, true
			continue
		}
		if day.Before(start) {
			start = day
		}
		if day.After(end) {
			end = day
		}
	}
	return start, end, ok
}

// taskMetaSpan derives a fallback plan window when there is no effort history in range.
// Prefer deadline/assigned over opened_date so long-lived affair tasks do not paint the whole month.
func taskMetaSpan(t models.LocalTask) (startAt, endAt *time.Time) {
	for _, c := range []*time.Time{t.DeadlineDate, t.StartedDate, t.AssignedDate, t.OpenedDate} {
		if calendarTimeOK(c) {
			startAt = c
			break
		}
	}
	for _, c := range []*time.Time{t.FinishedDate, t.ClosedDate, t.DeadlineDate} {
		if calendarTimeOK(c) {
			endAt = c
			break
		}
	}
	if startAt == nil {
		return nil, nil
	}
	if endAt == nil {
		endAt = startAt
	}
	return startAt, endAt
}

func calendarDayBounds(startAt, endAt time.Time) (startDay, endDay time.Time) {
	startLocal := startAt.In(time.Local)
	endLocal := endAt.In(time.Local)
	if endLocal.Before(startLocal) {
		endLocal = startLocal
	}
	startDay = time.Date(startLocal.Year(), startLocal.Month(), startLocal.Day(), 0, 0, 0, 0, time.Local)
	endDay = time.Date(endLocal.Year(), endLocal.Month(), endLocal.Day(), 23, 59, 59, 0, time.Local)
	return startDay, endDay
}

func calendarWindowOK(from, to time.Time) bool {
	if from.IsZero() || to.IsZero() || to.Before(from) {
		return false
	}
	return to.Sub(from) <= maxCalendarAggregateDays*24*time.Hour
}

// ListMyCalendarFeeds GET /api/me/calendar-feeds
func ListMyCalendarFeeds(c *gin.Context) {
	cu := GetCurrentUser(c)
	if cu == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	rows, err := db.ListUserCalendarFeeds(cu.User.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	type item struct {
		ID       int64  `json:"id"`
		Name     string `json:"name"`
		FeedHost string `json:"feed_host"`
		Color    string `json:"color"`
	}
	out := make([]item, 0, len(rows))
	for _, r := range rows {
		out = append(out, item{ID: r.ID, Name: r.Name, FeedHost: r.FeedHost, Color: r.Color})
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

type createCalendarFeedRequest struct {
	Name    string `json:"name" binding:"required"`
	ICalURL string `json:"ical_url" binding:"required"`
	Color   string `json:"color"`
}

// CreateMyCalendarFeed POST /api/me/calendar-feeds
func CreateMyCalendarFeed(c *gin.Context) {
	cu := GetCurrentUser(c)
	if cu == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	var req createCalendarFeedRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" || len(name) > 120 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid name"})
		return
	}
	if len(req.ICalURL) > 4096 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ical_url too long"})
		return
	}
	u, err := extcal.ValidateCalendarURL(req.ICalURL)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid ical_url: " + err.Error()})
		return
	}
	color := strings.TrimSpace(req.Color)
	if color == "" {
		color = "#6366F1"
	}
	if len(color) > 24 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid color"})
		return
	}
	id, err := db.InsertUserCalendarFeed(cu.User.ID, name, extcal.FeedHost(u), strings.TrimSpace(req.ICalURL), color)
	if err != nil {
		if strings.Contains(err.Error(), "limit reached") {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": id})
}

// DeleteMyCalendarFeed DELETE /api/me/calendar-feeds/:id
func DeleteMyCalendarFeed(c *gin.Context) {
	cu := GetCurrentUser(c)
	if cu == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	ok, err := db.DeleteUserCalendarFeed(cu.User.ID, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// GetMyCalendarAggregate GET /api/me/calendar-aggregate?date_from=YYYY-MM-DD&date_to=YYYY-MM-DD
func GetMyCalendarAggregate(c *gin.Context) {
	cu := GetCurrentUser(c)
	if cu == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	dateFrom := queryDate(c, "date_from")
	dateTo := queryDate(c, "date_to")
	if dateFrom.IsZero() || dateTo.IsZero() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "date_from and date_to are required"})
		return
	}
	if !calendarWindowOK(dateFrom, dateTo) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid date range (max 120 days)"})
		return
	}

	efforts := make([]models.LocalEffort, 0)
	if cu.ZentaoBinding != nil {
		acc := strings.TrimSpace(cu.ZentaoBinding.ZentaoAccount)
		if acc != "" {
			q := db.PG.Model(&models.LocalEffort{}).Where("deleted = false").
				Where("account = ?", acc).
				Where("work_date BETWEEN ? AND ?", dateFrom, dateTo).
				Order("work_date ASC, id ASC").Limit(500)
			if err := q.Find(&efforts).Error; err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		}
	}

	// Tasks: convert plan span into all-day multi-day events on the month calendar.
	// If the task has efforts in range, use min/max work_date (matches actual 报工 days).
	// Otherwise: start = deadline || started || assigned || opened;
	//            end   = finished || closed || deadline || start.
	tasks := make([]models.LocalTask, 0)
	var acc string
	if cu.ZentaoBinding != nil {
		acc = strings.TrimSpace(cu.ZentaoBinding.ZentaoAccount)
	}
	if acc != "" {
		// Date overlap filter using DATE(..) casts to avoid timezone mismatch.
		// Overlap rule (inclusive days):
		//   start_day <= dateTo AND end_day >= dateFrom
		startExpr := "COALESCE(deadline_date, started_date, assigned_date, opened_date)"
		endExpr := "COALESCE(finished_date, NULLIF(closed_date, '0001-01-01'::timestamptz), deadline_date, COALESCE(deadline_date, started_date, assigned_date, opened_date))"
		q := db.PG.Model(&models.LocalTask{}).
			Where("deleted = false").
			Where("assigned_to = ?", acc).
			Where(startExpr+" IS NOT NULL").
			Where(startExpr+"::date <= ?", dateTo).
			Where(endExpr+"::date >= ?", dateFrom).
			Order("id DESC").
			Limit(300)
		if err := q.Select("id, name, status, assigned_to, opened_date, started_date, assigned_date, deadline_date, closed_date, finished_date").Find(&tasks).Error; err != nil {
			// Fail open: tasks not critical for calendar; keep returning efforts/external.
			tasks = make([]models.LocalTask, 0)
		}
	}

	feeds, err := db.ListUserCalendarFeedsForFetch(cu.User.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	type extItem struct {
		SourceType string `json:"source_type"` // feed | account
		SourceID   int64  `json:"source_id"`
		SourceName string `json:"source_name"`
		Title      string `json:"title"`
		Start      string `json:"start"`
		End        string `json:"end"`
		AllDay     bool   `json:"all_day"`
		Color      string `json:"color"`
	}
	external := make([]extItem, 0)
	feedErrors := make([]gin.H, 0)
	accountErrors := make([]gin.H, 0)

	ctx, cancel := context.WithTimeout(c.Request.Context(), 45*time.Second)
	defer cancel()

	// Map tasks first so they can contribute to month dots and day detail immediately.
	statusColor := func(s string) string {
		switch strings.ToLower(strings.TrimSpace(s)) {
		case "doing", "active":
			return "#1677ff"
		case "wait":
			return "#fa8c16"
		case "done", "resolved":
			return "#52c41a"
		case "closed", "pause":
			return "#8c8c8c"
		case "rejected", "cancel":
			return "#ff4d4f"
		default:
			return "#6366F1"
		}
	}

	for _, t := range tasks {
		var startDay, endDay time.Time
		if es, ee, ok := taskEffortSpan(efforts, t.ID); ok {
			startDay, endDay = calendarDayBounds(es, ee)
		} else {
			startAt, endAt := taskMetaSpan(t)
			if startAt == nil {
				continue
			}
			startDay, endDay = calendarDayBounds(*startAt, *endAt)
		}

		external = append(external, extItem{
			SourceType: "task",
			SourceID:   t.ID,
			SourceName: t.AssignedTo,
			Title:      t.Name,
			Start:      startDay.Format(time.RFC3339),
			End:        endDay.Format(time.RFC3339),
			AllDay:     true,
			Color:      statusColor(t.Status),
		})
	}

	accounts, err := db.ListUserCalendarAccountsForFetch(cu.User.ID)
	if err != nil {
		accountErrors = append(accountErrors, gin.H{"account_id": 0, "type": "accounts", "username": "日历账户", "error": err.Error()})
		accounts = nil
	}

	type fetchResult struct {
		external      []extItem
		feedErrors    []gin.H
		accountErrors []gin.H
	}

	jobs := make([]func() fetchResult, 0, len(feeds)+len(accounts))
	for _, f := range feeds {
		f := f
		jobs = append(jobs, func() fetchResult {
			body, err := extcal.FetchICS(ctx, f.ICalURL)
			if err != nil {
				return fetchResult{feedErrors: []gin.H{{"feed_id": f.ID, "feed_name": f.Name, "error": err.Error()}}}
			}
			parsed, err := extcal.EventsInWindow(body, dateFrom, dateTo)
			if err != nil {
				return fetchResult{feedErrors: []gin.H{{"feed_id": f.ID, "feed_name": f.Name, "error": err.Error()}}}
			}
			col := f.Color
			if strings.TrimSpace(col) == "" {
				col = "#6366F1"
			}
			items := make([]extItem, 0, len(parsed))
			for _, ev := range parsed {
				items = append(items, extItem{
					SourceType: "feed",
					SourceID:   f.ID,
					SourceName: f.Name,
					Title:      ev.Title,
					Start:      ev.Start.Format(time.RFC3339),
					End:        ev.End.Format(time.RFC3339),
					AllDay:     ev.AllDay,
					Color:      col,
				})
			}
			return fetchResult{external: items}
		})
	}
	for _, a := range accounts {
		a := a
		jobs = append(jobs, func() fetchResult {
			var parsed []extcal.ParsedEvent
			var e error
			switch strings.ToLower(strings.TrimSpace(a.Type)) {
			case "caldav":
				parsed, e = extcal.FetchCalDAVEvents(ctx, a.Server, a.Username, a.Password, dateFrom, dateTo)
			case "exchange":
				parsed, e = extcal.FetchExchangeBusyEvents(ctx, a.Server, a.Username, a.Password, dateFrom, dateTo)
			default:
				return fetchResult{}
			}
			if e != nil {
				return fetchResult{accountErrors: []gin.H{{"account_id": a.ID, "type": a.Type, "username": a.Username, "error": e.Error()}}}
			}
			color := "#7C3AED"
			if strings.EqualFold(a.Type, "exchange") {
				color = "#2563EB"
			}
			srcName := a.Description
			if strings.TrimSpace(srcName) == "" {
				srcName = a.Username
			}
			items := make([]extItem, 0, len(parsed))
			for _, ev := range parsed {
				items = append(items, extItem{
					SourceType: "account",
					SourceID:   a.ID,
					SourceName: srcName,
					Title:      ev.Title,
					Start:      ev.Start.Format(time.RFC3339),
					End:        ev.End.Format(time.RFC3339),
					AllDay:     ev.AllDay,
					Color:      color,
				})
			}
			return fetchResult{external: items}
		})
	}

	if len(jobs) > 0 {
		workerCount := maxExternalCalendarFetchConcurrency
		if len(jobs) < workerCount {
			workerCount = len(jobs)
		}

		jobCh := make(chan func() fetchResult, len(jobs))
		resultCh := make(chan fetchResult, len(jobs))
		var wg sync.WaitGroup
		for i := 0; i < workerCount; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for job := range jobCh {
					resultCh <- job()
				}
			}()
		}
		for _, job := range jobs {
			jobCh <- job
		}
		close(jobCh)
		wg.Wait()
		close(resultCh)

		for r := range resultCh {
			external = append(external, r.external...)
			feedErrors = append(feedErrors, r.feedErrors...)
			accountErrors = append(accountErrors, r.accountErrors...)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"efforts":        effortsWithTitles(efforts),
		"external":       external,
		"feed_errors":    feedErrors,
		"account_errors": accountErrors,
	})
}
