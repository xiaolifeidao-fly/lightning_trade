---
name: frontend-architecture
description: Architecture patterns, module structure, API layer design, state management, performance optimization, and code templates for the React + Ant Design + Next.js frontend in an Electron desktop application.
---

# 前端架构设计规范

## 1. 模块架构模式

每个业务模块采用**分层隔离**的目录结构，保证职责单一、易于维护和扩展。

### 1.1 标准模块结构

```
src/app/{module}/
├── page.tsx                  # 页面入口 - 负责组件编排和布局
├── api/
│   └── {module}.api.ts       # API 层 - DTO 定义 + 请求封装
├── components/               # 组件层 - 可复用的 UI 单元
│   ├── XxxSearchForm.tsx     #   搜索/筛选表单
│   ├── XxxTable.tsx          #   数据表格
│   ├── XxxForm.tsx           #   新增/编辑表单（Modal 或 Drawer）
│   ├── XxxDetail.tsx         #   详情展示（Drawer 或 Panel）
│   └── XxxStatusTag.tsx      #   状态标签等原子组件
└── hooks/                    # Hook 层 - 状态逻辑封装
    ├── useXxxData.ts         #   数据获取 + 分页 + 刷新
    ├── useXxxActions.ts      #   CRUD 操作 + 反馈
    └── useXxxFilters.ts      #   筛选条件管理
```

### 1.2 各层职责边界

| 层级 | 职责 | 禁止 |
|------|------|------|
| **page.tsx** | 组件编排、布局结构、页面级状态组合 | 直接写 API 调用、大段业务逻辑 |
| **api/*.api.ts** | DTO 定义、HTTP/IPC 请求封装、数据映射 | UI 逻辑、React Hook、状态管理 |
| **components/*.tsx** | 单一 UI 功能、接收 props、触发事件 | 直接调用 API、管理全局状态 |
| **hooks/*.ts** | 数据获取、操作逻辑、状态管理 | 渲染 UI、操作 DOM |

### 1.3 模块间依赖规则

```
page.tsx
  ├── 引用 ./components/*    ✅
  ├── 引用 ./hooks/*         ✅
  ├── 引用 ./api/*           ✅（仅类型引用，实际调用在 hooks 中）
  └── 引用其他模块            ❌（通过公共 components/ 或 contexts/ 共享）

hooks/
  ├── 引用 ./api/*           ✅
  └── 引用 @utils/*          ✅

components/
  ├── 引用 antd              ✅
  ├── 引用 ./api/* 的类型    ✅（仅 type/interface）
  └── 引用 ./api/* 的函数    ❌（通过 props 传入）
```

## 2. API 层设计模式

### 2.1 服务端 API 封装（axios 通道）

```typescript
// src/app/{module}/api/{module}.api.ts
import { instance, getData, getDataList, getPage, PageData } from "@utils/axios";

// ━━━━━━━━━━━━━ DTO 定义 ━━━━━━━━━━━━━
export class OrderDTO {
  id!: string;
  orderNo!: string;
  customerName!: string;
  amount!: number;
  status!: "pending" | "processing" | "completed" | "cancelled";
  createdAt!: string;
  updatedAt!: string;
}

export interface OrderQueryParams {
  keyword?: string;
  status?: string;
  startDate?: string;
  endDate?: string;
  page?: number;
  size?: number;
}

// ━━━━━━━━━━━━━ 请求封装 ━━━━━━━━━━━━━
export const orderApi = {
  getPage: (params?: OrderQueryParams): Promise<PageData<OrderDTO>> =>
    getPage<OrderDTO>(OrderDTO, "/api/order/page", params),

  getList: (params?: Partial<OrderQueryParams>): Promise<OrderDTO[]> =>
    getDataList<OrderDTO>(OrderDTO, "/api/order/list", params),

  getDetail: (id: string): Promise<OrderDTO | null> =>
    getData<OrderDTO>(OrderDTO, `/api/order/${id}`),

  create: (data: Partial<OrderDTO>) =>
    instance.post("/api/order/create", data),

  update: (id: string, data: Partial<OrderDTO>) =>
    instance.put(`/api/order/${id}`, data),

  delete: (id: string) =>
    instance.delete(`/api/order/${id}`),

  batchDelete: (ids: string[]) =>
    instance.post("/api/order/batch-delete", { ids }),

  export: (params?: OrderQueryParams) =>
    instance.get("/api/order/export", { params, responseType: "blob" }),
};
```

**关键规则**：
- DTO 使用 `class` 而非 `interface`（配合 class-transformer 反序列化）
- 字段用 `!` 声明（明确非空断言，由反序列化填充）
- 将所有 API 函数聚合为一个 `xxxApi` 对象，而非散落的独立函数
- 查询参数使用独立的 `interface`（可以 Partial 复用）

### 2.2 Electron API 使用（RPC 通道）

详见 [Electron 桥接指南](./frontend-electron-bridge.md)。核心要点：

```typescript
import { SomeApi } from "@eleapi/some/some.api";

const someApi = new SomeApi();

// INVOKE：请求-响应
const result = await someApi.doSomething(params);

// TRRIGER：订阅推送
useEffect(() => {
  someApi.onSomeEvent(sessionId, (data) => { /* handle */ });
  return () => {
    someApi.removeOnMessage(someApi.getApiName(), "onSomeEvent");
  };
}, [sessionId]);
```

## 3. 状态管理模式

### 3.1 状态分层策略

| 状态类型 | 管理方式 | 示例 |
|----------|----------|------|
| **服务端数据** | 自定义 Hook（fetch + cache） | 列表数据、详情数据 |
| **页面交互状态** | `useState` | Modal 开关、选中行、Tab 切换 |
| **模块级共享状态** | `useContext` + Provider | 模块内多组件共享的筛选条件 |
| **全局应用状态** | 全局 Context | 用户信息、语言、主题 |
| **URL 状态** | `useSearchParams` | 分页参数、筛选条件（可选） |

### 3.2 数据获取 Hook 模式

```typescript
// hooks/useXxxData.ts
import { useState, useEffect, useCallback } from "react";
import { message } from "antd";
import { xxxApi, XxxDTO, XxxQueryParams } from "../api/xxx.api";
import type { PageData } from "@utils/axios";

interface UseXxxDataOptions {
  autoFetch?: boolean;
  defaultPageSize?: number;
}

interface UseXxxDataReturn {
  loading: boolean;
  dataSource: XxxDTO[];
  total: number;
  pagination: { current: number; pageSize: number };
  setPagination: (p: { current: number; pageSize: number }) => void;
  refresh: () => void;
  search: (params: XxxQueryParams) => void;
}

export function useXxxData(options: UseXxxDataOptions = {}): UseXxxDataReturn {
  const { autoFetch = true, defaultPageSize = 20 } = options;

  const [loading, setLoading] = useState(false);
  const [dataSource, setDataSource] = useState<XxxDTO[]>([]);
  const [total, setTotal] = useState(0);
  const [pagination, setPagination] = useState({
    current: 1,
    pageSize: defaultPageSize,
  });
  const [queryParams, setQueryParams] = useState<XxxQueryParams>({});

  const fetchData = useCallback(async () => {
    setLoading(true);
    try {
      const result = await xxxApi.getPage({
        ...queryParams,
        page: pagination.current,
        size: pagination.pageSize,
      });
      setDataSource(result.data);
      setTotal(result.total);
    } catch (error: unknown) {
      const msg = error instanceof Error ? error.message : "加载失败";
      message.error(msg);
    } finally {
      setLoading(false);
    }
  }, [pagination, queryParams]);

  useEffect(() => {
    if (autoFetch) fetchData();
  }, [fetchData, autoFetch]);

  const search = useCallback((params: XxxQueryParams) => {
    setQueryParams(params);
    setPagination((prev) => ({ ...prev, current: 1 }));
  }, []);

  return {
    loading,
    dataSource,
    total,
    pagination,
    setPagination,
    refresh: fetchData,
    search,
  };
}
```

### 3.3 操作逻辑 Hook 模式

```typescript
// hooks/useXxxActions.ts
import { useState, useCallback } from "react";
import { message, Modal } from "antd";
import { xxxApi, XxxDTO } from "../api/xxx.api";

interface UseXxxActionsReturn {
  submitting: boolean;
  handleCreate: (values: Partial<XxxDTO>) => Promise<boolean>;
  handleUpdate: (id: string, values: Partial<XxxDTO>) => Promise<boolean>;
  handleDelete: (id: string) => Promise<boolean>;
  handleBatchDelete: (ids: string[]) => Promise<boolean>;
}

export function useXxxActions(onSuccess?: () => void): UseXxxActionsReturn {
  const [submitting, setSubmitting] = useState(false);

  const handleCreate = useCallback(async (values: Partial<XxxDTO>) => {
    setSubmitting(true);
    try {
      await xxxApi.create(values);
      message.success("创建成功");
      onSuccess?.();
      return true;
    } catch (error: unknown) {
      const msg = error instanceof Error ? error.message : "创建失败";
      message.error(msg);
      return false;
    } finally {
      setSubmitting(false);
    }
  }, [onSuccess]);

  const handleDelete = useCallback(async (id: string) => {
    return new Promise<boolean>((resolve) => {
      Modal.confirm({
        title: "确认删除",
        content: "删除后不可恢复，是否确认？",
        onOk: async () => {
          try {
            await xxxApi.delete(id);
            message.success("删除成功");
            onSuccess?.();
            resolve(true);
          } catch (error: unknown) {
            const msg = error instanceof Error ? error.message : "删除失败";
            message.error(msg);
            resolve(false);
          }
        },
        onCancel: () => resolve(false),
      });
    });
  }, [onSuccess]);

  // handleUpdate, handleBatchDelete 同理...

  return { submitting, handleCreate, handleUpdate: handleCreate, handleDelete, handleBatchDelete: handleDelete };
}
```

## 4. 页面组合模式

### 4.1 CRUD 列表页模板

```typescript
// src/app/{module}/page.tsx
"use client";

import React, { useState } from "react";
import { Card, Space, Button } from "antd";
import { PlusOutlined, ReloadOutlined } from "@ant-design/icons";
import { useLocale } from "@/contexts/LocaleContext";
import { useXxxData } from "./hooks/useXxxData";
import { useXxxActions } from "./hooks/useXxxActions";
import XxxSearchForm from "./components/XxxSearchForm";
import XxxTable from "./components/XxxTable";
import XxxFormModal from "./components/XxxFormModal";
import XxxDetailDrawer from "./components/XxxDetailDrawer";
import type { XxxDTO } from "./api/xxx.api";

export default function XxxPage() {
  const { t } = useLocale();

  // 数据层
  const { loading, dataSource, total, pagination, setPagination, refresh, search } = useXxxData();
  const { submitting, handleCreate, handleUpdate, handleDelete } = useXxxActions(refresh);

  // 交互状态
  const [formVisible, setFormVisible] = useState(false);
  const [detailVisible, setDetailVisible] = useState(false);
  const [editingRecord, setEditingRecord] = useState<XxxDTO | null>(null);
  const [selectedRecord, setSelectedRecord] = useState<XxxDTO | null>(null);

  const handleEdit = (record: XxxDTO) => {
    setEditingRecord(record);
    setFormVisible(true);
  };

  const handleView = (record: XxxDTO) => {
    setSelectedRecord(record);
    setDetailVisible(true);
  };

  return (
    <Space direction="vertical" size="middle" style={{ width: "100%" }}>
      {/* 搜索区域 */}
      <Card size="small">
        <XxxSearchForm onSearch={search} loading={loading} />
      </Card>

      {/* 数据区域 */}
      <Card
        size="small"
        title={t("xxx.title")}
        extra={
          <Space>
            <Button icon={<ReloadOutlined />} onClick={refresh} />
            <Button
              type="primary"
              icon={<PlusOutlined />}
              onClick={() => { setEditingRecord(null); setFormVisible(true); }}
            >
              {t("common.create")}
            </Button>
          </Space>
        }
      >
        <XxxTable
          loading={loading}
          dataSource={dataSource}
          pagination={{ ...pagination, total }}
          onPaginationChange={setPagination}
          onEdit={handleEdit}
          onView={handleView}
          onDelete={handleDelete}
        />
      </Card>

      {/* 表单弹窗 */}
      <XxxFormModal
        open={formVisible}
        record={editingRecord}
        submitting={submitting}
        onSubmit={async (values) => {
          const ok = editingRecord
            ? await handleUpdate(editingRecord.id, values)
            : await handleCreate(values);
          if (ok) setFormVisible(false);
        }}
        onCancel={() => setFormVisible(false)}
      />

      {/* 详情抽屉 */}
      <XxxDetailDrawer
        open={detailVisible}
        record={selectedRecord}
        onClose={() => setDetailVisible(false)}
      />
    </Space>
  );
}
```

### 4.2 组件设计要点

**Table 组件**
```typescript
interface XxxTableProps {
  loading: boolean;
  dataSource: XxxDTO[];
  pagination: { current: number; pageSize: number; total: number };
  onPaginationChange: (p: { current: number; pageSize: number }) => void;
  onEdit: (record: XxxDTO) => void;
  onView: (record: XxxDTO) => void;
  onDelete: (id: string) => void;
}
```

**Form 组件**
```typescript
interface XxxFormModalProps {
  open: boolean;
  record: XxxDTO | null;  // null = 创建，有值 = 编辑
  submitting: boolean;
  onSubmit: (values: Partial<XxxDTO>) => Promise<void>;
  onCancel: () => void;
}
```

**设计原则**：
- 组件通过 props 接收数据和回调，不直接调用 API
- 使用 TypeScript interface 明确 props 契约
- 区分「受控」和「非受控」：表单状态内部管理，提交结果外部处理
- 操作按钮的 loading 状态由父级通过 props 传入

### 4.3 详情页模板

```typescript
"use client";

import React, { useEffect, useState } from "react";
import { useSearchParams } from "next/navigation";
import { Card, Descriptions, Spin, Timeline, Tag, message } from "antd";
import { xxxApi, XxxDTO } from "./api/xxx.api";

export default function XxxDetailPage() {
  const searchParams = useSearchParams();
  const id = searchParams.get("id");

  const [loading, setLoading] = useState(true);
  const [detail, setDetail] = useState<XxxDTO | null>(null);

  useEffect(() => {
    if (!id) return;
    setLoading(true);
    xxxApi.getDetail(id)
      .then(setDetail)
      .catch((e: Error) => message.error(e.message))
      .finally(() => setLoading(false));
  }, [id]);

  if (loading) return <Spin size="large" style={{ display: "block", margin: "100px auto" }} />;
  if (!detail) return <Card>数据不存在</Card>;

  return (
    <Card title={detail.name}>
      <Descriptions column={2} bordered>
        <Descriptions.Item label="ID">{detail.id}</Descriptions.Item>
        {/* ... */}
      </Descriptions>
    </Card>
  );
}
```

## 5. 错误处理规范

### 5.1 分层错误处理

```
API 层     → 抛出 HttpError（由 axios 拦截器统一包装）
Hook 层    → try/catch + message.error() 展示
组件层     → 通过 props.loading / props.error 展示状态
页面层     → 兜底 ErrorBoundary（可选）
```

### 5.2 加载状态管理

```typescript
// 推荐：在 Hook 中统一管理 loading
const [loading, setLoading] = useState(false);

// Table 的 loading
<Table loading={loading} ... />

// Button 的 loading
<Button loading={submitting} ... />

// 全页面 loading
{loading ? <Spin /> : <Content />}
```

### 5.3 空状态处理

```typescript
import { Empty, Result } from "antd";

// 无数据
{dataSource.length === 0 && !loading && (
  <Empty description={t("common.noData")} />
)}

// 请求失败
{error && (
  <Result
    status="error"
    title={t("common.loadFailed")}
    extra={<Button onClick={refresh}>重试</Button>}
  />
)}
```

## 6. 性能优化

### 6.1 渲染优化

```typescript
// 1. 使用 useCallback 稳定回调引用
const handleSearch = useCallback((values: SearchParams) => {
  search(values);
}, [search]);

// 2. 使用 useMemo 避免重复计算
const columns = useMemo<ColumnsType<XxxDTO>>(() => [
  { title: t("xxx.name"), dataIndex: "name" },
  // ...
], [t]);

// 3. 使用 React.memo 避免组件重渲染
const XxxTable = React.memo<XxxTableProps>(({ dataSource, loading, ... }) => {
  // ...
});
```

### 6.2 数据加载优化

```typescript
// 1. 防抖搜索
import { useDebouncedCallback } from "use-debounce";
const debouncedSearch = useDebouncedCallback((value: string) => {
  search({ keyword: value });
}, 300);

// 2. 请求取消（组件卸载时）
useEffect(() => {
  const controller = new AbortController();
  fetchData(controller.signal);
  return () => controller.abort();
}, []);

// 3. 按需加载组件
const XxxDetailDrawer = dynamic(() => import("./components/XxxDetailDrawer"), {
  loading: () => <Spin />,
});
```

### 6.3 Ant Design 优化

```typescript
// 1. Table 虚拟滚动（大数据量）
<Table virtual scroll={{ y: 600 }} />

// 2. 按需导入图标
import { PlusOutlined } from "@ant-design/icons";
// ❌ import * as Icons from "@ant-design/icons";

// 3. Modal/Drawer 销毁内容
<Modal destroyOnClose forceRender={false} ... />
```

## 7. 组件通信模式

### 7.1 父子通信：Props + Callback

```typescript
// 父 → 子：数据通过 props
<XxxTable dataSource={data} loading={loading} />

// 子 → 父：事件通过回调 props
<XxxSearchForm onSearch={(params) => search(params)} />
```

### 7.2 兄弟通信：状态提升到共同父组件

```typescript
// page.tsx 持有共享状态
const [selectedId, setSelectedId] = useState<string>();

<XxxList onSelect={setSelectedId} />
<XxxDetail id={selectedId} />
```

### 7.3 跨层通信：Context

```typescript
// 仅用于真正的全局/模块级共享状态
// 已有：LocaleContext（语言 + 主题）
// 按需创建模块级 Context
```

## 8. 文件命名规范

| 类型 | 命名格式 | 示例 |
|------|----------|------|
| 页面 | `page.tsx` | `src/app/order/page.tsx` |
| API | `{module}.api.ts` | `order.api.ts` |
| 组件 | `PascalCase.tsx` | `OrderTable.tsx` |
| Hook | `use{Name}.ts` | `useOrderData.ts` |
| 类型 | `{module}.types.ts` | `order.types.ts`（类型过多时抽离） |
| 工具 | `camelCase.ts` | `formatOrder.ts` |
| 样式 | `{Component}.module.css` | `OrderTable.module.css` |

## 9. 可扩展性设计

### 9.1 插件式组件

当表格列、表单字段需要按业务动态扩展时：

```typescript
// 列配置工厂
function createOrderColumns(options: {
  showActions?: boolean;
  onEdit?: (r: OrderDTO) => void;
}): ColumnsType<OrderDTO> {
  const base: ColumnsType<OrderDTO> = [
    { title: "订单号", dataIndex: "orderNo", width: 180 },
    { title: "客户", dataIndex: "customerName" },
    // ...
  ];

  if (options.showActions && options.onEdit) {
    base.push({
      title: "操作",
      fixed: "right",
      width: 120,
      render: (_, record) => (
        <Button type="link" onClick={() => options.onEdit!(record)}>编辑</Button>
      ),
    });
  }

  return base;
}
```

### 9.2 Hook 组合

```typescript
// 组合多个细粒度 Hook 为模块级 Hook
function useOrderModule() {
  const data = useOrderData();
  const actions = useOrderActions(data.refresh);
  const filters = useOrderFilters();

  return { ...data, ...actions, ...filters };
}
```
