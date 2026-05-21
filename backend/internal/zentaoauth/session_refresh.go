package zentaoauth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
	"zenmind/internal/config"
	"zenmind/internal/db"
	"zenmind/internal/redisclient"
	"zenmind/internal/zentao"
)

const SessionKeyPrefix = "zentao:sess:"

func ensureRedis(ctx context.Context) error {
	if redisclient.Client == nil {
		redisclient.Init()
	}
	return redisclient.Ping(ctx)
}

func requiredBoundAccountByUsername(username string) (string, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return "", fmt.Errorf("missing sub")
	}
	u, ok, err := db.GetSystemUserByUsername(username)
	if err != nil {
		return "", err
	}
	if !ok || u.Disabled {
		return "", fmt.Errorf("user disabled or missing")
	}
	binding, ok, err := db.GetZentaoBindingBySystemUserID(u.ID)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("请先在账号管理中绑定禅道账号")
	}
	acct := strings.TrimSpace(binding.ZentaoAccount)
	if acct == "" {
		return "", fmt.Errorf("当前账号未配置有效的禅道绑定")
	}
	return acct, nil
}

func SaveSessionCookies(ctx context.Context, sub string, cookies []*http.Cookie) error {
	if err := ensureRedis(ctx); err != nil {
		return fmt.Errorf("redis unavailable: %w", err)
	}
	b, err := json.Marshal(cookies)
	if err != nil {
		return fmt.Errorf("marshal session failed: %w", err)
	}
	key := SessionKeyPrefix + sub
	saveCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := redisclient.Client.Set(saveCtx, key, string(b), 12*time.Hour).Err(); err != nil {
		return fmt.Errorf("redis save session failed: %w", err)
	}
	return nil
}

// RefreshSavedSession 使用库中已保存的账号密码重建 Redis 会话。
// 该函数供 HTTP 接口与后台定时任务共同复用。
func RefreshSavedSession(ctx context.Context, sub string) (account string, finalURL string, err error) {
	sub = strings.TrimSpace(sub)
	if sub == "" {
		return "", "", fmt.Errorf("missing sub")
	}
	acct, pwd, ok, err := db.GetZentaoCredential(sub)
	if err != nil {
		return "", "", err
	}
	if !ok {
		return "", "", fmt.Errorf("no saved zentao credential")
	}
	loginURL := strings.TrimSpace(config.Global.ZentaoLoginURL)
	if loginURL == "" {
		return "", "", fmt.Errorf("zentao login_url not configured")
	}
	requiredAcct, bindErr := requiredBoundAccountByUsername(sub)
	if bindErr != nil {
		return "", "", bindErr
	}
	if acct != requiredAcct {
		return "", "", fmt.Errorf("已保存禅道账号与账号管理绑定不一致，请联系管理员处理")
	}
	r, err := zentao.LoginByForm(ctx, loginURL, acct, pwd)
	if err != nil {
		return "", "", err
	}
	if err := SaveSessionCookies(ctx, sub, r.Cookies); err != nil {
		return "", "", err
	}
	return acct, r.FinalURL, nil
}
