// argus-session-rotate 轮换某个实例的 DeepCoin 会话凭证（cookie / token）。
//
// 为什么需要它：账户是 login_type=config（静态凭证）模式 —— trade.BuildUserProvider
// 走 StaticUserProvider，进程**永远不会自己重登**（只有 password 模式才会去调
// pl-instance 的无头登录，而生产上没部署它）。而管理端的会话面板是**只读**的
// （SessionAudit：只回长度不回明文）。于是凭证过期后既没有自动续期、也没有产品化
// 入口，历史上只能直接改库（argus_runtime_session 里还留着 rotate-drill、
// fix-session-2026-09-08 这类手工痕迹）。这个 CLI 把那件事变成可复核的操作。
//
// 用法（默认 dry run，先看清楚再写）：
//
//	cd /data/program/app/manager-api
//	./argus-session-rotate --instance argus-single-fly --session /tmp/session.json
//	./argus-session-rotate --instance argus-single-fly --session /tmp/session.json --apply
//
// 全程不打印任何明文凭证，只打印长度与时间。
package main

import (
	"context"
	"flag"
	"log"
	"path/filepath"
	"strings"

	"common/middleware/db"
	commonRedis "common/middleware/redis"
	"common/middleware/vipper"
	argusConfig "service/argus_config"
)

func main() {
	instanceKey := flag.String("instance", "", "目标实例键，例如 argus-single-fly（必填）")
	sessionPath := flag.String("session", "", "新的 session.json 路径（必填，文件名必须是 session.json）")
	apply := flag.Bool("apply", false, "真正写库；不给则只打印计划")
	notify := flag.Bool("notify", true, "写完后通过 Redis 让实例立刻重读配置；不给则等它自己轮询（最多 60 秒）")
	actor := flag.String("actor", "argus-session-rotate", "审计标记，写进 updated_by")
	flag.Parse()

	if strings.TrimSpace(*instanceKey) == "" || strings.TrimSpace(*sessionPath) == "" {
		log.Fatal("--instance 与 --session 都是必填")
	}

	// 先做纯文件校验：文件名、JSON 结构、账户非空。这一步不碰库，
	// 拿错文件时不该先连上生产库再报错。
	incoming, err := argusConfig.LoadRotateSessionFile(filepath.Clean(*sessionPath))
	if err != nil {
		log.Fatalf("读 session.json 失败：%v", err)
	}
	log.Printf("session.json 校验通过：%d 个账户", len(incoming))

	vipper.Init()
	db.InitDB()
	if db.Db == nil {
		log.Fatal("数据库初始化失败")
	}

	ctx := context.Background()
	service := argusConfig.NewArgusConfigService()
	plan, err := service.PlanSessionRotate(ctx, *instanceKey, incoming)
	if err != nil {
		log.Fatalf("生成轮换计划失败：%v", err)
	}

	log.Printf("实例 %s 当前已发布 v%d，账户 %d 个：", plan.InstanceKey, plan.Version, len(plan.Items))
	for _, item := range plan.Items {
		log.Printf("  %s", item)
	}

	if plan.RotateCount() == 0 {
		log.Print("没有需要轮换的账户（凭证与库里一致，或 session.json 未覆盖到）；不写库")
		return
	}
	if !*apply {
		log.Printf("dry run 结束：将写 %d 行。确认无误后加 --apply 重跑", plan.RotateCount())
		return
	}

	written, err := service.ApplySessionRotate(ctx, plan, strings.TrimSpace(*actor))
	if err != nil {
		// 逐行写、每行都是完整一套凭证，所以中途失败不会留下"半套凭证"的账户。
		log.Fatalf("已写 %d 行后失败：%v", written, err)
	}
	log.Printf("已更新 %d 行会话凭证", written)

	if !*notify {
		log.Print("未发送 reload 通知：实例会在下一个配置比对周期（最多 60 秒）自行热加载")
		return
	}
	// Redis 只在需要通知时才初始化：dry run 与 --notify=false 都不该要求 Redis 可达。
	if err := commonRedis.InitRedisClient(vipper.GetString("redis.addr"), vipper.GetString("redis.password")); err != nil {
		log.Printf("⚠️ Redis 初始化失败，改由实例自行轮询生效（最多 60 秒）：%v", err)
		return
	}
	if err := service.NotifyReload(ctx, plan.InstanceKey); err != nil {
		log.Printf("⚠️ reload 通知发送失败，改由实例自行轮询生效（最多 60 秒）：%v", err)
		return
	}
	log.Print("已发送 reload 通知；到管理端「参数与运行控制」看该实例心跳回报的版本与校验和是否对上")
}
