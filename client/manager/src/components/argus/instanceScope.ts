"use client";

import { useCallback, useSyncExternalStore } from "react";

/**
 * 全局实例作用域。
 *
 * Argus 的五个页面（总览 / 实例对比 / 行情触发点 / 信号复盘 / 参数与运行控制）共用
 * 同一个「当前实例」，选择在页面之间与刷新之后都保持。之所以做成模块级 store 而不是
 * 各页面各存一份 localStorage：外壳顶栏的选择器和页面内容是两棵组件树，只靠
 * localStorage 不会互相通知，切了实例页面不会重渲染。
 *
 * 空串 = 全部实例（跨实例只读汇总）。**参数编辑与行情主视图不接受空串**：前者一次
 * 发布波及多个实例就毁掉 champion/challenger 对照组，后者的净持仓阶梯与阈值线跨实例
 * 混排没有意义。这两页由 ArgusInstanceSelector 的 allowAll=false 兜到具体实例。
 */
export const ALL_INSTANCES = "";

const STORAGE_KEY = "argus:instance-scope";

/** r7 参数页最早用的键。迁移期读一次做兜底，免得老用户回来时选择被清空。 */
const LEGACY_STORAGE_KEY = "argus-config:instance-key";

type Listener = () => void;

const listeners = new Set<Listener>();

/**
 * useSyncExternalStore 要求 getSnapshot 返回稳定引用，所以当前值必须缓存在模块变量里，
 * 不能每次现读 localStorage（字符串虽然是值类型不会抖，但 SSR 阶段读 window 会直接抛）。
 */
let current: string | null = null;

function readStored(): string {
  if (typeof window === "undefined") return ALL_INSTANCES;
  try {
    const stored = window.localStorage.getItem(STORAGE_KEY);
    if (stored !== null) return stored;
    return window.localStorage.getItem(LEGACY_STORAGE_KEY) ?? ALL_INSTANCES;
  } catch {
    // 隐私模式下 localStorage 可能直接抛，作用域退回「全部实例」即可，不该让页面挂掉。
    return ALL_INSTANCES;
  }
}

function ensureLoaded(): string {
  if (current === null) current = readStored();
  return current;
}

function emit() {
  listeners.forEach((listener) => listener());
}

function subscribe(listener: Listener): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

function getSnapshot(): string {
  return ensureLoaded();
}

/** SSR 阶段一律按「全部实例」渲染，客户端挂载后再由 localStorage 纠正。 */
function getServerSnapshot(): string {
  return ALL_INSTANCES;
}

/** 在组件外读取当前作用域（例如 API 层兜底）。 */
export function getArgusInstanceScope(): string {
  return ensureLoaded();
}

/** 写入作用域并广播。值没变时不触发重渲染，避免轮询回写导致的无谓刷新。 */
export function setArgusInstanceScope(next: string) {
  const value = next ?? ALL_INSTANCES;
  if (ensureLoaded() === value) return;
  current = value;
  if (typeof window !== "undefined") {
    try {
      window.localStorage.setItem(STORAGE_KEY, value);
    } catch {
      // 存不下就只在本次会话内生效，不影响当前页面。
    }
  }
  emit();
}

if (typeof window !== "undefined") {
  // 多标签页同时开着时，一边切实例另一边也跟上，避免两个标签对着不同实例读同一份结论。
  window.addEventListener("storage", (event) => {
    if (event.key !== STORAGE_KEY) return;
    const value = event.newValue ?? ALL_INSTANCES;
    if (current === value) return;
    current = value;
    emit();
  });
}

/** 读写全局实例作用域。返回的 setter 引用稳定，可以直接进依赖数组。 */
export function useArgusInstanceScope(): [string, (next: string) => void] {
  const scope = useSyncExternalStore(subscribe, getSnapshot, getServerSnapshot);
  const setScope = useCallback((next: string) => setArgusInstanceScope(next), []);
  return [scope, setScope];
}
