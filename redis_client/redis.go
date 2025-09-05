package redis_client

import (
	"context"

	"github.com/redis/go-redis/v9"
)

var (
	RDB *redis.Client
	Ctx = context.Background()
)

func init() {
	RDB = redis.NewClient(&redis.Options{
		Addr: "redis:6379",
	})
}
