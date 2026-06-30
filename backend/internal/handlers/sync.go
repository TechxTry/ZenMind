package handlers

import (
	"net/http"
	"zenmind/internal/config"
	"zenmind/internal/db"
	"zenmind/internal/etl"
	"zenmind/internal/models"

	"github.com/gin-gonic/gin"
)

// TriggerSync POST /api/sync/trigger — runs ETL in background goroutine
func TriggerSync(c *gin.Context) {
	cu, ok := RequireAdmin(c)
	if !ok {
		return
	}
	if db.GetZentao() == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Zentao datasource not configured"})
		return
	}
	ctx := etl.RunContext{Source: etl.TriggerManual, Actor: cu.User.Username}
	go etl.RunAllWith(ctx)
	c.JSON(http.StatusAccepted, gin.H{"message": "增量同步已启动"})
}

// TriggerEffortReconcile POST /api/sync/effort-reconcile — effort date-window reconcile (admin).
func TriggerEffortReconcile(c *gin.Context) {
	cu, ok := RequireAdmin(c)
	if !ok {
		return
	}
	if db.GetZentao() == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Zentao datasource not configured"})
		return
	}
	if !config.Global.EffortReconcileEnabled {
		c.JSON(http.StatusBadRequest, gin.H{"error": "报工回刷已在环境变量中关闭（EFFORT_RECONCILE_ENABLED=false）"})
		return
	}
	if etl.IsEffortReconcileRunning() {
		c.JSON(http.StatusConflict, gin.H{
			"error":   "回刷报工任务已在执行，请稍后再试",
			"running": "effort_daily_reconcile",
		})
		return
	}
	ctx := etl.RunContext{Source: etl.TriggerManual, Actor: cu.User.Username}
	go etl.RunEffortsDailyReconcileWith(ctx)
	days := config.ClampEffortReconcileDays(config.Global.EffortReconcileDays)
	c.JSON(http.StatusAccepted, gin.H{
		"message": "报工回刷已启动",
		"days":    days,
	})
}

// GetSyncActive GET /api/sync/active — in-flight ETL jobs (admin).
func GetSyncActive(c *gin.Context) {
	if _, ok := RequireAdmin(c); !ok {
		return
	}
	runs := etl.ActiveRuns()
	c.JSON(http.StatusOK, gin.H{"running": runs, "busy": len(runs) > 0})
}

// GetSyncStatus GET /api/sync/status
func GetSyncStatus(c *gin.Context) {
	tables := []string{
		"local_users",
		"local_tasks",
		"local_stories",
		"local_bugs",
		"local_efforts",
		"local_programs",
		"local_projects",
		"local_product_lines",
		"local_products",
		"local_executions",
	}
	var watermarks []models.SyncWatermark
	db.PG.Where("table_name IN ?", tables).Find(&watermarks)

	result := make(map[string]interface{})
	for _, wm := range watermarks {
		result[wm.Table] = gin.H{
			"watermark":  wm.Watermark,
			"last_count": wm.LastCount,
			"updated_at": wm.UpdatedAt,
		}
	}
	c.JSON(http.StatusOK, gin.H{"tables": result})
}

// ListSyncLogs GET /api/sync/logs — recent sync / effort-reconcile run history (admin).
func ListSyncLogs(c *gin.Context) {
	if _, ok := RequireAdmin(c); !ok {
		return
	}
	page, pageSize := parsePagination(c)
	rows, total, err := db.ListSyncRunLogs((page-1)*pageSize, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, pageResponse(rows, total, page, pageSize))
}
