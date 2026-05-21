package handlers

import (
	"net/http"
	"zenmind/internal/config"
	"zenmind/internal/db"
	"zenmind/internal/scheduler"

	"github.com/gin-gonic/gin"
)

// GetSyncSettings GET /api/config/sync-settings
func GetSyncSettings(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"interval_minutes":         config.Global.SyncIntervalMinutes,
		"effort_reconcile_enabled": config.Global.EffortReconcileEnabled,
		"effort_reconcile_hour":    config.ClampEffortReconcileHour(config.Global.EffortReconcileHour),
		"effort_reconcile_days":    config.ClampEffortReconcileDays(config.Global.EffortReconcileDays),
	})
}

type putSyncSettingsBody struct {
	IntervalMinutes int `json:"interval_minutes" binding:"required,min=1,max=1440"`
}

// PutSyncSettings PUT /api/config/sync-settings — persists interval and applies without restart.
func PutSyncSettings(c *gin.Context) {
	var req putSyncSettingsBody
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	n := config.ClampSyncIntervalMinutes(req.IntervalMinutes)
	if err := db.SetSyncIntervalMinutes(n); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	config.Global.SyncIntervalMinutes = n
	scheduler.NotifySyncIntervalChanged()
	c.JSON(http.StatusOK, gin.H{
		"message":          "sync interval updated",
		"interval_minutes": n,
	})
}
