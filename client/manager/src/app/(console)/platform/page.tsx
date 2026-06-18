"use client";

import { Tabs, Tag } from "antd";
import { CrudManagementPanel } from "../components/CrudManagementPanel";
import type { CrudField, CrudTableColumn } from "../components/CrudManagementPanel";
import {
  fetchPlatforms,
  createPlatform,
  updatePlatform,
  deletePlatform,
  fetchPlatformCoins,
  upsertPlatformCoin,
  updatePlatformCoin,
  deletePlatformCoin,
  fetchPlatformAccounts,
  createPlatformAccount,
  updatePlatformAccount,
  deletePlatformAccount,
  type PlatformRecord,
  type PlatformPayload,
  type PlatformCoinRecord,
  type PlatformCoinPayload,
  type PlatformAccountRecord,
  type PlatformAccountPayload,
} from "./api/platform.api";

const statusOptions = [
  { label: "上线", value: "online" },
  { label: "下线", value: "offline" },
  { label: "维护", value: "maintenance" },
];

const enableOptions = [
  { label: "启用", value: 1 },
  { label: "禁用", value: 0 },
];

const accountStatusOptions = [
  { label: "正常", value: "active" },
  { label: "禁用", value: "disabled" },
  { label: "过期", value: "expired" },
];

const platformInsight = (
  <>
    <strong>接入健康度</strong>
    <span>平台、币种映射、API 账户统一维护，优先保证上线平台具备交易币种和可用账户。</span>
  </>
);

function statusTag(value: unknown) {
  const v = String(value ?? "").toLowerCase();
  const color = v === "online" ? "green" : v === "offline" ? "red" : "orange";
  const label = v === "online" ? "上线" : v === "offline" ? "下线" : "维护";
  return <Tag color={color}>{label}</Tag>;
}

function enableTag(value: unknown) {
  return Number(value) === 1 ? <Tag color="green">启用</Tag> : <Tag color="default">禁用</Tag>;
}

function renderCsvTags(value: unknown) {
  const items = String(value || "")
    .split(",")
    .map((item) => item.trim())
    .filter(Boolean);
  return items.length > 0 ? items.map((item) => <Tag key={item}>{item}</Tag>) : "-";
}

function maskApiKey(value: unknown) {
  const text = String(value || "");
  return text ? `${text.slice(0, 8)}••••${text.slice(-4)}` : "-";
}

const fields: CrudField<PlatformRecord>[] = [
  { name: "code", label: "平台代码", required: true, placeholder: "如 binance / okx", disabledOnEdit: true },
  { name: "name", label: "平台名称", required: true, placeholder: "如 Binance" },
  { name: "fullName", label: "完整名称", placeholder: "如 Binance Exchange" },
  { name: "country", label: "国家/地区", placeholder: "如 Cayman Islands" },
  { name: "website", label: "官网", placeholder: "https://..." },
  { name: "apiBaseUrl", label: "API Base URL", placeholder: "https://api.binance.com" },
  { name: "wsBaseUrl", label: "WS Base URL", placeholder: "wss://..." },
  { name: "docsUrl", label: "接口文档", placeholder: "https://..." },
  { name: "supportedTypes", label: "支持类型", placeholder: "如 spot,futures,margin" },
  {
    name: "defaultFeeRate",
    label: "默认费率",
    type: "number",
    min: 0,
    precision: 6,
    placeholder: "0.001",
  },
  {
    name: "rateLimitPerSec",
    label: "限速(次/秒)",
    type: "number",
    min: 0,
    precision: 0,
    placeholder: "10",
  },
  {
    name: "status",
    label: "状态",
    type: "select",
    options: statusOptions,
    placeholder: "选择状态",
  },
  {
    name: "sortOrder",
    label: "排序",
    type: "number",
    min: 0,
    precision: 0,
    placeholder: "0",
  },
  { name: "description", label: "描述", type: "textarea", placeholder: "平台简介" },
];

const columns: CrudTableColumn<PlatformRecord>[] = [
  {
    name: "code",
    label: "平台",
    width: 170,
    render: (value, record) => (
      <div style={{ display: "flex", flexDirection: "column", gap: 4 }}>
        <span style={{ fontWeight: 800 }}>{String(value).toUpperCase()}</span>
        <span style={{ color: "var(--manager-text-faint)", fontSize: 12 }}>{record.name || "-"}</span>
      </div>
    ),
  },
  { name: "country", label: "国家", width: 120 },
  {
    name: "status",
    label: "状态",
    width: 100,
    render: (value) => statusTag(value),
  },
  { name: "website", label: "官网", width: 220, copyable: true },
  {
    name: "defaultFeeRate",
    label: "费率",
    width: 100,
    render: (value) => (value != null && value !== "" ? `${Number(value).toFixed(4)}` : "-"),
  },
  { name: "supportedTypes", label: "市场能力", width: 170, render: (value) => renderCsvTags(value) },
  { name: "updatedTime", label: "更新时间", width: 160 },
];

const platformCoinFields: CrudField<PlatformCoinRecord>[] = [
  { name: "platformId", label: "平台ID", type: "number", required: true, min: 1, precision: 0 },
  { name: "platformCode", label: "平台代码", required: true, placeholder: "binance / okx" },
  { name: "coinId", label: "币种ID", type: "number", required: true, min: 1, precision: 0 },
  { name: "coinCode", label: "币种代码", required: true, placeholder: "BTC / USDT" },
  { name: "platformSymbol", label: "平台符号", placeholder: "平台侧币种符号" },
  { name: "chainName", label: "链名称", placeholder: "ERC20 / TRC20" },
  { name: "contractAddress", label: "合约地址", placeholder: "Token 合约地址" },
  { name: "depositEnable", label: "充值", type: "select", options: enableOptions },
  { name: "withdrawEnable", label: "提现", type: "select", options: enableOptions },
  { name: "tradeEnable", label: "交易", type: "select", options: enableOptions },
  { name: "minWithdrawal", label: "最小提现", type: "number", min: 0, precision: 8 },
  { name: "withdrawalFee", label: "提现手续费", type: "number", min: 0, precision: 8 },
  { name: "confirmations", label: "确认数", type: "number", min: 0, precision: 0 },
];

const platformCoinColumns: CrudTableColumn<PlatformCoinRecord>[] = [
  { name: "platformCode", label: "平台", width: 110 },
  { name: "coinCode", label: "币种", width: 90, copyable: true },
  { name: "platformSymbol", label: "平台符号", width: 120 },
  { name: "chainName", label: "网络", width: 100 },
  { name: "depositEnable", label: "充值", width: 80, render: (v) => enableTag(v) },
  { name: "withdrawEnable", label: "提现", width: 80, render: (v) => enableTag(v) },
  { name: "tradeEnable", label: "交易", width: 80, render: (v) => enableTag(v) },
  { name: "minWithdrawal", label: "最小提现", width: 120, render: (v) => Number(v).toFixed(6) },
  { name: "withdrawalFee", label: "手续费", width: 110, render: (v) => Number(v).toFixed(6) },
  { name: "updatedTime", label: "更新时间", width: 160 },
];

const accountFields: CrudField<PlatformAccountRecord>[] = [
  { name: "platformId", label: "平台ID", type: "number", required: true, min: 1, precision: 0 },
  { name: "platformCode", label: "平台代码", required: true, placeholder: "binance / okx" },
  { name: "accountName", label: "账户名称", required: true, placeholder: "策略账户 / 资金账户" },
  { name: "accountType", label: "账户类型", type: "select", options: [
    { label: "主账户", value: "master" },
    { label: "子账户", value: "sub" },
    { label: "只读账户", value: "read_only" },
  ] },
  { name: "apiKey", label: "API Key", placeholder: "平台 API Key" },
  { name: "apiSecret", label: "API Secret", type: "password", placeholder: "创建或轮换时填写" },
  { name: "passphrase", label: "Passphrase", type: "password", placeholder: "OKX 等平台需要" },
  { name: "ipWhitelist", label: "IP白名单", placeholder: "多个 IP 用逗号分隔" },
  { name: "permissions", label: "权限", placeholder: "read,trade,withdraw" },
  { name: "status", label: "状态", type: "select", options: accountStatusOptions },
  { name: "remark", label: "备注", type: "textarea" },
];

const accountColumns: CrudTableColumn<PlatformAccountRecord>[] = [
  { name: "platformCode", label: "平台", width: 110 },
  { name: "accountName", label: "账户名称", width: 140 },
  { name: "accountType", label: "类型", width: 90 },
  { name: "apiKey", label: "API Key", width: 220, render: (value) => maskApiKey(value) },
  { name: "permissions", label: "权限", width: 170, render: (value) => renderCsvTags(value) },
  { name: "status", label: "状态", width: 90 },
  { name: "lastUsedTime", label: "最后使用", width: 160 },
  { name: "expireTime", label: "过期时间", width: 160 },
];

function PlatformSubPanel() {
  return (
    <CrudManagementPanel<PlatformRecord, PlatformPayload>
      title="平台"
      eyebrow="EXCHANGE ACCESS"
      description="维护交易平台主档、API 网关、市场能力和上线状态，让后续用户、币种、交易配置都能从同一套平台资产出发。"
      createText="新增平台"
      searchPlaceholder="搜索平台代码"
      searchParam="code"
      fields={fields}
      columns={columns}
      statusField="status"
      statusOptions={statusOptions}
      actionWidth={100}
      primaryMetricLabel="接入平台"
      insight={platformInsight}
      api={{
        list: (query) =>
          fetchPlatforms({
            pageIndex: query.pageIndex,
            pageSize: query.pageSize,
            code: query.code as string | undefined,
            status: query.status as string | undefined,
          }),
        create: (payload) => createPlatform(payload as PlatformPayload),
        update: (id, payload) => updatePlatform(id, payload as Partial<PlatformPayload>),
        remove: (id) => deletePlatform(id),
      }}
    />
  );
}

function PlatformCoinSubPanel() {
  return (
    <CrudManagementPanel<PlatformCoinRecord, PlatformCoinPayload>
      title="平台币种"
      eyebrow="ASSET MAPPING"
      description="维护不同交易平台上的币种符号、链网络、充值提现和交易能力，减少策略侧对平台差异的感知。"
      createText="新增平台币种"
      searchPlaceholder="按平台ID筛选"
      searchParam="platformId"
      fields={platformCoinFields}
      columns={platformCoinColumns}
      actionWidth={100}
      primaryMetricLabel="映射数量"
      insight={<><strong>映射策略</strong><span>平台符号、链网络和充提交易开关集中校验，适合批量接入新交易所后快速巡检。</span></>}
      filters={[
        { name: "coinCode", label: "币种代码", placeholder: "币种" },
        { name: "chainName", label: "网络", placeholder: "ERC20 / TRC20" },
      ]}
      api={{
        list: (query) => fetchPlatformCoins({
          pageIndex: query.pageIndex,
          pageSize: query.pageSize,
          platformId: query.platformId ? Number(query.platformId) : undefined,
          coinCode: query.coinCode as string | undefined,
          chainName: query.chainName as string | undefined,
        }),
        create: (payload) => upsertPlatformCoin(payload as PlatformCoinPayload),
        update: (id, payload) => updatePlatformCoin(id, payload as Partial<PlatformCoinPayload>),
        remove: (id) => deletePlatformCoin(id),
      }}
    />
  );
}

function PlatformAccountSubPanel() {
  return (
    <CrudManagementPanel<PlatformAccountRecord, PlatformAccountPayload>
      title="平台API账户"
      eyebrow="API CREDENTIALS"
      description="维护平台侧 API 账户、权限、白名单和生命周期。敏感字段仅在创建或轮换时填写。"
      createText="新增API账户"
      searchPlaceholder="按平台ID筛选"
      searchParam="platformId"
      fields={accountFields}
      columns={accountColumns}
      statusField="status"
      statusOptions={accountStatusOptions}
      actionWidth={100}
      primaryMetricLabel="API账户"
      insight={<><strong>密钥治理</strong><span>关注账户状态、权限范围和过期时间，优先处理禁用、过期或长期未使用账户。</span></>}
      filters={[
        { name: "accountName", label: "账户名称", placeholder: "账户名称" },
      ]}
      api={{
        list: (query) => fetchPlatformAccounts({
          pageIndex: query.pageIndex,
          pageSize: query.pageSize,
          platformId: query.platformId ? Number(query.platformId) : undefined,
          accountName: query.accountName as string | undefined,
          status: query.status as string | undefined,
        }),
        create: (payload) => createPlatformAccount(payload as PlatformAccountPayload),
        update: (id, payload) => updatePlatformAccount(id, payload as Partial<PlatformAccountPayload>),
        remove: (id) => deletePlatformAccount(id),
      }}
    />
  );
}

export default function PlatformPage() {
  return (
    <Tabs
      destroyInactiveTabPane
      items={[
        { key: "platforms", label: "平台管理", children: <PlatformSubPanel /> },
        { key: "coins", label: "平台币种", children: <PlatformCoinSubPanel /> },
        { key: "accounts", label: "API账户", children: <PlatformAccountSubPanel /> },
      ]}
    />
  );
}
