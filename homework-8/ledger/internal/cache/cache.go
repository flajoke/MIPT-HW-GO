package cache

import (
	"context"
	"fmt"
	"github.com/redis/go-redis/v9"
	"os"
	"strconv"
	"time"
)

const TTL = 30 * time.Second

type Cache struct{ client *redis.Client }

func Open(ctx context.Context) (*Cache, error) {
	index := 0
	if value := os.Getenv("REDIS_DB"); value != "" {
		var err error
		index, err = strconv.Atoi(value)
		if err != nil || index < 0 {
			return nil, fmt.Errorf("REDIS_DB должен быть неотрицательным целым числом")
		}
	}
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}
	client := redis.NewClient(&redis.Options{Addr: addr, DB: index, Password: os.Getenv("REDIS_PASSWORD"), DialTimeout: 2 * time.Second, ReadTimeout: time.Second, WriteTimeout: time.Second, MaxRetries: -1})
	if err := client.Ping(ctx).Err(); err != nil {
		client.Close()
		return nil, fmt.Errorf("Redis недоступен; проверьте REDIS_ADDR, REDIS_DB и REDIS_PASSWORD: %w", err)
	}
	return &Cache{client: client}, nil
}
func (c *Cache) Close() error { return c.client.Close() }
func (c *Cache) Get(ctx context.Context, key string) ([]byte, error) {
	return c.client.Get(ctx, key).Bytes()
}
func (c *Cache) Set(ctx context.Context, key string, value []byte) error {
	return c.client.Set(ctx, key, value, TTL).Err()
}
func (c *Cache) Version(ctx context.Context) (string, error) {
	value, err := c.client.Get(ctx, "report:version").Result()
	if err == redis.Nil {
		return "0", nil
	}
	return value, err
}
func (c *Cache) Invalidate(ctx context.Context) error {
	return c.client.Incr(ctx, "report:version").Err()
}
