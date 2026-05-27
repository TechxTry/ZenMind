package handlers

import (
	"net/http"
	"strconv"
	"strings"
	"time"
	"zenmind/internal/db"
	"zenmind/internal/models"

	"github.com/gin-gonic/gin"
)

type createMCPTokenBody struct {
	Name       string `json:"name"`
	ExpireDays int    `json:"expire_days"`
}

func ListMyMCPTokens(c *gin.Context) {
	cu := GetCurrentUser(c)
	if cu == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	rows, err := db.ListMCPAccessTokensByUser(cu.User.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": rows})
}

func CreateMyMCPToken(c *gin.Context) {
	cu := GetCurrentUser(c)
	if cu == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	var req createMCPTokenBody
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = "我的 MCP 密钥"
	}
	days := req.ExpireDays
	if days <= 0 {
		days = 90
	}
	if days > 365 {
		days = 365
	}
	exp := time.Now().Add(time.Duration(days) * 24 * time.Hour)
	row, raw, err := db.CreateMCPAccessToken(cu.User.ID, name, &exp)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	_ = db.WriteAudit(db.AuditInput{
		ActorUserID:   &cu.User.ID,
		ActorUsername: cu.User.Username,
		Action:        "mcp_token_created",
		TargetType:    "mcp_access_token",
		TargetID:      strconv.FormatInt(row.ID, 10),
		Metadata:      models.JSONB{"token_name": row.TokenName, "expire_days": days},
		IP:            c.ClientIP(),
		UA:            c.GetHeader("User-Agent"),
	})
	c.JSON(http.StatusOK, gin.H{
		"id":         row.ID,
		"token":      raw,
		"token_name": row.TokenName,
		"expires_at": row.ExpiresAt,
	})
}

func DeleteMyMCPToken(c *gin.Context) {
	cu := GetCurrentUser(c)
	if cu == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	id64, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	id := id64
	if id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := db.RevokeMCPAccessToken(cu.User.ID, id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	_ = db.WriteAudit(db.AuditInput{
		ActorUserID:   &cu.User.ID,
		ActorUsername: cu.User.Username,
		Action:        "mcp_token_revoked",
		TargetType:    "mcp_access_token",
		TargetID:      strconv.FormatInt(id, 10),
		IP:            c.ClientIP(),
		UA:            c.GetHeader("User-Agent"),
	})
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
