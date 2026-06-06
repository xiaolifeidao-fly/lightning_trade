package middleware

import (
	"fmt"
	"log"
	"time"

	"github.com/gin-gonic/gin"
)

// Logger 日志中间件
func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 开始时间
		startTime := time.Now()

		// 记录请求信息
		path := c.Request.URL.Path
		method := c.Request.Method
		clientIP := c.ClientIP()

		log.Printf("[请求开始] %s %s from %s", method, path, clientIP)

		// 处理请求
		c.Next()

		// 结束时间
		endTime := time.Now()
		latency := endTime.Sub(startTime)

		// 记录响应信息
		statusCode := c.Writer.Status()

		log.Printf("[请求完成] %s %s from %s - 状态码: %d - 耗时: %v",
			method, path, clientIP, statusCode, latency)
	}
}

// Recovery 恢复中间件，防止 panic 导致服务崩溃
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				log.Printf("[PANIC] %v", err)
				c.JSON(500, gin.H{
					"code":  1,
					"data":  "服务器内部错误",
					"error": fmt.Sprintf("%v", err),
				})
				c.Abort()
			}
		}()
		c.Next()
	}
}
