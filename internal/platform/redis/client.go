package redis

import (
	"context"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// Config contains redis client settings.
type Config struct {
	Addr                string
	Password            string
	DB                  int
	PoolSize            int
	MinIdleConns        int
	DialTimeoutSeconds  int
	ReadTimeoutSeconds  int
	WriteTimeoutSeconds int
	PingTimeoutSeconds  int
}

// Client wraps a go-redis client.
type Client struct {
	client *goredis.Client
}

// New creates a redis client and verifies connectivity by ping.
func New(cfg Config) (*Client, error) {
	if cfg.Addr == "" {
		return nil, fmt.Errorf("redis address is empty")
	}

	options := &goredis.Options{
		Addr:         cfg.Addr,
		Password:     cfg.Password,
		DB:           cfg.DB,
		PoolSize:     cfg.PoolSize,
		MinIdleConns: cfg.MinIdleConns,
	}

	if cfg.DialTimeoutSeconds > 0 {
		options.DialTimeout = time.Duration(cfg.DialTimeoutSeconds) * time.Second
	}
	if cfg.ReadTimeoutSeconds > 0 {
		options.ReadTimeout = time.Duration(cfg.ReadTimeoutSeconds) * time.Second
	}
	if cfg.WriteTimeoutSeconds > 0 {
		options.WriteTimeout = time.Duration(cfg.WriteTimeoutSeconds) * time.Second
	}

	redisClient := goredis.NewClient(options)
	client := &Client{client: redisClient}

	pingTimeout := 5 * time.Second
	if cfg.PingTimeoutSeconds > 0 {
		pingTimeout = time.Duration(cfg.PingTimeoutSeconds) * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), pingTimeout)
	defer cancel()
	if err := client.Ping(ctx); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("ping redis %s failed: %w", cfg.Addr, err)
	}

	return client, nil
}

func (c *Client) Ping(ctx context.Context) error {
	if c == nil || c.client == nil {
		return fmt.Errorf("redis client is nil")
	}
	return c.client.Ping(ctx).Err()
}

func (c *Client) Client() *goredis.Client {
	if c == nil {
		return nil
	}
	return c.client
}

func (c *Client) Close() error {
	if c == nil || c.client == nil {
		return nil
	}
	return c.client.Close()
}
