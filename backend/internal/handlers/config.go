package handlers

import (
	"net/http"
	"zenmind/internal/config"
	"zenmind/internal/db"

	"github.com/gin-gonic/gin"
)

type datasourceConfig struct {
	Host   string `json:"host" binding:"required"`
	Port   string `json:"port" binding:"required"`
	User   string `json:"user" binding:"required"`
	Pass   string `json:"password" binding:"required"`
	DBName string `json:"db_name" binding:"required"`
}

// GetDatasource GET /api/config/datasource
func GetDatasource(c *gin.Context) {
	if _, ok := RequireAdmin(c); !ok {
		return
	}
	cfg := config.Global
	c.JSON(http.StatusOK, gin.H{
		"host":    cfg.ZentaoHost,
		"port":    cfg.ZentaoPort,
		"user":    cfg.ZentaoUser,
		"db_name": cfg.ZentaoDBName,
		// password intentionally omitted
	})
}

// PutDatasource PUT /api/config/datasource
func PutDatasource(c *gin.Context) {
	if _, ok := RequireAdmin(c); !ok {
		return
	}
	var req datasourceConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	config.Global.ZentaoHost = req.Host
	config.Global.ZentaoPort = req.Port
	config.Global.ZentaoUser = req.User
	config.Global.ZentaoPass = req.Pass
	config.Global.ZentaoDBName = req.DBName

	if err := db.ConnectZentao(config.Global); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "connection failed: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "datasource connected successfully"})
}

// TestDatasource POST /api/config/datasource/test
func TestDatasource(c *gin.Context) {
	if _, ok := RequireAdmin(c); !ok {
		return
	}
	var req datasourceConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	testCfg := config.Global
	testCfg.ZentaoHost = req.Host
	testCfg.ZentaoPort = req.Port
	testCfg.ZentaoUser = req.User
	testCfg.ZentaoPass = req.Pass
	testCfg.ZentaoDBName = req.DBName

	if err := db.ConnectZentao(testCfg); err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
