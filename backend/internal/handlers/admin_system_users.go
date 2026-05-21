package handlers

import (
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"
	"zenmind/internal/db"
	"zenmind/internal/models"

	"github.com/gin-gonic/gin"
)

type adminCreateSystemUserReq struct {
	Username       string `json:"username" binding:"required"`
	DisplayName    string `json:"display_name"`
	Password       string `json:"password" binding:"required"`
	Role           string `json:"role"`
	DataScope      string `json:"data_scope"`
	DefaultGroupID *int   `json:"default_group_id"`
}

type adminUpdateSystemUserReq struct {
	DisplayName    *string `json:"display_name"`
	Role           *string `json:"role"`
	DataScope      *string `json:"data_scope"`
	DefaultGroupID **int   `json:"default_group_id"`
	Disabled       *bool   `json:"disabled"`
}

// AdminListSystemUsers GET /api/admin/system-users?q=&page=&page_size=
func AdminListSystemUsers(c *gin.Context) {
	if _, ok := RequireAdmin(c); !ok {
		return
	}
	q := c.Query("q")
	page, pageSize := parsePagination(c)
	rows, total, err := db.ListSystemUsers(q, page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, pageResponse(rows, total, page, pageSize))
}

// AdminCreateSystemUser POST /api/admin/system-users
func AdminCreateSystemUser(c *gin.Context) {
	cu, ok := RequireAdmin(c)
	if !ok {
		return
	}
	var req adminCreateSystemUserReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	u, err := db.CreateSystemUser(db.CreateSystemUserInput{
		Username:       req.Username,
		DisplayName:    req.DisplayName,
		Password:       req.Password,
		Role:           req.Role,
		DataScope:      req.DataScope,
		DefaultGroupID: req.DefaultGroupID,
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	_ = db.WriteAudit(db.AuditInput{
		ActorUserID:   &cu.User.ID,
		ActorUsername: cu.User.Username,
		Action:        "admin_create_system_user",
		TargetType:    "system_user",
		TargetID:      u.Username,
		Metadata: db.RowToJSONB(map[string]interface{}{
			"role":        u.Role,
			"data_scope":  u.DataScope,
			"default_gid": u.DefaultGroupID,
		}),
		IP: c.ClientIP(),
		UA: c.GetHeader("User-Agent"),
	})
	c.JSON(http.StatusCreated, u)
}

// AdminUpdateSystemUser PATCH /api/admin/system-users/:id
func AdminUpdateSystemUser(c *gin.Context) {
	cu, ok := RequireAdmin(c)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var req adminUpdateSystemUserReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	u, ok, e := db.UpdateSystemUser(id, db.UpdateSystemUserInput{
		DisplayName:    req.DisplayName,
		Role:           req.Role,
		DataScope:      req.DataScope,
		DefaultGroupID: req.DefaultGroupID,
		Disabled:       req.Disabled,
	})
	if e != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": e.Error()})
		return
	}
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	_ = db.WriteAudit(db.AuditInput{
		ActorUserID:   &cu.User.ID,
		ActorUsername: cu.User.Username,
		Action:        "admin_update_system_user",
		TargetType:    "system_user",
		TargetID:      strconv.FormatInt(id, 10),
		Metadata:      db.RowToJSONB(req),
		IP:            c.ClientIP(),
		UA:            c.GetHeader("User-Agent"),
	})
	c.JSON(http.StatusOK, u)
}

type adminResetPasswordReq struct {
	NewPassword string `json:"new_password"`
}

func randomPassword() string {
	const letters = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789"
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	b := make([]byte, 12)
	for i := range b {
		b[i] = letters[rng.Intn(len(letters))]
	}
	return string(b)
}

// AdminResetSystemUserPassword POST /api/admin/system-users/:id/reset-password
// If new_password is omitted, server generates one and returns it once.
func AdminResetSystemUserPassword(c *gin.Context) {
	cu, ok := RequireAdmin(c)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var req adminResetPasswordReq
	_ = c.ShouldBindJSON(&req)
	pw := req.NewPassword
	if pw == "" {
		pw = randomPassword()
	}
	if err := db.ResetSystemUserPassword(id, pw); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	_ = db.WriteAudit(db.AuditInput{
		ActorUserID:   &cu.User.ID,
		ActorUsername: cu.User.Username,
		Action:        "admin_reset_password",
		TargetType:    "system_user",
		TargetID:      strconv.FormatInt(id, 10),
		IP:            c.ClientIP(),
		UA:            c.GetHeader("User-Agent"),
	})
	c.JSON(http.StatusOK, gin.H{"ok": true, "new_password": pw})
}

type adminBatchCreateSystemUsersReq struct {
	Accounts       []string `json:"accounts" binding:"required"`
	Role           string   `json:"role"`
	DataScope      string   `json:"data_scope"`
	DefaultGroupID *int     `json:"default_group_id"`
}

type adminBatchCreateCreatedUser struct {
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Password    string `json:"password"`
}

// AdminBatchCreateSystemUsers POST /api/admin/system-users/batch
// Creates system users from local_users (ZenTao personnel mirror), generates a random password for each user,
// and automatically creates 1:1 zentao bindings.
func AdminBatchCreateSystemUsers(c *gin.Context) {
	cu, ok := RequireAdmin(c)
	if !ok {
		return
	}

	var req adminBatchCreateSystemUsersReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	accountsSet := map[string]struct{}{}
	accounts := make([]string, 0, len(req.Accounts))
	for _, a := range req.Accounts {
		a = strings.TrimSpace(a)
		if a == "" {
			continue
		}
		if _, ok := accountsSet[a]; ok {
			continue
		}
		accountsSet[a] = struct{}{}
		accounts = append(accounts, a)
	}
	if len(accounts) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "accounts required"})
		return
	}

	// Resolve local_users info and validate existence.
	var localUsers []models.LocalUser
	if err := db.PG.Where("deleted = false").Where("account IN ?", accounts).Find(&localUsers).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	localMap := make(map[string]models.LocalUser, len(localUsers))
	for _, u := range localUsers {
		localMap[u.Account] = u
	}
	missing := make([]string, 0)
	for _, a := range accounts {
		if _, ok := localMap[a]; !ok {
			missing = append(missing, a)
		}
	}
	if len(missing) > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "some accounts not found in local_users", "missing": missing})
		return
	}

	// Pre-check conflicts: unique constraints on system_users.username & zentao_bindings.zentao_account.
	var existingUsers []models.SystemUser
	if err := db.PG.Where("username IN ?", accounts).Find(&existingUsers).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	existingUserSet := make(map[string]struct{}, len(existingUsers))
	for _, u := range existingUsers {
		existingUserSet[u.Username] = struct{}{}
	}

	var existingBindings []models.ZentaoBinding
	if err := db.PG.Where("zentao_account IN ?", accounts).Find(&existingBindings).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	boundSet := make(map[string]struct{}, len(existingBindings))
	for _, b := range existingBindings {
		boundSet[b.ZentaoAccount] = struct{}{}
	}

	type conflict struct {
		Account string `json:"account"`
		Reason  string `json:"reason"`
	}
	conflicts := make([]conflict, 0)
	for _, a := range accounts {
		if _, ok := existingUserSet[a]; ok {
			conflicts = append(conflicts, conflict{Account: a, Reason: "system_user_exists"})
			continue
		}
		if _, ok := boundSet[a]; ok {
			conflicts = append(conflicts, conflict{Account: a, Reason: "zentao_account_already_bound"})
			continue
		}
	}
	if len(conflicts) > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "conflicts", "conflicts": conflicts})
		return
	}

	tx := db.PG.Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": tx.Error.Error()})
		return
	}

	created := make([]adminBatchCreateCreatedUser, 0, len(accounts))
	for _, a := range accounts {
		local := localMap[a]
		pw := randomPassword()

		u, err := db.CreateSystemUserTx(tx, db.CreateSystemUserInput{
			Username:       a,
			DisplayName:    local.Realname,
			Password:       pw,
			Role:           req.Role,
			DataScope:      req.DataScope,
			DefaultGroupID: req.DefaultGroupID,
		})
		if err != nil {
			_ = tx.Rollback()
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if _, err := db.SetZentaoBindingTx(tx, u.ID, a); err != nil {
			_ = tx.Rollback()
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		created = append(created, adminBatchCreateCreatedUser{
			Username:    u.Username,
			DisplayName: u.DisplayName,
			Password:    pw,
		})
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Audit logs (best-effort; do not fail the whole request).
	for _, cuRow := range created {
		_ = db.WriteAudit(db.AuditInput{
			ActorUserID:   &cu.User.ID,
			ActorUsername: cu.User.Username,
			Action:        "admin_batch_create_system_user",
			TargetType:    "system_user",
			TargetID:      cuRow.Username,
			Metadata: db.RowToJSONB(map[string]interface{}{
				"display_name": cuRow.DisplayName,
				"role":         req.Role,
				"data_scope":   req.DataScope,
				"default_gid":  req.DefaultGroupID,
			}),
			IP: c.ClientIP(),
			UA: c.GetHeader("User-Agent"),
		})
	}

	c.JSON(http.StatusCreated, gin.H{"created": created})
}
