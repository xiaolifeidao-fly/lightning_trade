package initialization

import (
	"common/middleware/db"
	"common/middleware/redis"
	"common/middleware/vipper"
	"fmt"
	"log"
	"manager-api/routers"
	"time"
)

// InitOrder 定义初始化顺序
type InitOrder int

const (
	ConfigInit InitOrder = iota
	DBInit
	RedisInit
	OssInit
	IpManagerInit
	IpManagerV2Init    // 新增V2版本初始化
	IpManagerShortInit // 新增Short版本初始化
	DeviceManagerInit
	RouterInit
	ActivationCodeConsumerInit
)

type Initializer struct {
	Order  InitOrder
	Name   string
	InitFn func() error
}

var initializers = []Initializer{
	{
		Order: ConfigInit,
		Name:  "Config",
		InitFn: func() error {
			vipper.Init()
			return nil
		},
	},
	{
		Order: DBInit,
		Name:  "Database",
		InitFn: func() error {
			db.InitDB()
			return nil
		},
	},
	{
		Order: RedisInit,
		Name:  "Redis",
		InitFn: func() error {
			redisAddr := vipper.GetString("redis.addr")
			redisPwd := vipper.GetString("redis.password")
			return redis.InitRedisClient(redisAddr, redisPwd)
		},
	},

	// {
	// 	Order: OssInit,
	// 	Name:  "Router",
	// 	InitFn: func() error {
	// 		dirPrefix := vipper.GetString("oss.dirPrefix")
	// 		bucketName := vipper.GetString("oss.bucketName")
	// 		accessKeyId := vipper.GetString("oss.accessKeyId")
	// 		accessKeySecret := vipper.GetString("oss.accessKeySecret")
	// 		endpoint := vipper.GetString("oss.endpoint")
	// 		ossEntity := &oss.OssEntity{
	// 			DirPrefix:       dirPrefix,
	// 			Endpoint:        endpoint,
	// 			BucketName:      bucketName,
	// 			AccessKeyId:     accessKeyId,
	// 			AccessKeySecret: accessKeySecret,
	// 		}
	// 		oss.Setup(ossEntity)
	// 		return nil
	// 	},
	// },
	// {
	// 	Order:  IpManagerInit,
	// 	Name:   "IP Manager",
	// 	InitFn: ipBiz.InitIpManager,
	// },
	// {
	// 	Order:  IpManagerV2Init,
	// 	Name:   "IP Manager V2",
	// 	InitFn: ipBiz.InitIpManagerV2,
	// },
	// {
	// 	Order:  IpManagerShortInit,
	// 	Name:   "IP Manager Short",
	// 	InitFn: ipBiz.InitIpManagerShort,
	// },
	{
		Order: RouterInit,
		Name:  "Router",
		InitFn: func() error {
			routers.Init()
			return nil
		},
	},
}

// Init 统一初始化入口
func Init() error {
	// 按顺序执行初始化
	for _, init := range initializers {
		log.Printf("Initializing %s...", init.Name)
		start := time.Now()
		if err := init.InitFn(); err != nil {
			switch init.Order {
			case RedisInit, OssInit:
				log.Printf("Skipping optional initializer %s after %s: %v", init.Name, time.Since(start), err)
				continue
			default:
				return fmt.Errorf("failed to initialize %s after %s: %v", init.Name, time.Since(start), err)
			}
		}
		log.Printf("%s initialized successfully in %s", init.Name, time.Since(start))
	}
	return nil
}
