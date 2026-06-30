package handlers

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"
	"zenmind/internal/db"
	"zenmind/internal/extcal"

	"github.com/gin-gonic/gin"
)

func normalizeCalendarAccountType(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	switch s {
	case "exchange", "caldav":
		return s
	default:
		return ""
	}
}

// ListMyCalendarAccounts GET /api/me/calendar-accounts
func ListMyCalendarAccounts(c *gin.Context) {
	cu := GetCurrentUser(c)
	if cu == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	rows, err := db.ListUserCalendarAccounts(cu.User.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	type item struct {
		ID          int64  `json:"id"`
		Type        string `json:"type"`
		Server      string `json:"server"`
		Username    string `json:"username"`
		Description string `json:"description"`
		CreatedAt   string `json:"created_at"`
	}
	out := make([]item, 0, len(rows))
	for _, r := range rows {
		out = append(out, item{
			ID:          r.ID,
			Type:        r.Type,
			Server:      r.Server,
			Username:    r.Username,
			Description: r.Description,
			CreatedAt:   r.CreatedAt.Format(time.RFC3339),
		})
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

type createCalendarAccountRequest struct {
	Type        string `json:"type" binding:"required"`
	Server      string `json:"server"`
	Username    string `json:"username" binding:"required"`
	Password    string `json:"password" binding:"required"`
	Description string `json:"description"`
}

func normalizeCalendarAccountRequest(req createCalendarAccountRequest) (typ, server, username, password, desc string, err string) {
	typ = normalizeCalendarAccountType(req.Type)
	if typ == "" {
		err = "invalid type"
		return
	}
	server = strings.TrimSpace(req.Server)
	username = strings.TrimSpace(req.Username)
	password = strings.TrimSpace(req.Password)
	desc = strings.TrimSpace(req.Description)

	if username == "" || len(username) > 200 {
		err = "invalid username"
		return
	}
	if password == "" || len(password) > 512 {
		err = "invalid password"
		return
	}
	if len(server) > 1024 {
		err = "invalid server"
		return
	}
	if len(desc) > 200 {
		err = "invalid description"
		return
	}
	if typ == "caldav" && server == "" {
		err = "server is required for caldav"
		return
	}
	return
}

// CreateMyCalendarAccount POST /api/me/calendar-accounts
func CreateMyCalendarAccount(c *gin.Context) {
	cu := GetCurrentUser(c)
	if cu == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	var req createCalendarAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	typ, server, username, password, desc, msg := normalizeCalendarAccountRequest(req)
	if msg != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": msg})
		return
	}

	id, err := db.InsertUserCalendarAccount(cu.User.ID, typ, server, username, password, desc)
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

// TestMyCalendarAccount POST /api/me/calendar-accounts/test
func TestMyCalendarAccount(c *gin.Context) {
	cu := GetCurrentUser(c)
	if cu == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	var req createCalendarAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	typ, server, username, password, _, msg := normalizeCalendarAccountRequest(req)
	if msg != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": msg})
		return
	}

	from := time.Now().In(time.Local)
	from = time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, time.Local)
	to := from.AddDate(0, 0, 30)

	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	var (
		events int
		err    error
	)
	switch typ {
	case "caldav":
		var parsed []extcal.ParsedEvent
		parsed, err = extcal.FetchCalDAVEvents(ctx, server, username, password, from, to)
		events = len(parsed)
	case "exchange":
		var parsed []extcal.ParsedEvent
		parsed, err = extcal.FetchExchangeBusyEvents(ctx, server, username, password, from, to)
		events = len(parsed)
	default:
		err = nil
	}
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "events": events})
}

// DeleteMyCalendarAccount DELETE /api/me/calendar-accounts/:id
func DeleteMyCalendarAccount(c *gin.Context) {
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
	ok, err := db.DeleteUserCalendarAccount(cu.User.ID, id)
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
