package handlers

import (
	"net/http"
	"zenmind/internal/db"
	"zenmind/internal/models"

	"github.com/gin-gonic/gin"
)

// ListUsers GET /api/users?q=keyword&page=1&page_size=50
func ListUsers(c *gin.Context) {
	q := c.Query("q")
	page, pageSize := parsePagination(c)

	query := db.PG.Model(&models.LocalUser{}).Where("deleted = false")
	if q != "" {
		like := "%" + q + "%"
		query = query.Where("realname ILIKE ? OR account ILIKE ?", like, like)
	}

	var total int64
	query.Count(&total)

	var users []models.LocalUser
	query.Offset((page - 1) * pageSize).Limit(pageSize).Find(&users)

	c.JSON(http.StatusOK, pageResponse(users, total, page, pageSize))
}
