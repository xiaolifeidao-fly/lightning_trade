package main

import (
	"context"
	"log"

	"argus_single/initialization"
	pcweb "common/utils/pc_trade/web"
)

func main() {
	// 初始化全局 WASM 签名器（直连 DeepCoin Web 接口必须）
	if err := pcweb.InitGlobalSigner(context.Background()); err != nil {
		log.Fatalf("❌ 初始化 WASM 签名器失败: %v", err)
	}
	log.Println("✅ WASM 签名器初始化成功")

	initialization.Init()
}
