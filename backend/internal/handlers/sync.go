package handlers

import (
	"net/http"
	"zenmind/internal/db"
	"zenmind/internal/etl"
	"zenmind/internal/models"

	"github.com/gin-gonic/gin"
)

// TriggerSync POST /api/sync/trigger — runs ETL in background goroutine
func TriggerSync(c *gin.Context) {
	if _, ok := RequireAdmin(c); !ok {
		return
	}
	if db.GetZentao() == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Zentao datasource not configured"})
		return
	}
	go etl.RunAll()
	c.JSON(http.StatusAccepted, gin.H{"message": "sync started"})
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
