package routers

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"common/middleware/routers"
	"common/middleware/vipper"

	"argus_single/middleware"
	"argus_single/pkg/monitor"
	"argus_single/pkg/runtimehealth"

	"github.com/gin-gonic/gin"
)

var router *routers.GinRouter

func Init() {
	router = routers.NewGinRouter()

	// 添加日志和恢复中间件
	router.Use(middleware.Recovery())
	router.Use(middleware.Logger())

	registerHandlers := registerHandler()
	InitAllRouters(router, registerHandlers)
}

func Run(middleware ...gin.HandlerFunc) error {
	router.Use(middleware...)

	// 获取 gin engine
	engine := router.GetEngine()

	// 配置路由
	routerGroup := engine.Group(vipper.GetString("request.path"))
	for _, opt := range router.GetOptions() {
		opt(routerGroup)
	}

	// 获取端口
	port := vipper.GetString("server.port")

	// 配置 HTTP Server，添加超时设置
	srv := &http.Server{
		Addr:           ":" + port,
		Handler:        engine,
		ReadTimeout:    60 * time.Second,  // 读取请求超时（跨境网络需要更长时间）
		WriteTimeout:   90 * time.Second,  // 写入响应超时（跨境写入可能很慢）
		IdleTimeout:    120 * time.Second, // 空闲连接超时
		MaxHeaderBytes: 1 << 20,           // 1MB
		// 禁用 HTTP/2，使用 HTTP/1.1 更稳定
		ReadHeaderTimeout: 30 * time.Second,
	}

	// 优雅关闭
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("启动服务失败: %v\n", err)
		}
	}()

	log.Printf("服务器已启动，监听端口: %s", port)

	// 等待中断信号以优雅地关闭服务器
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("正在关闭服务器...")

	// 5秒超时关闭
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal("服务器强制关闭:", err)
	}

	// P4/R2：停止账户监控并同步 flush trail 状态。此前 SIGTERM 只关 HTTP，
	// AccountMonitor.Stop() 从未被调用，30s 异步快照盖不住正常部署重启
	// ——重启即丢已激活的峰值保护。必须在 HTTP 关闭之后、进程退出之前。
	log.Println("正在停止账户监控并保存 trail 状态...")
	monitor.StopAccountMonitor()
	runtimehealth.StopDefaultReporter()

	log.Println("服务器已关闭")
	return nil
}

// InitAllRouters 初始化所有router
func InitAllRouters(router *routers.GinRouter, handlers []routers.Handler) {
	for _, handler := range handlers {
		router.Include(handler.RegisterHandler)
	}
}
