package handlers

import (
	"net/http"
	"strconv"
	"zenmind/internal/db"

	"github.com/gin-gonic/gin"
)

type adminSetBindingReq struct {
	ZentaoAccount string `json:"zentao_account" binding:"required"`
}

// AdminGetZentaoBinding GET /api/admin/system-users/:id/zentao-binding
func AdminGetZentaoBinding(c *gin.Context) {
	if _, ok := RequireAdmin(c); !ok {
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	b, ok, err := db.GetZentaoBindingBySystemUserID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !ok {
		c.JSON(http.StatusOK, gin.H{"bound": false})
		return
	}
	c.JSON(http.StatusOK, gin.H{"bound": true, "binding": b})
}

// AdminSetZentaoBinding PUT /api/admin/system-users/:id/zentao-binding
func AdminSetZentaoBinding(c *gin.Context) {
	cu, ok := RequireAdmin(c)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var req adminSetBindingReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	b, err := db.SetZentaoBinding(id, req.ZentaoAccount)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	_ = db.WriteAudit(db.AuditInput{
		ActorUserID:   &cu.User.ID,
		ActorUsername: cu.User.Username,
		Action:        "admin_set_zentao_binding",
		TargetType:    "system_user",
		TargetID:      strconv.FormatInt(id, 10),
		Metadata:      db.RowToJSONB(map[string]interface{}{"zentao_account": req.ZentaoAccount}),
		IP:            c.ClientIP(),
		UA:            c.GetHeader("User-Agent"),
	})
	c.JSON(http.StatusOK, gin.H{"ok": true, "binding": b})
}

// AdminDeleteZentaoBinding DELETE /api/admin/system-users/:id/zentao-binding
func AdminDeleteZentaoBinding(c *gin.Context) {
	cu, ok := RequireAdmin(c)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := db.DeleteZentaoBinding(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	_ = db.WriteAudit(db.AuditInput{
		ActorUserID:   &cu.User.ID,
		ActorUsername: cu.User.Username,
		Action:        "admin_delete_zentao_binding",
		TargetType:    "system_user",
		TargetID:      strconv.FormatInt(id, 10),
		IP:            c.ClientIP(),
		UA:            c.GetHeader("User-Agent"),
	})
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
