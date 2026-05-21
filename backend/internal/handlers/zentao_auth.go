package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"zenmind/internal/config"
	"zenmind/internal/db"
	"zenmind/internal/redisclient"
	"zenmind/internal/zentao"
	"zenmind/internal/zentaoauth"

	"github.com/gin-gonic/gin"
)

const ztSessKeyPrefix = "zentao:sess:"

type zentaoAuthPasswordBody struct {
	Password string `json:"password" binding:"required"`
}

func ensureRedis(ctx context.Context) error {
	if redisclient.Client == nil {
		redisclient.Init()
	}
	return redisclient.Ping(ctx)
}

func currentSub(c *gin.Context) string {
	if v, ok := c.Get("sub"); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func requiredBoundAccount(c *gin.Context) (string, error) {
	cu := GetCurrentUser(c)
	if cu == nil {
		return "", fmt.Errorf("unauthorized")
	}
	if cu.ZentaoBinding == nil {
		return "", fmt.Errorf("请先在账号管理中绑定禅道账号")
	}
	acct := strings.TrimSpace(cu.ZentaoBinding.ZentaoAccount)
	if acct == "" {
		return "", fmt.Errorf("当前账号未配置有效的禅道绑定")
	}
	return acct, nil
}

// GetZentaoAuthStatus GET /api/zentao/auth/status
func GetZentaoAuthStatus(c *gin.Context) {
	sub := currentSub(c)
	if sub == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing sub"})
		return
	}
	acct, credSaved, err := db.GetZentaoCredentialAccount(sub)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out := gin.H{
		"bound":             false,
		"credential_saved":  credSaved,
		"account":           acct,
		"redis_unavailable": false,
	}
	if requiredAcct, err := requiredBoundAccount(c); err == nil {
		out["required_account"] = requiredAcct
	}
	if err := ensureRedis(c.Request.Context()); err != nil {
		out["redis_unavailable"] = true
		c.JSON(http.StatusOK, out)
		return
	}
	key := ztSessKeyPrefix + sub
	n, err := redisclient.Client.Exists(c.Request.Context(), key).Result()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out["bound"] = n > 0
	c.JSON(http.StatusOK, out)
}

// TestZentaoAuth POST /api/zentao/auth/test — validates credential & connectivity; does not persist session.
func TestZentaoAuth(c *gin.Context) {
	var req zentaoAuthPasswordBody
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	loginURL := strings.TrimSpace(config.Global.ZentaoLoginURL)
	if loginURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "zentao login_url not configured"})
		return
	}
	requiredAcct, err := requiredBoundAccount(c)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"ok": false, "error": err.Error()})
		return
	}
	r, err := zentao.LoginByForm(c.Request.Context(), loginURL, requiredAcct, req.Password)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "final_url": r.FinalURL})
}

// TestZentaoAuthSaved POST /api/zentao/auth/test-saved — 使用库中已保存的账号密码测登录（无需请求体传密码）。
func TestZentaoAuthSaved(c *gin.Context) {
	sub := currentSub(c)
	if sub == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing sub"})
		return
	}
	acct, pwd, ok, err := db.GetZentaoCredential(sub)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error()})
		return
	}
	if !ok {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": "no saved zentao credential"})
		return
	}
	loginURL := strings.TrimSpace(config.Global.ZentaoLoginURL)
	if loginURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "zentao login_url not configured"})
		return
	}
	requiredAcct, bindErr := requiredBoundAccount(c)
	if bindErr != nil {
		c.JSON(http.StatusForbidden, gin.H{"ok": false, "error": bindErr.Error()})
		return
	}
	if acct != requiredAcct {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": "已保存禅道账号与账号管理绑定不一致，请联系管理员处理"})
		return
	}
	_, err = zentao.LoginByForm(c.Request.Context(), loginURL, acct, pwd)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// BindZentaoAuthSaved POST /api/zentao/auth/bind-saved — 使用库中已保存的账号密码登录并重建 Redis 会话（无需请求体传密码）。
func BindZentaoAuthSaved(c *gin.Context) {
	sub := currentSub(c)
	if sub == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing sub"})
		return
	}
	acct, finalURL, err := zentaoauth.RefreshSavedSession(c.Request.Context(), sub)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "message": "bound", "final_url": finalURL, "account": acct})
}

// BindZentaoAuth POST /api/zentao/auth/bind — login and persist session cookies in Redis.
func BindZentaoAuth(c *gin.Context) {
	sub := currentSub(c)
	if sub == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing sub"})
		return
	}
	var req zentaoAuthPasswordBody
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	loginURL := strings.TrimSpace(config.Global.ZentaoLoginURL)
	if loginURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "zentao login_url not configured"})
		return
	}
	if err := ensureRedis(c.Request.Context()); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "error": "redis unavailable: " + err.Error()})
		return
	}
	requiredAcct, err := requiredBoundAccount(c)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"ok": false, "error": err.Error()})
		return
	}
	r, err := zentao.LoginByForm(c.Request.Context(), loginURL, requiredAcct, req.Password)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error()})
		return
	}

	// Persist encrypted credential (account + password) in PG.
	if err := db.UpsertZentaoCredential(sub, requiredAcct, req.Password); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": "save credential failed: " + err.Error()})
		return
	}

	if err := zentaoauth.SaveSessionCookies(c.Request.Context(), sub, r.Cookies); err != nil {
		_ = db.DeleteZentaoCredential(sub)
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "message": "bound", "final_url": r.FinalURL})
}

// ClearZentaoAuth DELETE /api/zentao/auth/clear — deletes persisted credential and redis session.
func ClearZentaoAuth(c *gin.Context) {
	sub := currentSub(c)
	if sub == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing sub"})
		return
	}
	_ = db.DeleteZentaoCredential(sub)
	if redisclient.Client != nil {
		_ = redisclient.Client.Del(c.Request.Context(), ztSessKeyPrefix+sub).Err()
		_ = redisclient.Client.Del(c.Request.Context(), ztAPITokenKeyPrefix+sub).Err()
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
