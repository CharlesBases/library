package redis

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/charlesbases/library/codec"
	"github.com/charlesbases/library/codec/json"
)

var (
	defaultContext        = context.Background()
	defaultTimeout        = 3 * time.Second
	defaultRetries        = 0
	defaultMarshaler      = json.Marshaler
	defaultMutexHeartbeat = 100 * time.Millisecond

	// RandomExpiry 0-24h 随机过期时间
	RandomExpiry = func() time.Duration {
		return time.Duration(rand.Intn(86400)) * time.Second
	}
)

type ClientType string

const (
	RedisClient  ClientType = "client"
	RedisCluster ClientType = "cluster"
)

// Options .
type Options struct {
	Context context.Context
	// Type client or cluster
	Type     ClientType
	Addrs    []string
	Username string
	Password string
	// Timeout second
	Timeout time.Duration
	// MaxRetries 命令执行失败时的重试次数
	MaxRetries int
}

// client .
func (o *Options) client() *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:         o.Addrs[0],
		Username:     o.Username,
		Password:     o.Password,
		MaxRetries:   o.MaxRetries,
		DialTimeout:  o.Timeout,
		ReadTimeout:  o.Timeout,
		WriteTimeout: o.Timeout,
		// PoolSize:     4 * runtime.NumCPU(), // 连接池最大 socket 连接数。(default: 4 * runtime.NumCPU())
		// MinIdleConns: 16,                   // 空闲连接数
		// TLSConfig: &tls.Config{
		// 	InsecureSkipVerify: true,
		// },
	})
}

// cluster .
func (o *Options) cluster() *redis.ClusterClient {
	return redis.NewClusterClient(&redis.ClusterOptions{
		Addrs:        o.Addrs,
		Username:     o.Username,
		Password:     o.Password,
		MaxRetries:   o.MaxRetries,
		DialTimeout:  o.Timeout,
		ReadTimeout:  o.Timeout,
		WriteTimeout: o.Timeout,
		// PoolSize:     4 * runtime.NumCPU(), // 连接池最大 socket 连接数。(default: 4 * runtime.NumCPU())
		// MinIdleConns: 16, // 空闲连接数
		// TLSConfig: &tls.Config{
		// 	InsecureSkipVerify: true,
		// },
	})
}

// newc .
func (o *Options) newc() (*rkv, error) {
	redis.SetLogger(new(disprint))

	var r *rkv
	switch o.Type {
	case RedisClient:
		client := o.client()
		r = &rkv{opts: o, client: client, closing: client.Close}
	case RedisCluster:
		client := o.cluster()
		r = &rkv{opts: o, client: client, closing: client.Close}
	default:
		return nil, fmt.Errorf(`[redis] unsupported type of %s`, o.Type)
	}

	if err := r.ping(); err != nil {
		return nil, err
	}
	return r, nil
}

// parseoptions .
func parseoptions(opts ...func(o *Options)) *Options {
	o := &Options{
		Type:       RedisClient,
		Addrs:      []string{"0.0.0.0:6379"},
		Context:    defaultContext,
		Timeout:    defaultTimeout,
		MaxRetries: defaultRetries,
	}
	for _, opt := range opts {
		opt(o)
	}
	return o
}

// SetOptions .
type SetOptions struct {
	Context context.Context
	// TTL key 过期时间
	TTL time.Duration
	// Expiry key 过期时间。TTL 和 Expiry 同时设置时，以 Expiry 为准
	Expiry time.Time
	// Marshaler value 编码方式
	Marshaler codec.Marshaler
}

// parsesetoptions .
func parsesetoptions(opts ...func(o *SetOptions)) *SetOptions {
	o := &SetOptions{
		Context:   defaultContext,
		Marshaler: defaultMarshaler,
	}
	for _, opt := range opts {
		opt(o)
	}
	return o
}

// GetOptions .
type GetOptions struct {
	Context context.Context
	// Marshaler value 编码方式
	Marshaler codec.Marshaler
}

// parsegetoptions .
func parsegetoptions(opts ...func(o *GetOptions)) *GetOptions {
	o := &GetOptions{
		Context:   defaultContext,
		Marshaler: defaultMarshaler,
	}
	for _, opt := range opts {
		opt(o)
	}
	return o
}

// DelOptions .
type DelOptions struct {
	Context context.Context
}

// parsedeloptions .
func parsedeloptions(opts ...func(o *DelOptions)) *DelOptions {
	o := &DelOptions{
		Context: defaultContext,
	}
	for _, opt := range opts {
		opt(o)
	}
	return o
}

// MutexOptions .
type MutexOptions struct {
	Context context.Context
	// Mark 分布式锁标记
	Mark string
	// Heartbeat 尝试获取锁的时间间隔
	Heartbeat time.Duration
	// TTL 超时时间
	TTL time.Duration
}

// parsemutexoptions .
func parsemutexoptions(opts ...func(o *MutexOptions)) *MutexOptions {
	o := &MutexOptions{
		Context:   defaultContext,
		Heartbeat: defaultMutexHeartbeat,
		TTL:       defaultTimeout,
	}
	for _, opt := range opts {
		opt(o)
	}
	return o
}

// ExpireOptions .
type ExpireOptions struct {
	Context context.Context
	// TTL key 过期时间
	TTL time.Duration
	// Expiry key 过期时间。TTL 和 Expiry 同时设置时，以 Expiry 为准
	Expiry time.Time
}

// parseexpireoptions .
func parseexpireoptions(opts ...func(o *ExpireOptions)) *ExpireOptions {
	o := &ExpireOptions{
		Context: defaultContext,
	}
	for _, opt := range opts {
		opt(o)
	}
	return o
}