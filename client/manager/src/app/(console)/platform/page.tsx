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

function statusTag(value: unknown) {
  const v = String(value ?? "").toLowerCase();
  const color = v === "online" ? "green" : v === "offline" ? "red" : "orange";
  const label = v === "online" ? "上线" : v === "offline" ? "下线" : "维护";
  return <Tag color={color}>{label}</Tag>;
}

function enableTag(value: unknown) {
  return Number(value) === 1 ? <Tag color="green">启用</Tag> : <Tag color="default">禁用</Tag>;
}

const fields: CrudField<PlatformRecord>[] = [
  { name: "code", label: "平台代码", required: true, placeholder: "如 binance / okx", disabledOnEdit: true },
  { name: "name", label: "平台名称", required: true, placeholder: "如 Binance" },
  { name: "fullName", label: "完整名称", placeholder: "如 Binance Exchange" },
  { name: "country", label: "国家/地区", placeholder: "如 Cayman Islands" },
  { name: "website", label: "官网", placeholder: "https://..." },
  { name: "apiBaseUrl", label: "API Base URL", placeholder: "https://api.binance.com" },
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
  { name: "code", label: "代码", width: 120, copyable: true },
  { name: "name", label: "平台名称", width: 140 },
  { name: "country", label: "国家", width: 120 },
  {
    name: "status",
    label: "状态",
    width: 100,
    render: (value) => statusTag(value),
  },
  { name: "website", label: "官网", width: 200 },
  {
    name: "defaultFeeRate",
    label: "费率",
    width: 100,
    render: (value) => (value != null && value !== "" ? `${Number(value).toFixed(4)}` : "-"),
  },
  { name: "supportedTypes", label: "支持类型", width: 160 },
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
  { name: "chainName", label: "链", width: 100 },
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
    { label: "现货", value: "spot" },
    { label: "合约", value: "futures" },
    { label: "资金", value: "funding" },
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
  { name: "apiKey", label: "API Key", width: 220, copyable: true },
  { name: "permissions", label: "权限", width: 160 },
  { name: "status", label: "状态", width: 90 },
  { name: "lastUsedTime", label: "最后使用", width: 160 },
  { name: "expireTime", label: "过期时间", width: 160 },
];

function PlatformSubPanel() {
  return (
    <CrudManagementPanel<PlatformRecord, PlatformPayload>
      title="平台"
      createText="新增平台"
      searchPlaceholder="搜索平台代码或名称"
      searchParam="name"
      fields={fields}
      columns={columns}
      statusField="status"
      statusOptions={statusOptions}
      actionWidth={100}
      api={{
        list: (query) =>
          fetchPlatforms({
            pageIndex: query.pageIndex,
            pageSize: query.pageSize,
            name: query.name as string | undefined,
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
      createText="新增平台币种"
      searchPlaceholder="按平台ID筛选"
      searchParam="platformId"
      fields={platformCoinFields}
      columns={platformCoinColumns}
      actionWidth={100}
      filters={[
        { name: "coinCode", label: "币种代码", placeholder: "币种" },
      ]}
      api={{
        list: async (query) => {
          const rows = await fetchPlatformCoins(query.platformId ? Number(query.platformId) : undefined);
          const coinCode = String(query.coinCode ?? "").trim().toUpperCase();
          const filtered = coinCode ? rows.filter((item) => item.coinCode.toUpperCase().includes(coinCode)) : rows;
          return { total: filtered.length, data: filtered };
        },
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
      createText="新增API账户"
      searchPlaceholder="按平台ID筛选"
      searchParam="platformId"
      fields={accountFields}
      columns={accountColumns}
      statusField="status"
      statusOptions={accountStatusOptions}
      actionWidth={100}
      filters={[
        { name: "accountName", label: "账户名称", placeholder: "账户名称" },
      ]}
      api={{
        list: async (query) => {
          const rows = await fetchPlatformAccounts(query.platformId ? Number(query.platformId) : undefined);
          const accountName = String(query.accountName ?? "").trim();
          const status = String(query.status ?? "").trim();
          const filtered = rows.filter((item) => {
            const matchedName = accountName ? item.accountName.includes(accountName) : true;
            const matchedStatus = status ? item.status === status : true;
            return matchedName && matchedStatus;
          });
          return { total: filtered.length, data: filtered };
        },
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
