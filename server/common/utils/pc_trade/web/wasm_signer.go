//go:build !js && !wasm

package web

import (
	"context"
	_ "embed"
	"fmt"
	"sync"
	"unsafe"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

//go:embed sign_params_bg.f38cd627.wasm
var wasmFileData []byte

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
		fmt.Printf("[initWasm] 步骤1: 创建 wazero 运行时\n")
		// 创建 wazero 运行时，启用所有特性包括 reference-types
		runtimeConfig := wazero.NewRuntimeConfig().
			WithCoreFeatures(api.CoreFeaturesV2)
		wasmRuntime = wazero.NewRuntimeWithConfig(ctx, runtimeConfig)
		fmt.Printf("[initWasm] wazero 运行时创建成功\n")

		fmt.Printf("[initWasm] 步骤2: 实例化 WASI\n")
		// 实例化 WASI，虽然这个 WASM 可能不需要，但添加以防万一
		wasi_snapshot_preview1.MustInstantiate(ctx, wasmRuntime)
		fmt.Printf("[initWasm] WASI 实例化成功\n")

		// 使用嵌入的 WASM 文件数据
		fmt.Printf("[initWasm] 步骤3: 检查嵌入的 WASM 文件数据\n")
		if len(wasmFileData) == 0 {
			wasmInitError = fmt.Errorf("嵌入的 WASM 文件数据为空")
			fmt.Printf("[initWasm] ❌ WASM 文件数据为空\n")
			return
		}
		fmt.Printf("[initWasm] WASM 文件数据大小: %d 字节\n", len(wasmFileData))

		fmt.Printf("[initWasm] 步骤4: 定义主机函数\n")
		// 定义主机函数（WASM 导入的函数）
		_, err := wasmRuntime.NewHostModuleBuilder("wbg").
			NewFunctionBuilder().
			WithGoModuleFunction(api.GoModuleFunc(func(ctx context.Context, m api.Module, stack []uint64) {
				// __wbindgen_cast_2241b6af4c4b2941
				// 从 WASM 内存读取字符串，返回 externref
				ptr := api.DecodeU32(stack[0])
				length := api.DecodeU32(stack[1])

				bytes, ok := m.Memory().Read(ptr, length)
				if !ok {
					// 返回编码后的 null externref
					stack[0] = api.EncodeExternref(0)
					return
				}
				// 将字符串转换为 uintptr 并编码为 externref
				str := string(bytes)
				// 注意：这里我们需要保持字符串的引用，因为 WASM 可能会使用它
				// 在实际应用中，可能需要一个全局的引用管理机制
				strPtr := uintptr(unsafe.Pointer(&str))
				stack[0] = api.EncodeExternref(strPtr)
			}), []api.ValueType{api.ValueTypeI32, api.ValueTypeI32}, []api.ValueType{api.ValueTypeExternref}).
			Export("__wbindgen_cast_2241b6af4c4b2941").
			NewFunctionBuilder().
			WithGoFunction(api.GoFunc(func(ctx context.Context, stack []uint64) {
				// __wbindgen_init_externref_table
				// 初始化外部引用表
				// 在 wazero 中，externref 表由运行时自动管理
				// 这个函数可以是空实现
			}), []api.ValueType{}, []api.ValueType{}).
			Export("__wbindgen_init_externref_table").
			Instantiate(ctx)

		if err != nil {
			wasmInitError = fmt.Errorf("实例化主机模块失败: %w", err)
			fmt.Printf("[initWasm] ❌ 实例化主机模块失败: %v\n", err)
			return
		}
		fmt.Printf("[initWasm] 主机函数定义成功\n")

		fmt.Printf("[initWasm] 步骤5: 编译并实例化 WASM 模块\n")
		// 编译并实例化 WASM 模块
		wasmModule, err = wasmRuntime.Instantiate(ctx, wasmFileData)
		if err != nil {
			wasmInitError = fmt.Errorf("实例化 WASM 模块失败: %w", err)
			fmt.Printf("[initWasm] ❌ 实例化 WASM 模块失败: %v\n", err)
			return
		}
		fmt.Printf("[initWasm] ✅ WASM 模块实例化成功\n")

		// 注意：如果 WASM 模块有 start 函数（如 __wbindgen_start），
		// wazero 会在 Instantiate 时自动调用它，无需显式调用
	})

	return wasmInitError
}

// InitGlobalSigner 初始化全局签名器实例（在程序启动时调用）
func InitGlobalSigner(ctx context.Context) error {
	signerInitOnce.Do(func() {
		fmt.Printf("[InitGlobalSigner] 开始初始化WASM签名器...\n")
		fmt.Printf("[InitGlobalSigner] 嵌入的WASM文件大小: %d 字节\n", len(wasmFileData))

		globalSigner, signerInitError = NewWasmSigner(ctx)

		if signerInitError != nil {
			fmt.Printf("[InitGlobalSigner] ❌ 初始化失败: %v\n", signerInitError)
		} else {
			fmt.Printf("[InitGlobalSigner] ✅ 初始化成功\n")
		}
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
