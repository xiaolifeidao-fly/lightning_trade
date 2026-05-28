# Backend Performance & Scalability (Go)

Go 后端性能优化策略、缓存模式与可扩展性最佳实践。

## 数据库性能

### 索引策略

**影响：** 30% 磁盘 I/O 降低，10-100x 查询加速

```sql
-- 常用查询列索引
CREATE INDEX idx_users_email ON user_record(email);
CREATE INDEX idx_orders_user_id ON orders(user_id);

-- 复合索引
CREATE INDEX idx_orders_user_date ON orders(user_id, created_time DESC);

-- 前缀索引（长字符串列）
CREATE INDEX idx_users_email_prefix ON user_record(email(50));
```

**MySQL 索引类型：**
- **B-tree** - 默认，通用（等值、范围、排序查询）
- **Hash** - 仅 MEMORY 引擎支持，快速等值查找
- **FULLTEXT** - 全文搜索

**不适合建索引的场景：** 小表(<1000行)、频繁更新的列、低基数列(如 boolean/active)

### 连接池

**影响：** 5-10x 性能提升

```go
func InitDB(dsn string) (*gorm.DB, error) {
    db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
        Logger: logger.Default.LogMode(logger.Error),
    })
    if err != nil {
        return nil, fmt.Errorf("db connect failed: %w", err)
    }

    sqlDB, _ := db.DB()
    sqlDB.SetMaxOpenConns(100)
    sqlDB.SetMaxIdleConns(20)
    sqlDB.SetConnMaxLifetime(30 * time.Minute)
    sqlDB.SetConnMaxIdleTime(5 * time.Minute)
    return db, nil
}
```

**推荐池大小：** `connections = (cpu_cores * 2) + effective_spindle_count`，典型值 20-100

### N+1 查询问题

```go
// Bad: N+1 查询
var posts []Post
db.Find(&posts)
for i := range posts {
    db.Where("id = ?", posts[i].AuthorID).First(&posts[i].Author) // N 次查询!
}

// Good: 预加载
var posts []Post
db.Preload("Author").Find(&posts) // 仅 2 次查询
```

## 缓存策略

### Cache-Aside 模式

**影响：** 90% DB 负载降低，10-100x 响应加速

```go
func GetUser(ctx context.Context, rdb *redis.Client, db *gorm.DB, userID string) (*User, error) {
    key := fmt.Sprintf("user:%s", userID)

    val, err := rdb.Get(ctx, key).Result()
    if err == nil {
        var user User
        json.Unmarshal([]byte(val), &user)
        return &user, nil
    }

    var user User
    if err := db.First(&user, "id = ?", userID).Error; err != nil {
        return nil, err
    }

    data, _ := json.Marshal(user)
    rdb.Set(ctx, key, data, time.Hour)
    return &user, nil
}
```

### Write-Through 模式

```go
func UpdateUser(ctx context.Context, rdb *redis.Client, db *gorm.DB, userID string, updates map[string]interface{}) error {
    if err := db.Model(&User{}).Where("id = ?", userID).Updates(updates).Error; err != nil {
        return err
    }

    var user User
    db.First(&user, "id = ?", userID)
    data, _ := json.Marshal(user)
    rdb.Set(ctx, fmt.Sprintf("user:%s", userID), data, time.Hour)
    return nil
}
```

### 缓存最佳实践

1. **缓存热数据** - 用户资料、配置、商品目录
2. **合理设置 TTL** - 平衡新鲜度与性能
3. **写时失效** - 保持缓存一致性
4. **Key 命名规范** - `resource:id:attribute`
5. **监控命中率** - 目标 >80%

## 负载均衡

### 算法

```nginx
# Round Robin
upstream backend {
    server backend1:8080;
    server backend2:8080;
}

# 最少连接
upstream backend {
    least_conn;
    server backend1:8080;
    server backend2:8080;
}

# IP Hash（会话保持）
upstream backend {
    ip_hash;
    server backend1:8080;
    server backend2:8080;
}
```

### 健康检查

```go
func HealthHandler(db *gorm.DB, rdb *redis.Client) gin.HandlerFunc {
    return func(c *gin.Context) {
        checks := map[string]string{"status": "ok"}
        code := http.StatusOK

        if sqlDB, err := db.DB(); err != nil || sqlDB.Ping() != nil {
            checks["database"] = "unhealthy"
            code = http.StatusServiceUnavailable
        }
        if rdb.Ping(c).Err() != nil {
            checks["redis"] = "unhealthy"
            code = http.StatusServiceUnavailable
        }

        c.JSON(code, checks)
    }
}
```

## 异步处理

### 使用 Goroutine + Channel

```go
func ProcessOrderAsync(orderCh <-chan Order, workerCount int) {
    var wg sync.WaitGroup
    for i := 0; i < workerCount; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            for order := range orderCh {
                if err := processOrder(order); err != nil {
                    log.Printf("process order %s failed: %v", order.ID, err)
                }
            }
        }()
    }
    wg.Wait()
}
```

**适用场景：** 邮件发送、图片处理、报表生成、数据导出、Webhook 投递

## 数据库扩展模式

### 读写分离

```go
import "gorm.io/plugin/dbresolver"

db.Use(dbresolver.Register(dbresolver.Config{
    Sources:  []gorm.Dialector{mysql.Open(primaryDSN)},
    Replicas: []gorm.Dialector{mysql.Open(replica1DSN), mysql.Open(replica2DSN)},
    Policy:   dbresolver.RandomPolicy{},
}))
```

### 分片策略

- **Range-based：** Users 1-1M → Shard 1, 1M-2M → Shard 2
- **Hash-based：** Hash(userID) % shard_count
- **Geographic：** 按地区分片

## 性能监控

**关键指标：**
- 响应时间 (p50, p95, p99)
- 吞吐量 (QPS)
- 错误率
- CPU / 内存使用率
- 连接池饱和度
- 缓存命中率

**工具栈：** Prometheus + Grafana (指标) | OpenTelemetry (链路追踪) | Sentry (错误追踪)

## 性能优化 Checklist

- [ ] 常用查询列已建索引
- [ ] 连接池已配置（DB / Redis / HTTP）
- [ ] N+1 查询已消除
- [ ] Redis 缓存热数据，命中率 >80%
- [ ] 长耗时任务异步处理
- [ ] 响应压缩已开启 (gzip)
- [ ] 负载均衡已配置
- [ ] 健康检查已实现
- [ ] APM 监控已接入
- [ ] 慢查询日志已开启
