package handlers

import (
	"fmt"
	"net/http"
	"strings"
	"zenmind/internal/config"
	"zenmind/internal/db"

	"github.com/gin-gonic/gin"
)

type datasourceConfig struct {
	Host   string `json:"host" binding:"required"`
	Port   string `json:"port" binding:"required"`
	User   string `json:"user" binding:"required"`
	Pass   string `json:"password"` // optional: empty keeps existing password in memory/env
	DBName string `json:"db_name" binding:"required"`
}

func resolveDatasourcePass(in string) (string, error) {
	if p := strings.TrimSpace(in); p != "" {
		return p, nil
	}
	if p := strings.TrimSpace(config.Global.ZentaoPass); p != "" {
		return p, nil
	}
	if p, ok, err := db.GetPersistedZentaoDatasourcePassword(); err != nil {
		return "", err
	} else if ok && strings.TrimSpace(p) != "" {
		return strings.TrimSpace(p), nil
	}
	return "", fmt.Errorf("请填写 MySQL 密码（首次配置必填；已保存过可留空沿用）")
}

func datasourcePasswordConfigured() bool {
	if strings.TrimSpace(config.Global.ZentaoPass) != "" {
		return true
	}
	p, ok, err := db.GetPersistedZentaoDatasourcePassword()
	return err == nil && ok && strings.TrimSpace(p) != ""
}

// GetDatasource GET /api/config/datasource
func GetDatasource(c *gin.Context) {
	if _, ok := RequireAdmin(c); !ok {
		return
	}
	cfg := config.Global
	c.JSON(http.StatusOK, gin.H{
		"host":                cfg.ZentaoHost,
		"port":                cfg.ZentaoPort,
		"user":                cfg.ZentaoUser,
		"db_name":             cfg.ZentaoDBName,
		"password_configured": datasourcePasswordConfigured(),
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
	pass, err := resolveDatasourcePass(req.Pass)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	config.Global.ZentaoHost = req.Host
	config.Global.ZentaoPort = req.Port
	config.Global.ZentaoUser = req.User
	config.Global.ZentaoPass = pass
	config.Global.ZentaoDBName = req.DBName

	if err := db.ConnectZentao(config.Global); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "connection failed: " + err.Error()})
		return
	}
	if err := db.SaveZentaoDatasource(req.Host, req.Port, req.User, req.DBName, pass); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存配置失败: " + err.Error()})
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
	pass, err := resolveDatasourcePass(req.Pass)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error()})
		return
	}

	testCfg := config.Global
	testCfg.ZentaoHost = req.Host
	testCfg.ZentaoPort = req.Port
	testCfg.ZentaoUser = req.User
	testCfg.ZentaoPass = pass
	testCfg.ZentaoDBName = req.DBName

	if err := db.ConnectZentao(testCfg); err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
