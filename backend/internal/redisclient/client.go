package redisclient

import (
	"context"
	"time"
	"zenmind/internal/config"

	"github.com/redis/go-redis/v9"
)

var Client *redis.Client

func Init() {
	Client = redis.NewClient(&redis.Options{
		Addr:        config.Global.RedisAddr,
		DialTimeout: 5 * time.Second,
	})
}

func Ping(ctx context.Context) error {
	if Client == nil {
		Init()
	}
	return Client.Ping(ctx).Err()
}
