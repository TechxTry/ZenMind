package zentaoauth

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

const (
	APITokenKeyPrefix = "zentao:api:token:"
	apiTokenTTL       = 10 * time.Hour
)

// GetCachedAPIToken returns the cached API token from Redis; returns "" (not an error) on miss.
func GetCachedAPIToken(ctx context.Context, sub string) (string, error) {
	if err := ensureRedis(ctx); err != nil {
		return "", err
	}
	v, err := redisclient.Client.Get(ctx, APITokenKeyPrefix+sub).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(v), nil
}

// SaveAPIToken stores the token in Redis with a fixed TTL.
func SaveAPIToken(ctx context.Context, sub, token string) error {
	if err := ensureRedis(ctx); err != nil {
		return err
	}
	return redisclient.Client.Set(ctx, APITokenKeyPrefix+sub, token, apiTokenTTL).Err()
}

// DeleteAPIToken removes the cached token (called after 401 or unbind).
func DeleteAPIToken(ctx context.Context, sub string) {
	if redisclient.Client == nil {
		return
	}
	_ = redisclient.Client.Del(ctx, APITokenKeyPrefix+sub).Err()
}

// LoginAndCacheAPIToken fetches credentials from DB, calls /api.php/v1/tokens, and caches the result.
func LoginAndCacheAPIToken(ctx context.Context, sub string) (string, error) {
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
	if err := SaveAPIToken(ctx, sub, lr.Token); err != nil {
		return lr.Token, fmt.Errorf("save token to redis: %w", err)
	}
	return lr.Token, nil
}

// EnsureAPIToken returns the cached token or obtains a new one.
func EnsureAPIToken(ctx context.Context, sub string) (string, error) {
	if t, err := GetCachedAPIToken(ctx, sub); err == nil && t != "" {
		return t, nil
	}
	return LoginAndCacheAPIToken(ctx, sub)
}
