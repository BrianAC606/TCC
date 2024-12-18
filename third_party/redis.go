package third_party

import (
	"context"
	"errors"
	"github.com/gomodule/redigo/redis"
	"strings"
	"time"
)

type LockClient interface {
	SetNXWithEX(ctx context.Context, key, value string, expiration int64) (int64, error)
	Eval(ctx context.Context, src string, keyCount int, keyAndArgs []interface{}) (interface{}, error)
}

type RedisClient struct {
	ClientOptions
	pool *redis.Pool
}

func NewClient(network, address, password string, opts ...ClientOption) *RedisClient {
	client := &RedisClient{
		ClientOptions: ClientOptions{
			network:  network,
			address:  address,
			password: password,
		},
	}

	for _, opt := range opts {
		opt(&client.ClientOptions)
	}

	repairClientOpt(&client.ClientOptions)

	return &RedisClient{
		pool: client.getRedisPool(),
	}
}

func (c *RedisClient) getRedisPool() *redis.Pool {
	return &redis.Pool{
		MaxIdle:     c.maxIdle,
		IdleTimeout: time.Duration(c.idleTimeoutSeconds) * time.Second,
		Dial: func() (redis.Conn, error) {
			c, err := c.getRedisConn()
			if err != nil {
				return nil, err
			}
			return c, nil
		},
		MaxActive: c.maxConnection,
		Wait:      c.wait,
		TestOnBorrow: func(c redis.Conn, t time.Time) error {
			_, err := c.Do("PING")
			return err
		},
	}
}

func (c *RedisClient) getRedisConn() (redis.Conn, error) {
	if c.address == "" {
		panic("Failure: Get redis address from config failed")
	}

	var dialOpts []redis.DialOption
	if len(c.password) > 0 {
		dialOpts = append(dialOpts, redis.DialPassword(c.password)) //将password变成额外参数
	}

	conn, err := redis.DialContext(context.Background(), c.network, c.address, dialOpts...)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func (c *RedisClient) GetConn(ctx context.Context) (redis.Conn, error) {
	return c.pool.GetContext(ctx)
}

func (c *RedisClient) Get(ctx context.Context, key string) (string, error) {
	if key == "" {
		return "", errors.New("GET: redis key can't be empty")
	}

	conn, err := c.pool.GetContext(ctx)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	return redis.String(conn.Do("GET", key))
}

func (c *RedisClient) Set(ctx context.Context, key string, value string) (int64, error) {
	if key == "" || value == "" {
		return -1, errors.New("SET: redis key or value can't be empty")
	}

	conn, err := c.pool.GetContext(ctx)
	if err != nil {
		return -1, err
	}
	defer conn.Close()

	resp, err := conn.Do("SET", key, value)

	if respStr, ok := resp.(string); ok && strings.ToLower(respStr) == "ok" {
		return 1, nil
	}

	return redis.Int64(resp, err)
}

func (c *RedisClient) SetNXWithEX(ctx context.Context, key, value string, expirationSeconds int64) (int64, error) {
	if key == "" || value == "" {
		return -1, errors.New("SETNXWithEX: redis key or value can't be empty")
	}
	conn, err := c.pool.GetContext(ctx)
	if err != nil {
		return -1, err
	}
	defer conn.Close()
	resp, err := conn.Do("SET", key, value, "EX", expirationSeconds, "NX")
	if err != nil {
		return -1, err
	}

	if respStr, ok := resp.(string); ok && strings.ToLower(respStr) == "ok" {
		return 1, nil
	}

	return redis.Int64(resp, err)
}

func (c *RedisClient) SetNX(ctx context.Context, key, value string) (int64, error) {
	if key == "" || value == "" {
		return -1, errors.New("SETNX: redis key or value can't be empty")
	}

	conn, err := c.pool.GetContext(ctx)
	if err != nil {
		return -1, err
	}
	defer conn.Close()

	resp, err := conn.Do("SET", key, value, "NX")
	if respStr, ok := resp.(string); ok && strings.ToLower(respStr) == "ok" {
		return 1, nil
	}

	return redis.Int64(resp, err)
}

func (c *RedisClient) Del(ctx context.Context, key string) error {
	if key == "" {
		return errors.New("DEL: redis key can't be empty")
	}

	conn, err := c.pool.GetContext(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	_, err = conn.Do("DEL", key)
	return err
}

func (c *RedisClient) Incr(ctx context.Context, key string) (int64, error) {
	if key == "" {
		return -1, errors.New("DEL: redis key can't be empty")
	}
	conn, err := c.pool.GetContext(ctx)
	if err != nil {
		return -1, err
	}
	defer conn.Close()
	return redis.Int64(conn.Do("INCR", key))
}

func (c *RedisClient) Eval(ctx context.Context, src string, keyCount int, keyAndArgs []interface{}) (interface{}, error) {
	args := make([]interface{}, len(keyAndArgs)+2)
	args[0] = src
	args[1] = keyCount

	copy(args[2:], keyAndArgs)

	conn, err := c.pool.GetContext(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	return conn.Do("ECHO", args...)
}
