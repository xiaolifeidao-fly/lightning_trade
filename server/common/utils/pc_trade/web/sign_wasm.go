//go:build js || wasm

package web

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

const wasmFileName = "sign_params_bg.f38cd627.wasm"

var (
	wasmRuntime   wazero.Runtime
	wasmModule    api.Module
	wasmMutex     sync.Mutex
	wasmInitOnce  sync.Once
	wasmInitError error

	// 全局单例 signer 实例
	globalSigner    *WasmSigner
	signerInitOnce  sync.Once
	signerInitError error
)

// WasmSigner WASM 签名器
type WasmSigner struct {
	runtime wazero.Runtime
	module  api.Module
	ctx     context.Context
}

// initWasm 初始化 WASM 模块（单例模式）
func initWasm(ctx context.Context) error {
	wasmInitOnce.Do(func() {
		// 创建 wazero 运行时
		wasmRuntime = wazero.NewRuntime(ctx)

		// 实例化 WASI，虽然这个 WASM 可能不需要，但添加以防万一
		wasi_snapshot_preview1.MustInstantiate(ctx, wasmRuntime)

		// 读取 WASM 文件 - 从文件系统读取
		// 获取当前文件所在目录
		_, filename, _, ok := runtime.Caller(0)
		if !ok {
			wasmInitError = fmt.Errorf("无法获取当前文件路径")
			return
		}
		dir := filepath.Dir(filename)
		wasmPath := filepath.Join(dir, wasmFileName)

		wasmData, err := os.ReadFile(wasmPath)
		if err != nil {
			wasmInitError = fmt.Errorf("读取 WASM 文件失败: %w", err)
			return
		}

		// 定义主机函数（WASM 导入的函数）
		_, err = wasmRuntime.NewHostModuleBuilder("wbg").
			NewFunctionBuilder().
			WithFunc(func(ctx context.Context, m api.Module, ptr, len uint32) uint64 {
				// __wbindgen_cast_2241b6af4c4b2941
				// 从 WASM 内存读取字符串
				bytes, ok := m.Memory().Read(ptr, len)
				if !ok {
					return 0
				}
				// 这里只是读取，实际使用时可能需要处理
				_ = string(bytes)
				return 0
			}).Export("__wbindgen_cast_2241b6af4c4b2941").
			NewFunctionBuilder().
			WithFunc(func(ctx context.Context, m api.Module) {
				// __wbindgen_init_externref_table
				// 初始化外部引用表（如果需要的话）
			}).Export("__wbindgen_init_externref_table").
			Instantiate(ctx)

		if err != nil {
			wasmInitError = fmt.Errorf("实例化主机模块失败: %w", err)
			return
		}

		// 编译并实例化 WASM 模块
		wasmModule, err = wasmRuntime.Instantiate(ctx, wasmData)
		if err != nil {
			wasmInitError = fmt.Errorf("实例化 WASM 模块失败: %w", err)
			return
		}

		// 调用 __wbindgen_start 初始化函数（如果存在）
		start := wasmModule.ExportedFunction("__wbindgen_start")
		if start != nil {
			_, err = start.Call(ctx)
			if err != nil {
				wasmInitError = fmt.Errorf("调用 __wbindgen_start 失败: %w", err)
				return
			}
		}
	})

	return wasmInitError
}

// InitGlobalSigner 初始化全局签名器实例（在程序启动时调用）
func InitGlobalSigner(ctx context.Context) error {
	signerInitOnce.Do(func() {
		globalSigner, signerInitError = NewWasmSigner(ctx)
	})
	return signerInitError
}

// GetGlobalSigner 获取全局签名器实例
func GetGlobalSigner() (*WasmSigner, error) {
	if globalSigner == nil {
		return nil, fmt.Errorf("签名器未初始化，请先调用 InitGlobalSigner")
	}
	return globalSigner, nil
}

// NewWasmSigner 创建 WASM 签名器
func NewWasmSigner(ctx context.Context) (*WasmSigner, error) {
	if err := initWasm(ctx); err != nil {
		return nil, err
	}

	return &WasmSigner{
		runtime: wasmRuntime,
		module:  wasmModule,
		ctx:     ctx,
	}, nil
}

// SignParams 使用 WASM 生成签名
// params: 参数字符串，格式如 "appid=547798&convertPOST=1&randomstr=mC2CwB&timestamp=1770282777985"
func (s *WasmSigner) SignParams(params string) (string, error) {
	wasmMutex.Lock()
	defer wasmMutex.Unlock()

	// 获取 WASM 导出的函数
	malloc := s.module.ExportedFunction("__wbindgen_malloc")
	if malloc == nil {
		return "", fmt.Errorf("未找到 __wbindgen_malloc 函数")
	}

	realloc := s.module.ExportedFunction("__wbindgen_realloc")
	if realloc == nil {
		return "", fmt.Errorf("未找到 __wbindgen_realloc 函数")
	}

	signParamsJS := s.module.ExportedFunction("sign_params_js")
	if signParamsJS == nil {
		return "", fmt.Errorf("未找到 sign_params_js 函数")
	}

	free := s.module.ExportedFunction("__wbindgen_free")
	if free == nil {
		return "", fmt.Errorf("未找到 __wbindgen_free 函数")
	}

	// 将参数字符串写入 WASM 内存
	paramBytes := []byte(params)
	paramLen := uint64(len(paramBytes))

	// 分配内存
	results, err := malloc.Call(s.ctx, paramLen, 1)
	if err != nil {
		return "", fmt.Errorf("分配内存失败: %w", err)
	}
	ptr := uint32(results[0])

	// 写入参数
	memory := s.module.Memory()
	if !memory.Write(ptr, paramBytes) {
		return "", fmt.Errorf("写入参数到 WASM 内存失败")
	}

	// 调用签名函数
	// sign_params_js(ptr: u32, len: u32) -> [u32, u32, u32, u32]
	// 返回值: [resultPtr, resultLen, errorRef, hasError]
	results, err = signParamsJS.Call(s.ctx, uint64(ptr), paramLen)
	if err != nil {
		return "", fmt.Errorf("调用 sign_params_js 失败: %w", err)
	}

	// 解析返回值
	resultPtr := uint32(results[0])
	resultLen := uint32(results[1])
	// errorRef := uint32(results[2])
	hasError := uint32(results[3])

	// 检查是否有错误
	if hasError != 0 {
		return "", fmt.Errorf("WASM 签名函数返回错误")
	}

	// 读取结果字符串
	resultBytes, ok := memory.Read(resultPtr, resultLen)
	if !ok {
		return "", fmt.Errorf("从 WASM 内存读取结果失败")
	}
	signature := string(resultBytes)

	// 释放内存
	_, err = free.Call(s.ctx, uint64(resultPtr), uint64(resultLen), 1)
	if err != nil {
		// 记录错误但不中断，因为签名已经获取到了
		fmt.Printf("警告: 释放 WASM 内存失败: %v\n", err)
	}

	return signature, nil
}

// SignParamsSimple 简化的签名函数（自动创建和销毁 signer）
func SignParamsSimple(params string) (string, error) {
	signer, err := NewWasmSigner(context.Background())
	if err != nil {
		return "", err
	}
	return signer.SignParams(params)
}

// Close 关闭 WASM 运行时（通常在程序退出时调用）
func (s *WasmSigner) Close(ctx context.Context) error {
	if s.runtime != nil {
		return s.runtime.Close(ctx)
	}
	return nil
}
