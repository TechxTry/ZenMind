package handlers

import (
	"net/http"
	"time"
	"zenmind/internal/db"
	"zenmind/internal/models"

	"github.com/gin-gonic/gin"
)

// AdminListAuditLogs GET /api/admin/audit-logs?action=&actor=&from=&to=&page=&page_size=
func AdminListAuditLogs(c *gin.Context) {
	if _, ok := RequireAdmin(c); !ok {
		return
	}
	action := c.Query("action")
	actor := c.Query("actor")
	from := queryDate(c, "from")
	to := queryDate(c, "to")
	if !to.IsZero() {
		to = time.Date(to.Year(), to.Month(), to.Day(), 23, 59, 59, 0, time.Local)
	}

	page, pageSize := parsePagination(c)

	query := db.PG.Model(&models.AuditLog{})
	if action != "" {
		query = query.Where("action = ?", action)
	}
	if actor != "" {
		query = query.Where("actor_username ILIKE ?", "%"+actor+"%")
	}
	if !from.IsZero() {
		query = query.Where("created_at >= ?", from)
	}
	if !to.IsZero() {
		query = query.Where("created_at <= ?", to)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var rows []models.AuditLog
	if err := query.Order("id DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, pageResponse(rows, total, page, pageSize))
}
