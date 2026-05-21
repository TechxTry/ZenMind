package handlers

import (
	"net/http"
	"zenmind/internal/db"

	"github.com/gin-gonic/gin"
)

// GetBusinessConfig GET /api/config/business
func GetBusinessConfig(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"daily_standard_hours": db.GetDailyStandardHours(),
	})
}

type putBusinessConfigBody struct {
	DailyStandardHours int `json:"daily_standard_hours" binding:"required,min=1,max=24"`
}

// PutBusinessConfig PUT /api/config/business
func PutBusinessConfig(c *gin.Context) {
	if _, ok := RequireAdmin(c); !ok {
		return
	}
	var req putBusinessConfigBody
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := db.SetDailyStandardHours(req.DailyStandardHours); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"message":              "business config updated",
		"daily_standard_hours": req.DailyStandardHours,
	})
}
