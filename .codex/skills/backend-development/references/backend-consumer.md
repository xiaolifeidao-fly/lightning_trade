# Backend Consumer Reference

本仓库后端 consumer 的默认位置是 `server/service/{domain}/consumer`。后续只要提到 consumer、消费者、异步队列消费、后台消费循环，都优先按这个路径落地。

## 职责边界

Consumer 包负责：

- 定义队列 key / topic / stream 名称
- 定义消息结构，例如 `XxxMessage`
- 提供入队函数，例如 `EnqueueXxx(...)`
- 提供启动函数，例如 `StartXxxConsumer(ctx, processor)`
- 处理出队、反序列化、日志、空消息、上下文退出

Consumer 包不负责：

- 复杂领域业务逻辑
- 直接操作 Repository 完成业务流程
- 在包内自行 new 具体 Service 造成强耦合

业务处理应继续放在 `service/{domain}`，由 consumer 通过小接口调用。

## 推荐目录

```text
server/service/foo/
  foo_service.go
  dto/
  repository/
  consumer/
    foo_consumer.go
```

## 推荐写法

```go
package consumer

import (
    "context"
    "encoding/json"
    "fmt"
    "log"
    "time"

    redisMiddleware "common/middleware/redis"

    "github.com/go-redis/redis"
)

const FooQueueKey = "foo:queue"

type FooProcessor interface {
    ProcessFoo(id uint64) error
}

type FooMessage struct {
    ID uint64 `json:"id"`
}

func EnqueueFoo(id uint64) error {
    if redisMiddleware.Rdb == nil {
        return fmt.Errorf("redis is not initialized")
    }
    payload, err := json.Marshal(FooMessage{ID: id})
    if err != nil {
        return err
    }
    return redisMiddleware.Rdb.RPush(FooQueueKey, string(payload)).Err()
}

func StartFooConsumer(ctx context.Context, processor FooProcessor) {
    if redisMiddleware.Rdb == nil {
        log.Printf("foo consumer skipped: redis is not initialized")
        return
    }
    if processor == nil {
        log.Printf("foo consumer skipped: processor is nil")
        return
    }
    go func() {
        log.Printf("foo consumer started")
        for {
            select {
            case <-ctx.Done():
                log.Printf("foo consumer stopped")
                return
            default:
            }

            values, err := redisMiddleware.Rdb.BRPop(5*time.Second, FooQueueKey).Result()
            if err != nil {
                if err != redis.Nil {
                    log.Printf("foo consumer pop failed: %v", err)
                }
                continue
            }
            if len(values) < 2 {
                continue
            }

            var message FooMessage
            if err := json.Unmarshal([]byte(values[1]), &message); err != nil {
                log.Printf("foo consumer invalid message: %v", err)
                continue
            }
            if err := processor.ProcessFoo(message.ID); err != nil {
                log.Printf("foo %d failed: %v", message.ID, err)
            }
        }
    }()
}
```

## Service 侧配合

```go
package foo

func (s *FooService) Generate(req *dto.GenerateFooDTO) error {
    // 先创建业务记录，再入队。
    return consumer.EnqueueFoo(uint64(entity.Id))
}

func (s *FooService) ProcessFoo(id uint64) error {
    // 业务校验、状态机、Repository 操作都放在 Service。
    return nil
}
```

## 初始化

在 `manager-api/initialization` 或 `app-api/initialization` 中启动 consumer，并确保启动顺序晚于 DB / Redis：

```go
consumer.StartFooConsumer(
    context.Background(),
    foo.NewFooService(),
)
```

## 检查清单

- Consumer 是否位于 `server/service/{domain}/consumer`
- Consumer 是否通过接口接收 Service，而不是 import 上层 API 包
- 是否避免了 `service/{domain}` 与 `service/{domain}/consumer` 的循环 import
- Redis 未初始化、processor 为 nil 时是否能清晰跳过
- `BRPop` 是否设置超时，以便定期检查 `ctx.Done()`
- 消息反序列化失败是否只记录日志并继续消费
- 业务失败是否由 Service 更新业务状态，Consumer 只记录错误
