package handlers

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"zenmind/internal/config"
	"zenmind/internal/db"
	"zenmind/internal/redisclient"
	"zenmind/internal/zentao"

	"github.com/redis/go-redis/v9"
)

const ztAPITokenKeyPrefix = "zentao:api:token:"

// apiTokenTTL 采用保守值（10h）。禅道默认 Token 有效期通常 8-24h，实际过期由调用方捕获 401 后清缓存再换。
const apiTokenTTL = 10 * time.Hour

// getCachedAPIToken 返回 Redis 里缓存的 Token；未命中返回 ""（非错误）。
func getCachedAPIToken(ctx context.Context, sub string) (string, error) {
	if err := ensureRedis(ctx); err != nil {
		return "", err
	}
	key := ztAPITokenKeyPrefix + sub
	v, err := redisclient.Client.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(v), nil
}

// saveAPIToken 把 Token 写入 Redis 并设置 TTL。
func saveAPIToken(ctx context.Context, sub, token string) error {
	if err := ensureRedis(ctx); err != nil {
		return err
	}
	key := ztAPITokenKeyPrefix + sub
	return redisclient.Client.Set(ctx, key, token, apiTokenTTL).Err()
}

// deleteAPIToken 清除 Redis 中的 Token 缓存（用于 401 或解绑）。
func deleteAPIToken(ctx context.Context, sub string) {
	if redisclient.Client == nil {
		return
	}
	_ = redisclient.Client.Del(ctx, ztAPITokenKeyPrefix+sub).Err()
}

// loginAndCacheAPIToken 从 PG 读出账号+加密密码 → 调 /api.php/v1/tokens 换 Token → 写 Redis。
// 调用方应在 Token 过期（401）时先删除缓存再重复调用本函数。
func loginAndCacheAPIToken(ctx context.Context, sub string) (string, error) {
	baseURL := strings.TrimSpace(config.Global.ZentaoBaseURL)
	if baseURL == "" {
		return "", fmt.Errorf("zentao base_url not configured")
	}
	account, password, ok, err := db.GetZentaoCredential(sub)
	if err != nil {
		return "", fmt.Errorf("read saved credential: %w", err)
	}
	if !ok {
		return "", fmt.Errorf("no saved zentao credential; please rebind on /zentao-auth")
	}
	cli := zentao.NewAPIClient(baseURL)
	lr, err := cli.APILogin(ctx, account, password)
	if err != nil {
		return "", err
	}
	if err := saveAPIToken(ctx, sub, lr.Token); err != nil {
		return lr.Token, fmt.Errorf("save token to redis: %w", err)
	}
	return lr.Token, nil
}

// ensureAPIToken 优先返回缓存的 Token；未命中则重新登录换 Token。
func ensureAPIToken(ctx context.Context, sub string) (string, error) {
	if t, err := getCachedAPIToken(ctx, sub); err == nil && t != "" {
		return t, nil
	}
	return loginAndCacheAPIToken(ctx, sub)
}
