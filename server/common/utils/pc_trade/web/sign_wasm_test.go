package web

import (
	"context"
	"fmt"
	"testing"
)

func TestWasmSigner_SignParams(t *testing.T) {
	// 测试参数（与 sign_demo.js 中的一致）
	testParams := "appid=547798&convertPOST=1&randomstr=mC2CwB&timestamp=1770282777985"

	// 创建 WASM 签名器
	signer, err := NewWasmSigner(context.Background())
	if err != nil {
		t.Fatalf("创建 WASM 签名器失败: %v", err)
	}

	// 生成签名
	signature, err := signer.SignParams(testParams)
	if err != nil {
		t.Fatalf("生成签名失败: %v", err)
	}

	// 验证签名格式
	if len(signature) != 32 {
		t.Errorf("签名长度不正确，期望 32，实际 %d", len(signature))
	}

	fmt.Printf("输入参数: %s\n", testParams)
	fmt.Printf("生成签名: %s\n", signature)

	// 预期签名（根据 sign_demo.js 的输出）
	expectedSignature := "255c26e49bcf8eac8898276573bc9907"
	if signature != expectedSignature {
		t.Errorf("签名不匹配\n期望: %s\n实际: %s", expectedSignature, signature)
	}
}

func TestSignParamsSimple(t *testing.T) {
	// 使用简化函数
	testParams := "appid=547798&convertPOST=1&randomstr=mC2CwB&timestamp=1770282777985"

	signature, err := SignParamsSimple(testParams)
	if err != nil {
		t.Fatalf("生成签名失败: %v", err)
	}

	fmt.Printf("简化函数签名结果: %s\n", signature)

	// 验证签名
	expectedSignature := "255c26e49bcf8eac8898276573bc9907"
	if signature != expectedSignature {
		t.Errorf("签名不匹配\n期望: %s\n实际: %s", expectedSignature, signature)
	}
}

func TestWasmSigner_MultipleSignatures(t *testing.T) {
	// 创建签名器
	signer, err := NewWasmSigner(context.Background())
	if err != nil {
		t.Fatalf("创建 WASM 签名器失败: %v", err)
	}

	// 测试多个不同的参数
	testCases := []struct {
		name   string
		params string
	}{
		{
			name:   "测试参数1",
			params: "appid=547798&convertPOST=1&randomstr=mC2CwB&timestamp=1770282777985",
		},
		{
			name:   "测试参数2",
			params: "appid=547798&convertPOST=1&randomstr=ABC123&timestamp=1770282777986",
		},
		{
			name:   "测试参数3",
			params: "appid=547798&convertPOST=1&randomstr=XYZ789&timestamp=1770282777987",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			signature, err := signer.SignParams(tc.params)
			if err != nil {
				t.Errorf("生成签名失败: %v", err)
				return
			}

			if len(signature) != 32 {
				t.Errorf("签名长度不正确，期望 32，实际 %d", len(signature))
			}

			fmt.Printf("%s: %s -> %s\n", tc.name, tc.params, signature)
		})
	}
}

// BenchmarkWasmSigner 性能基准测试
func BenchmarkWasmSigner(b *testing.B) {
	signer, err := NewWasmSigner(context.Background())
	if err != nil {
		b.Fatalf("创建 WASM 签名器失败: %v", err)
	}

	testParams := "appid=547798&convertPOST=1&randomstr=mC2CwB&timestamp=1770282777985"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := signer.SignParams(testParams)
		if err != nil {
			b.Fatalf("生成签名失败: %v", err)
		}
	}
}

// BenchmarkSignParamsSimple 简化函数性能基准测试
func BenchmarkSignParamsSimple(b *testing.B) {
	testParams := "appid=547798&convertPOST=1&randomstr=mC2CwB&timestamp=1770282777985"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := SignParamsSimple(testParams)
		if err != nil {
			b.Fatalf("生成签名失败: %v", err)
		}
	}
}
