package utils

import (
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/spf13/viper"
)

// NewRedisClient 创建Redis客户端
func NewRedisClient(config *viper.Viper) *redis.Client {
	if config == nil {
		return redis.NewClient(&redis.Options{
			Addr: "localhost:6379",
		})
	}

	return redis.NewClient(&redis.Options{
		Addr:         config.GetString("host") + ":" + config.GetString("port"),
		Password:     config.GetString("password"),
		DB:           config.GetInt("db"),
		PoolSize:     config.GetInt("pool_size"),
		MinIdleConns: config.GetInt("min_idle_conns"),
		DialTimeout:  config.GetDuration("dial_timeout"),
		ReadTimeout:  config.GetDuration("read_timeout"),
		WriteTimeout: config.GetDuration("write_timeout"),
	})
}

// RedisLock Redis分布式锁
type RedisLock struct {
	client    *redis.Client
	key       string
	value     string
	expire    time.Duration
	acquired  bool
}

// NewRedisLock 创建分布式锁
func NewRedisLock(client *redis.Client, key string, expire time.Duration) *RedisLock {
	return &RedisLock{
		client: client,
		key:    key,
		expire: expire,
	}
}

// TryLock 尝试获取锁
func (l *RedisLock) TryLock() (bool, error) {
	// 使用SET NX命令
	result, err := l.client.SetNX(nil, l.key, "1", l.expire).Result()
	if err != nil {
		return false, err
	}
	if result {
		l.acquired = true
	}
	return result, nil
}

// Unlock 释放锁
func (l *RedisLock) Unlock() error {
	if !l.acquired {
		return nil
	}
	_, err := l.client.Del(nil, l.key).Result()
	if err == nil {
		l.acquired = false
	}
	return err
}

// WithLock 执行带锁的操作
func WithLock(client *redis.Client, key string, expire time.Duration, fn func() error) error {
	lock := NewRedisLock(client, key, expire)

	locked, err := lock.TryLock()
	if err != nil {
		return err
	}
	if !locked {
		return nil
	}

	defer lock.Unlock()
	return fn()
}
