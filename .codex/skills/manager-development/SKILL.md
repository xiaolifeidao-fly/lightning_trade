---
name: manager-development
description: Build production-grade frontend modules using React + Ant Design + Next.js. Use this skill when creating new pages, components, API integrations, or optimizing existing frontend code in the webview layer.
license: Complete terms in LICENSE.txt
---

# Frontend Development Skill

## Overview

本项目是一个管理界面的前端层，基于 **React 18 + Ant Design 5 + Next.js 14 (App Router)** 构建。
## Skill Behavior

当用户请求涉及前端开发时，遵循本技能指引。

### 触发场景

- 新增页面模块（page + api + components + hooks）
- 创建/修改 UI 组件、表单、列表、详情页
- 对接服务端 API（axios 代理转发）,参考
- 对接 Electron IPC API（ElectronApi RPC 调用 / 消息监听）
- 状态管理、数据流设计
- 国际化（i18n）
- 主题定制、样式优化
- 性能优化、可维护性重构

### 参考文档

- [架构设计规范](references/frontend-architecture.md) — 模块结构、API 层模式、状态管理、性能优化、代码模板
## Instructions

### 1. 技术栈速查

| 层级 | 技术 | 说明 |
|------|------|------|
| 框架 | Next.js 14 (App Router) | `"use client"` 客户端组件 |
| UI 库 | Ant Design 5 + Pro Components 2.8 | Token 主题系统 |
| 语言 | TypeScript 5 | 严格类型，禁止 any |
| HTTP | import { getDataList, getPage, getData, instance } from "@utils/axios"; |  自动映射 |
| 国际化 | next-intl + Ant Design Locale | `useLocale()` Hook |
| 样式 | Ant Design Token + CSS Modules | 不使用 Tailwind |
| 路径别名 | `@/*`、`@utils/*`、`@eleapi/*`、`@model/*` | 见 tsconfig.json |

### 2. 新模块开发标准流程

按以下顺序创建新的业务模块：

**Step 1** → 创建目录结构：`src/app/{module}/page.tsx` + `api/{module}.api.ts`

**Step 2** → 定义 API 层： 类 + 请求函数（详见架构设计规范）

**Step 3** → 编写页面组件：状态管理 + 组件组合 + 错误处理

**Step 4** → 抽取组件：将页面中的独立功能块拆分到 `components/`

**Step 5** → 抽取 Hooks：将数据获取和操作逻辑移到 `hooks/`

**Step 6** → 国际化：在 `i18n/messages/*.json` 中添加翻译键值

**Step 7** → 如需 Electron 交互，遵循 [Electron 桥接指南](reference/frontend-electron-bridge.md)

### 3. 两条 API 通道

本项目有两种完全不同的 API 交互方式，**根据数据来源选择正确的通道**：

#### 通道 A：服务端 API（通过 axios 代理）

适用于：与远程服务器交互的数据（用户数据、业务数据、列表查询等）

> **⚠️ 强制要求**：所有 HTTP 请求**必须**使用 `@utils/axios` 封装，**严禁**手写 `fetch`/`request` 函数。`@utils/axios` 内置了权限校验、错误处理、 自动映射等关键能力，手写会绕过这些机制。

**✅ 正确写法**（必须遵循）：

```typescript
import { instance, getData, getDataList, getPage } from "@utils/axios";

//  必须用 class（配合 class-transformer 自动反序列化）
export class Xxx {
  id!: number;
  name!: string;
}

// GET 请求 - 自动反序列化为  实例
const list = await getDataList<Xxx>(Xxx, '/api/xxx/list', params);
const page = await getPage<Xxx>(Xxx, '/api/xxx/page', params);
const item = await getData<Xxx>(Xxx, '/api/xxx/detail');

// POST/PUT/DELETE - 直接使用 axios 实例
await instance.post('/api/xxx/create', data);
await instance.put('/api/xxx/update', data);
await instance.delete('/api/xxx/delete');
```

**❌ 禁止写法**（即使在已有代码中看到也不要模仿）：

```typescript
// ❌ 禁止手写 request 函数
async function request<T>(url: string, options?: RequestInit): Promise<T> {
  const response = await fetch(url, { ...options });
  // ...
}
// ❌ 禁止直接使用 fetch
const response = await fetch('/api/xxx', { method: 'GET' });
// ❌ 禁止用 interface 定义 （无法配合 class-transformer）
interface Xxx { id: number; }
```

> 如果在代码库中发现有文件使用了上述禁止写法，应主动将其重构为正确写法。

#### 通道 B：Electron API（通过 RPC 框架）

适用于：本地能力调用（文件操作、系统通知、本地存储、设备信息等）

```typescript
import { XxxApi } from "@eleapi/xxx/xxx.api";

const api = new XxxApi();

// INVOKE：请求-响应（同步 RPC）
const result = await api.someMethod(params);

// TRRIGER：监听主进程推送的消息
api.onSomeEvent(sessionId, (data) => {
  // 处理 Electron 主进程推送的数据
});
```

### 4. 数据存储策略

| 数据类型 | 存储位置 | 方式 |
|----------|----------|------|
| 用户配置、认证凭证 | electron-store | 通过 ElectronApi 调用 |
| 业务缓存数据 | electron-store | 通过 ElectronApi 调用 |
| UI 偏好（语言、主题） | localStorage | 直接读写 |
| 临时 UI 状态（折叠、排序） | React State | 组件内管理 |
| 表单草稿 | sessionStorage | 页面级生命周期 |

**核心原则**：Electron 客户端应用的重要数据存储在 electron-store（主进程），只有不重要的 UI 偏好才放 localStorage。

### 5. 国际化

使用 `useLocale()` Hook，所有用户可见文本必须走 i18n：

```typescript
const { t, locale, setLocale } = useLocale();

// 翻译键使用 module.key 格式
<span>{t("case.status.pending")}</span>
```

新增模块时在 `src/i18n/messages/` 下 **所有语言文件** 中添加对应条目。

### 6. 开发规范速查

1. **`"use client"`**：所有页面组件顶部必须声明
2. **API 分离**：page.tsx 不直接写 HTTP 请求，统一通过 api/ 层
3. ** 映射**：使用 class + class-transformer，不使用原始 JSON
4. **组件粒度**：page.tsx 负责编排，可复用 UI 拆到 components/
5. **Hook 抽取**：数据获取、操作逻辑封装为自定义 Hook
6. **类型安全**：禁止 any，所有接口定义明确类型
7. **错误处理**：统一 `message.error()` 展示，不吞异常
8. **国际化**：用户可见文本全部使用 `t()` 函数
9. **存储**：重要数据 → electron-store，UI 偏好 → localStorage
