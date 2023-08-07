package server

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charlesbases/logger"
	"github.com/charlesbases/logger/filewriter"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/charlesbases/library/broker"
	"github.com/charlesbases/library/codec/yaml"
	"github.com/charlesbases/library/database"
	"github.com/charlesbases/library/database/orm"
	"github.com/charlesbases/library/database/orm/driver"
	"github.com/charlesbases/library/jwtauth"
	"github.com/charlesbases/library/lifecycle"
	"github.com/charlesbases/library/redis"
	"github.com/charlesbases/library/server/middlewares"
	"github.com/charlesbases/library/server/middlewares/jwt"
	"github.com/charlesbases/library/server/websocket"
	"github.com/charlesbases/library/storage"
	"github.com/charlesbases/library/storage/s3"
)

var conf = new(configuration)

// configuration .
type configuration struct {
	// Name server name
	Name string `yaml:"name"`
	// Port http port
	Port string `yaml:"port" default:":8080"`
	// Spec spec
	Spec spec `yaml:"spec"`
	// Data 服务自定义配置
	Data interface{} `yaml:"data"`
}

// spec .
type spec struct {
	// JWT jwt
	JWT specJwtAuth `yaml:"jwt"`
	// Logging logging
	Logging logging `yaml:"logging"`
	// Metrics metrics
	Metrics metrics `yaml:"metrics"`
	// WebSocket websocket
	WebSocket ws `yaml:"websocket"`
	// Plugins plugins
	Plugins plugins `yaml:"plugins"`
}

// plugins .
type plugins struct {
	// Redis redis
	Redis serverRedis `yaml:"redis"`
	// Broker broker
	Broker serverBroker `yaml:"broker"`
	// Storage storage
	Storage serverStorage `yaml:"storage"`
	// Database database
	Database serverDatabase `yaml:"database"`
}

// logging .
type logging struct {
	OutputPath string `yaml:"outputPath"`
	MaxRolls   int    `yaml:"maxRolls"`
	Minlevel   string `yaml:"minlevel"`
}

// metrics .
type metrics struct {
	// Enabled enabled
	Enabled bool `yaml:"enabled"`
}

// ws .
type ws struct {
	// Enabled enabled
	Enabled bool `yaml:"enabled"`
	// EnSubscription 是否启用消息订阅
	EnSubscription bool `yaml:"enSubscription"`
}

// specJwtAuth .
type specJwtAuth struct {
	// Enabled enabled
	Enabled bool `yaml:"enabled"`
	// Secret jwt secret
	Secret string `yaml:"secret"`
	// Expire token 过期时间。单位：秒
	Expire int `yaml:"expire"`
	// Interceptor jwt 拦截器
	Interceptor *jwt.Interceptor `yaml:"intercept"`
}

// serverRedis .
type serverRedis struct {
	// Enabled enabled
	Enabled bool `yaml:"enabled"`
	// Type client or cluster
	Type redis.ClientType `yaml:"type"`
	// Address address for redis
	Address []string `yaml:"address"`
	// Username username
	Username string `yaml:"username"`
	// Password password
	Password string `yaml:"password"`
	// Timeout timeout
	Timeout int `yaml:"timeout"`
	// MaxRetries 命令执行失败时的最大重试次数
	MaxRetries int `yaml:"maxRetries"`
}

// serverBroker .
type serverBroker struct {
	// Enabled enabled
	Enabled bool `yaml:"enabled"`
	// Type type of broker
	Type broker.Type `yaml:"type"`
	// Address address
	Address string `yaml:"address"`
}

// serverStorage .
type serverStorage struct {
	// Enabled enabled
	Enabled bool `yaml:"enabled"`
	// Type storage.Type
	Type string `yaml:"type"`
	// Address address
	Address string `yaml:"address"`
	// AccessKey accesskey
	AccessKey string `yaml:"accessKey"`
	// SecretKey secretkey
	SecretKey string `yaml:"secretKey"`
	// Timeout timeout
	Timeout int `yaml:"timeout"`
	// UseSSL usessl
	UseSSL bool `yaml:"useSsl"`
}

// serverDatabase .
type serverDatabase struct {
	// Enabled enabled
	Enabled bool `yaml:"enabled"`
	// Type database.Driver
	Type string `yaml:"type"`
	// Dsn database dsn
	Dsn string `yaml:"dsn"`
	// MaxOpenConns 最大连接数
	MaxOpenConns int `yaml:"maxOpenConns" default:"0"`
	// MaxIdleConns 连接池中最大空闲数
	MaxIdleConns int `yaml:"maxIdleConns" default:"4"`
}

// parseconf use default config file of './config.yaml'
func parseconf() *configuration {
	if err := yaml.NewDecoder().Decode(conf); err != nil {
		logger.Fatal(err)
	}
	return conf
}

// server .
func (c *configuration) server() *Server {
	srv := &Server{
		name:      c.Name,
		uuid:      uuid.New(),
		ctx:       context.Background(),
		lifecycle: new(lifecycle.Lifecycle)}
	srv.id = strings.Join([]string{srv.name, srv.uuid.String()}, ".")

	// gin.Engine
	c.initEngine(srv)

	// redis
	c.initRedis(srv)

	// broker
	c.initBroker(srv)

	// storage
	c.initStorage(srv)

	// database
	c.initDatabase(srv)

	return srv
}

// initEngine .
func (c *configuration) initEngine(srv *Server) {
	gin.SetMode(gin.ReleaseMode)

	srv.engine = gin.New()
	srv.engine.Use(middlewares.Cors())
	srv.engine.Use(middlewares.Negroni())
	srv.engine.Use(middlewares.Recovery())

	// logging
	logger.SetDefault(func(o *logger.Options) {
		conf := c.Spec.Logging
		o.MinLevel = conf.Minlevel
		o.Writer = filewriter.New(filewriter.OutputPath(conf.OutputPath), filewriter.MaxRolls(conf.MaxRolls))
	})

	// jwt
	if c.Spec.JWT.Enabled {
		// init jwt
		jwtauth.Set(c.Spec.JWT.Secret, jwtauth.Expire(c.Spec.JWT.Expire))
		// use middlewares
		srv.engine.Use(jwt.New(func(j *jwt.JwtHandler) { j.Interceptor = c.Spec.JWT.Interceptor }).HandlerFunc())
	}

	// metrics
	if c.Spec.Metrics.Enabled {
		srv.engine.GET("/metrics", gin.WrapH(promhttp.Handler()))
	}
}

// initRedis .
func (c *configuration) initRedis(srv *Server) {
	if c.Spec.Plugins.Redis.Enabled {
		conf := c.Spec.Plugins.Redis

		srv.lifecycle.Append(
			&lifecycle.Hook{
				Name: "redis",
				OnStart: func(ctx context.Context) error {
					return redis.Init(func(o *redis.Options) {
						o.Type = conf.Type
						o.Addrs = conf.Address
						o.Username = conf.Username
						o.Password = conf.Password
						o.Timeout = time.Duration(conf.Timeout) * time.Second
						o.MaxRetries = conf.MaxRetries
					})
				},
				OnStop: func(ctx context.Context) error {
					return redis.Close()
				},
			})
	}
}

// initBroker TODO
func (c *configuration) initBroker(srv *Server) {
	if c.Spec.Plugins.Broker.Enabled {
		conf := c.Spec.Plugins.Broker

		// broker
		srv.lifecycle.Append(
			&lifecycle.Hook{
				Name:    conf.Type.String(),
				OnStart: nil,
				OnStop:  nil,
			})

		// websocket
		if c.Spec.WebSocket.Enabled && c.Spec.WebSocket.EnSubscription {
			srv.lifecycle.Append(&lifecycle.Hook{
				Name: "websocket.staion",
				OnStart: func(ctx context.Context) error {
					return websocket.InitStation(srv.broker)
				},
			})
		}
	}
}

// initStorage .
func (c *configuration) initStorage(srv *Server) {
	if c.Spec.Plugins.Storage.Enabled {
		conf := c.Spec.Plugins.Storage

		srv.lifecycle.Append(
			&lifecycle.Hook{
				Name: conf.Type,
				OnStart: func(ctx context.Context) error {
					switch conf.Type {
					case "s3":
						client, err := s3.NewClient(conf.Address, conf.AccessKey, conf.SecretKey, func(o *storage.Options) {
							o.Timeout = time.Duration(conf.Timeout) * time.Second
							o.UseSSL = conf.UseSSL
						})
						if err != nil {
							return err
						}
						srv.storage = client
						return nil
					default:
						return fmt.Errorf(`load configuration failed: unsupported values of 'spec.plugins.storage.type: "%s"'`, conf.Type)
					}
				},
			})
	}
}

// initDatabase .
func (c *configuration) initDatabase(srv *Server) {
	if c.Spec.Plugins.Database.Enabled {
		conf := c.Spec.Plugins.Database

		var dr driver.Driver
		switch conf.Type {
		case "mysql":
			dr = new(driver.Mysql)
		case "postgres":
			dr = new(driver.Postgres)
		default:
			logger.Fatalf(`load configuration failed: unsupported values of 'spec.plugins.database.type: "%s"'`, conf.Type)
		}

		srv.lifecycle.Append(
			&lifecycle.Hook{
				Name: dr.Type(),
				OnStart: func(ctx context.Context) error {
					return orm.Init(dr, func(o *database.Options) {
						o.Address = conf.Dsn
						o.MaxOpenConns = conf.MaxOpenConns
						o.MaxIdleConns = conf.MaxIdleConns
					})
				},
			})
	}
}