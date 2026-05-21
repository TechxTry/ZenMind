package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
	"zenmind/internal/config"
	"zenmind/internal/db"
	"zenmind/internal/redisclient"
	"zenmind/internal/zentao"

	"github.com/gin-gonic/gin"
)

type probeBody struct {
	TaskID int64 `json:"task_id"`
}

// ProbeZentaoAuth POST /api/zentao/auth/probe
// 基于当前 JWT 用户已绑定的禅道会话 Cookie，探测真实的报工 URL / 表单字段 / CSRF 名称。
// 纯 GET 探测，不产生任何数据副作用。
func ProbeZentaoAuth(c *gin.Context) {
	sub := currentSub(c)
	if sub == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing sub"})
		return
	}

	var req probeBody
	_ = c.ShouldBindJSON(&req)

	baseURL := strings.TrimSpace(config.Global.ZentaoBaseURL)
	if baseURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "zentao base_url not configured"})
		return
	}

	if err := ensureRedis(c.Request.Context()); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "redis unavailable: " + err.Error()})
		return
	}
	key := ztSessKeyPrefix + sub
	raw, err := redisclient.Client.Get(c.Request.Context(), key).Result()
	if err != nil || strings.TrimSpace(raw) == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "zentao session not bound or expired"})
		return
	}
	var cookies []*http.Cookie
	if err := json.Unmarshal([]byte(raw), &cookies); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid session cookie payload"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	opts := zentao.ProbeOptions{}
	if acct, pwd, ok, err := db.GetZentaoCredential(sub); err == nil && ok {
		reqAcct, bindErr := requiredBoundAccount(c)
		if bindErr != nil {
			// 无账号管理绑定时不在探测中带已存密码，避免误用历史凭证
		} else if acct != reqAcct {
			c.JSON(http.StatusForbidden, gin.H{"ok": false, "error": "已保存禅道账号与账号管理绑定不一致，请先在账号管理中更正绑定或清除后重新授权"})
			return
		} else {
			opts.APILoginAccount = acct
			opts.APILoginPassword = pwd
		}
	}

	r, err := zentao.ProbeWithOptions(ctx, baseURL, cookies, req.TaskID, opts)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "result": r})
}
