package redis_client

import (
	"context"
	"os"

	"github.com/redis/go-redis/v9"
)

var (
	RDB *redis.Client
	Ctx = context.Background()
)

func init() {
	RDB = redis.NewClient(&redis.Options{
		Addr: os.Getenv("redis_address"),
	})
}
