# Go 高可用设计

Go 服务高可用架构模式与实践，涵盖优雅关闭、熔断降级、限流、重试、超时传播、分布式锁、健康探针与可观测性。

## 优雅关闭 (Graceful Shutdown)

服务停止时等待在途请求处理完毕，避免请求丢失。

```go
func main() {
    router := gin.Default()
    registerRoutes(router)

    srv := &http.Server{Addr: ":8080", Handler: router}

    go func() {
        if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            log.Fatalf("listen: %v", err)
        }
    }()

    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
    <-quit

    log.Println("shutting down, waiting for in-flight requests...")
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    if err := srv.Shutdown(ctx); err != nil {
        log.Fatalf("forced shutdown: %v", err)
    }
    log.Println("server exited")
}
```

**要点：**
- 使用 `signal.Notify` 捕获 SIGINT/SIGTERM
- `Shutdown` 会停止接收新连接，等待活跃连接处理完毕
- 设置超时上限（通常 10-30s），防止无限等待
- 关闭前先注销服务注册（如 Consul/etcd）

## Panic Recovery

防止单个请求 panic 导致整个进程崩溃。

```go
func Recovery() gin.HandlerFunc {
    return func(c *gin.Context) {
        defer func() {
            if r := recover(); r != nil {
                log.Printf("panic recovered: %v\n%s", r, debug.Stack())
                c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
                    "code":  500,
                    "error": "internal server error",
                })
            }
        }()
        c.Next()
    }
}

func main() {
    r := gin.New()
    r.Use(Recovery(), gin.Logger())
}
```

**Gin 内置方案：** 直接使用 `gin.Recovery()` 或 `gin.CustomRecovery(handler)`。

## 超时控制与 Context 传播

### 请求超时中间件

```go
func TimeoutMiddleware(timeout time.Duration) gin.HandlerFunc {
    return func(c *gin.Context) {
        ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
        defer cancel()
        c.Request = c.Request.WithContext(ctx)
        c.Next()
    }
}

r.Use(TimeoutMiddleware(5 * time.Second))
```

### 下游调用传递 Context

```go
func CallDownstream(ctx context.Context, url string) ([]byte, error) {
    req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
    if err != nil {
        return nil, err
    }
    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    return io.ReadAll(resp.Body)
}
```

**原则：** 所有 I/O 操作（DB、Redis、HTTP、RPC）都应传入 `context.Context`，确保超时可以逐层传播取消。

## 重试与退避 (Retry with Backoff)

### 指数退避重试

```go
func RetryWithBackoff(ctx context.Context, maxRetries int, fn func() error) error {
    var err error
    for i := 0; i < maxRetries; i++ {
        if err = fn(); err == nil {
            return nil
        }
        if ctx.Err() != nil {
            return ctx.Err()
        }
        backoff := time.Duration(1<<uint(i)) * 100 * time.Millisecond
        jitter := time.Duration(rand.Int63n(int64(backoff / 2)))
        time.Sleep(backoff + jitter)
    }
    return fmt.Errorf("max retries exceeded: %w", err)
}
```

### 使用示例

```go
err := RetryWithBackoff(ctx, 3, func() error {
    return callExternalAPI(ctx, payload)
})
```

**适用场景：** 网络抖动、临时不可用、数据库锁冲突
**不适合重试：** 参数错误 (4xx)、鉴权失败、幂等性无法保证的写操作

## 熔断器 (Circuit Breaker)

防止雪崩效应，当下游服务异常率超过阈值时自动熔断。

### 使用 sony/gobreaker

```go
import "github.com/sony/gobreaker/v2"

var cb = gobreaker.NewCircuitBreaker[[]byte](gobreaker.Settings{
    Name:        "downstream-api",
    MaxRequests: 3,                // Half-Open 态最多放行 3 个请求
    Interval:    10 * time.Second, // Closed 态统计窗口
    Timeout:     30 * time.Second, // Open → Half-Open 等待时间
    ReadyToTrip: func(counts gobreaker.Counts) bool {
        return counts.ConsecutiveFailures >= 5
    },
    OnStateChange: func(name string, from, to gobreaker.State) {
        log.Printf("circuit breaker [%s]: %s → %s", name, from, to)
    },
})

func CallWithBreaker(ctx context.Context, url string) ([]byte, error) {
    body, err := cb.Execute(func() ([]byte, error) {
        return CallDownstream(ctx, url)
    })
    if err != nil {
        return nil, fmt.Errorf("breaker: %w", err)
    }
    return body, nil
}
```

**状态流转：** Closed（正常） → Open（熔断） → Half-Open（试探） → Closed

**配置建议：**
- `ConsecutiveFailures` 根据 QPS 调整，高 QPS 服务可设 5-10
- `Timeout` 建议 15-60s
- 搭配降级策略（返回缓存、默认值、友好错误）

## 限流 (Rate Limiting)

### 令牌桶 - 单机限流

```go
import "golang.org/x/time/rate"

var limiter = rate.NewLimiter(rate.Limit(100), 200) // 100 QPS, burst 200

func RateLimitMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        if !limiter.Allow() {
            c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
                "code":  429,
                "error": "rate limit exceeded",
            })
            return
        }
        c.Next()
    }
}
```

### 分布式限流 - Redis 滑动窗口

```go
func RateLimitByIP(ctx context.Context, rdb *redis.Client, ip string, limit int, window time.Duration) bool {
    key := fmt.Sprintf("rate:%s", ip)
    now := time.Now().UnixMilli()
    pipe := rdb.Pipeline()
    pipe.ZRemRangeByScore(ctx, key, "0", fmt.Sprintf("%d", now-window.Milliseconds()))
    pipe.ZAdd(ctx, key, &redis.Z{Score: float64(now), Member: now})
    pipe.ZCard(ctx, key)
    pipe.Expire(ctx, key, window)
    cmds, err := pipe.Exec(ctx)
    if err != nil {
        return true // 降级放行
    }
    count := cmds[2].(*redis.IntCmd).Val()
    return count <= int64(limit)
}
```

## 分布式锁

### Redis 分布式锁

```go
type RedisLock struct {
    client *redis.Client
    key    string
    value  string
    ttl    time.Duration
}

func NewRedisLock(client *redis.Client, key string, ttl time.Duration) *RedisLock {
    return &RedisLock{
        client: client,
        key:    key,
        value:  uuid.New().String(),
        ttl:    ttl,
    }
}

func (l *RedisLock) Lock(ctx context.Context) (bool, error) {
    return l.client.SetNX(ctx, l.key, l.value, l.ttl).Result()
}

const unlockLua = `if redis.call("GET",KEYS[1])==ARGV[1] then return redis.call("DEL",KEYS[1]) else return 0 end`

func (l *RedisLock) Unlock(ctx context.Context) error {
    res, err := l.client.Eval(ctx, unlockLua, []string{l.key}, l.value).Int()
    if err != nil {
        return err
    }
    if res == 0 {
        return fmt.Errorf("lock not held")
    }
    return nil
}
```

### 使用示例

```go
lock := NewRedisLock(rdb, "order:create:"+orderID, 10*time.Second)
ok, err := lock.Lock(ctx)
if err != nil || !ok {
    return ErrAcquireLock
}
defer lock.Unlock(ctx)

// 执行互斥业务逻辑
```

**注意事项：**
- 锁值使用 UUID，防止误解别人的锁
- 释放锁用 Lua 脚本保证原子性
- 设置合理 TTL，防止死锁
- 高可靠场景考虑 Redlock 或 etcd

## 连接池管理

### 数据库连接池

```go
sqlDB, _ := db.DB()
sqlDB.SetMaxOpenConns(100)              // 最大打开连接数
sqlDB.SetMaxIdleConns(20)               // 最大空闲连接数
sqlDB.SetConnMaxLifetime(30 * time.Minute) // 连接最大存活时间
sqlDB.SetConnMaxIdleTime(5 * time.Minute)  // 空闲连接最大存活时间
```

### HTTP 客户端连接池

```go
var httpClient = &http.Client{
    Timeout: 10 * time.Second,
    Transport: &http.Transport{
        MaxIdleConns:        100,
        MaxIdleConnsPerHost: 20,
        IdleConnTimeout:     90 * time.Second,
        TLSHandshakeTimeout: 5 * time.Second,
    },
}
```

### Redis 连接池

```go
rdb := redis.NewClient(&redis.Options{
    Addr:         "localhost:6379",
    PoolSize:     50,
    MinIdleConns: 10,
    DialTimeout:  3 * time.Second,
    ReadTimeout:  2 * time.Second,
    WriteTimeout: 2 * time.Second,
    PoolTimeout:  4 * time.Second,
})
```

**原则：** 所有外部连接都应使用连接池，避免频繁创建/销毁连接的开销。

## 健康探针 (Health Probes)

### Kubernetes Liveness + Readiness

```go
func LivenessHandler() gin.HandlerFunc {
    return func(c *gin.Context) {
        c.JSON(http.StatusOK, gin.H{"status": "alive"})
    }
}

func ReadinessHandler(db *gorm.DB, rdb *redis.Client) gin.HandlerFunc {
    return func(c *gin.Context) {
        result := gin.H{"status": "ready"}
        code := http.StatusOK

        sqlDB, err := db.DB()
        if err != nil || sqlDB.PingContext(c.Request.Context()) != nil {
            result["database"] = "not ready"
            code = http.StatusServiceUnavailable
        }
        if rdb.Ping(c.Request.Context()).Err() != nil {
            result["redis"] = "not ready"
            code = http.StatusServiceUnavailable
        }

        result["status"] = map[bool]string{true: "ready", false: "not ready"}[code == 200]
        c.JSON(code, result)
    }
}

r.GET("/healthz", LivenessHandler())
r.GET("/readyz", ReadinessHandler(db, rdb))
```

**Kubernetes 配置：**
```yaml
livenessProbe:
  httpGet:
    path: /healthz
    port: 8080
  initialDelaySeconds: 5
  periodSeconds: 10
readinessProbe:
  httpGet:
    path: /readyz
    port: 8080
  initialDelaySeconds: 5
  periodSeconds: 5
```

## 可观测性 (Observability)

### Prometheus 指标采集

```go
import "github.com/prometheus/client_golang/prometheus"

var (
    httpRequestsTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "http_requests_total",
            Help: "Total HTTP requests",
        },
        []string{"method", "path", "status"},
    )
    httpRequestDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "http_request_duration_seconds",
            Help:    "HTTP request latency",
            Buckets: prometheus.DefBuckets,
        },
        []string{"method", "path"},
    )
)

func init() {
    prometheus.MustRegister(httpRequestsTotal, httpRequestDuration)
}

func MetricsMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        start := time.Now()
        c.Next()
        duration := time.Since(start).Seconds()

        httpRequestsTotal.WithLabelValues(c.Request.Method, c.FullPath(), fmt.Sprintf("%d", c.Writer.Status())).Inc()
        httpRequestDuration.WithLabelValues(c.Request.Method, c.FullPath()).Observe(duration)
    }
}
```

### 结构化日志

```go
import "github.com/sirupsen/logrus"

func InitLogger() {
    logrus.SetFormatter(&logrus.JSONFormatter{
        TimestampFormat: time.RFC3339Nano,
    })
    logrus.SetLevel(logrus.InfoLevel)
}

func RequestLogger() gin.HandlerFunc {
    return func(c *gin.Context) {
        start := time.Now()
        c.Next()

        logrus.WithFields(logrus.Fields{
            "method":   c.Request.Method,
            "path":     c.Request.URL.Path,
            "status":   c.Writer.Status(),
            "latency":  time.Since(start).String(),
            "client_ip": c.ClientIP(),
        }).Info("request completed")
    }
}
```

## 并发模式

### errgroup 并行调用

```go
import "golang.org/x/sync/errgroup"

func FetchDashboard(ctx context.Context) (*Dashboard, error) {
    var (
        user   *User
        orders []Order
        stats  *Stats
    )

    g, ctx := errgroup.WithContext(ctx)

    g.Go(func() error {
        var err error
        user, err = fetchUser(ctx)
        return err
    })
    g.Go(func() error {
        var err error
        orders, err = fetchOrders(ctx)
        return err
    })
    g.Go(func() error {
        var err error
        stats, err = fetchStats(ctx)
        return err
    })

    if err := g.Wait(); err != nil {
        return nil, err
    }
    return &Dashboard{User: user, Orders: orders, Stats: stats}, nil
}
```

### Worker Pool

```go
func WorkerPool(ctx context.Context, tasks <-chan Task, workers int) <-chan Result {
    results := make(chan Result, workers)
    var wg sync.WaitGroup

    for i := 0; i < workers; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            for task := range tasks {
                select {
                case <-ctx.Done():
                    return
                default:
                    results <- task.Execute()
                }
            }
        }()
    }

    go func() {
        wg.Wait()
        close(results)
    }()

    return results
}
```

## 服务注册与发现

### Consul 注册

```go
import consul "github.com/hashicorp/consul/api"

func RegisterService(addr string, port int) error {
    client, err := consul.NewClient(consul.DefaultConfig())
    if err != nil {
        return err
    }

    reg := &consul.AgentServiceRegistration{
        ID:      fmt.Sprintf("my-service-%d", port),
        Name:    "my-service",
        Address: addr,
        Port:    port,
        Check: &consul.AgentServiceCheck{
            HTTP:                           fmt.Sprintf("http://%s:%d/healthz", addr, port),
            Interval:                       "10s",
            Timeout:                        "3s",
            DeregisterCriticalServiceAfter: "30s",
        },
    }
    return client.Agent().ServiceRegister(reg)
}
```

## 高可用 Checklist

### 基础保障
- [ ] 优雅关闭（SIGINT/SIGTERM 信号捕获）
- [ ] Panic Recovery 中间件
- [ ] 请求超时中间件 + Context 传播
- [ ] 所有连接池已配置（DB / Redis / HTTP）

### 容错机制
- [ ] 下游调用使用熔断器
- [ ] 网络调用配置重试 + 指数退避
- [ ] 分布式锁保护互斥资源
- [ ] 限流中间件（单机 / 分布式）

### 可观测性
- [ ] Prometheus 指标采集（QPS、延迟、错误率）
- [ ] 结构化日志 (JSON)
- [ ] 健康探针 (/healthz, /readyz)
- [ ] 链路追踪 (OpenTelemetry)

### 部署
- [ ] 多实例部署 + 负载均衡
- [ ] Kubernetes Liveness / Readiness Probe
- [ ] 服务注册与发现
- [ ] 滚动更新 / 蓝绿部署

## 常用依赖

| 用途 | 包 |
|------|-----|
| 熔断器 | `github.com/sony/gobreaker/v2` |
| 限流 | `golang.org/x/time/rate` |
| 并发控制 | `golang.org/x/sync/errgroup` |
| 指标采集 | `github.com/prometheus/client_golang` |
| 链路追踪 | `go.opentelemetry.io/otel` |
| 服务发现 | `github.com/hashicorp/consul/api` |
| 结构化日志 | `github.com/sirupsen/logrus` |
| 分布式锁 | `github.com/go-redis/redis` |
