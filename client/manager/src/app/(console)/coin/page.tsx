"use client";

import { useState } from "react";
import { Tabs, Tag } from "antd";
import { CrudManagementPanel } from "../components/CrudManagementPanel";
import type { CrudField, CrudTableColumn } from "../components/CrudManagementPanel";
import {
  fetchCoins,
  createCoin,
  updateCoin,
  deleteCoin,
  fetchCoinPairs,
  createCoinPair,
  updateCoinPair,
  deleteCoinPair,
  type CoinRecord,
  type CoinPayload,
  type CoinPairRecord,
  type CoinPairPayload,
} from "./api/coin.api";

const statusOptions = [
  { label: "上线", value: "online" },
  { label: "下线", value: "offline" },
  { label: "维护", value: "maintenance" },
];

const enableOptions = [
  { label: "启用", value: 1 },
  { label: "禁用", value: 0 },
];

const assetInsight = (
  <>
    <strong>资产目录</strong>
    <span>币种、链网络、充提交易能力和精度统一维护，降低平台接入和策略下单时的字段漂移。</span>
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

const coinFields: CrudField<CoinRecord>[] = [
  { name: "code", label: "币种代码", required: true, placeholder: "如 BTC / ETH / USDT", disabledOnEdit: true },
  { name: "name", label: "名称", required: true, placeholder: "如 Bitcoin" },
  { name: "fullName", label: "全称", placeholder: "如 Bitcoin" },
  { name: "chainName", label: "主链", placeholder: "如 BTC / ERC20 / TRC20" },
  { name: "contractAddress", label: "合约地址", placeholder: "Token 合约地址" },
  { name: "decimals", label: "小数位", type: "number", min: 0, precision: 0, placeholder: "8" },
  { name: "pricePrecision", label: "价格精度", type: "number", min: 0, precision: 0, placeholder: "8" },
  { name: "amountPrecision", label: "数量精度", type: "number", min: 0, precision: 0, placeholder: "8" },
  { name: "minWithdrawal", label: "最小提现", type: "number", min: 0, precision: 8, placeholder: "0" },
  { name: "maxWithdrawal", label: "最大提现", type: "number", min: 0, precision: 8, placeholder: "0" },
  { name: "withdrawalFee", label: "提现手续费", type: "number", min: 0, precision: 8, placeholder: "0" },
  { name: "tradeEnable", label: "允许交易", type: "select", options: enableOptions },
  { name: "depositEnable", label: "允许充值", type: "select", options: enableOptions },
  { name: "withdrawEnable", label: "允许提现", type: "select", options: enableOptions },
  { name: "status", label: "状态", type: "select", options: statusOptions },
  { name: "sortOrder", label: "排序", type: "number", min: 0, precision: 0, placeholder: "0" },
  { name: "description", label: "描述", type: "textarea", placeholder: "币种说明" },
];

const coinColumns: CrudTableColumn<CoinRecord>[] = [
  {
    name: "code",
    label: "资产",
    width: 170,
    render: (value, record) => (
      <div style={{ display: "flex", flexDirection: "column", gap: 4 }}>
        <span style={{ fontWeight: 800 }}>{String(value).toUpperCase()}</span>
        <span style={{ color: "var(--manager-text-faint)", fontSize: 12 }}>{record.fullName || record.name || "-"}</span>
      </div>
    ),
  },
  { name: "chainName", label: "网络", width: 100 },
  { name: "status", label: "状态", width: 90, render: (v) => statusTag(v) },
  { name: "tradeEnable", label: "交易", width: 80, render: (v) => enableTag(v) },
  { name: "depositEnable", label: "充值", width: 80, render: (v) => enableTag(v) },
  { name: "withdrawEnable", label: "提现", width: 80, render: (v) => enableTag(v) },
  {
    name: "decimals",
    label: "精度",
    width: 150,
    render: (_, record) => `${record.decimals}/${record.pricePrecision}/${record.amountPrecision}`,
  },
  { name: "sortOrder", label: "排序", width: 80 },
  { name: "updatedTime", label: "更新时间", width: 160 },
];

const pairFields: CrudField<CoinPairRecord>[] = [
  { name: "symbol", label: "交易对", required: true, placeholder: "如 BTC-USDT", disabledOnEdit: true },
  { name: "baseCoinCode", label: "基础币种", required: true, placeholder: "如 BTC" },
  { name: "quoteCoinCode", label: "计价币种", required: true, placeholder: "如 USDT" },
  { name: "pricePrecision", label: "价格精度", type: "number", min: 0, precision: 0, placeholder: "8" },
  { name: "amountPrecision", label: "数量精度", type: "number", min: 0, precision: 0, placeholder: "8" },
  { name: "minAmount", label: "最小下单量", type: "number", min: 0, precision: 8, placeholder: "0" },
  { name: "maxAmount", label: "最大下单量", type: "number", min: 0, precision: 8, placeholder: "0" },
  { name: "minTotal", label: "最小成交额", type: "number", min: 0, precision: 8, placeholder: "0" },
  { name: "takerFeeRate", label: "吃单费率", type: "number", min: 0, precision: 6, placeholder: "0.001" },
  { name: "makerFeeRate", label: "挂单费率", type: "number", min: 0, precision: 6, placeholder: "0.001" },
  { name: "status", label: "状态", type: "select", options: statusOptions },
  { name: "sortOrder", label: "排序", type: "number", min: 0, precision: 0, placeholder: "0" },
];

const pairColumns: CrudTableColumn<CoinPairRecord>[] = [
  {
    name: "symbol",
    label: "市场",
    width: 170,
    render: (value, record) => (
      <div style={{ display: "flex", flexDirection: "column", gap: 4 }}>
        <span style={{ fontWeight: 800 }}>{String(value).toUpperCase()}</span>
        <span style={{ color: "var(--manager-text-faint)", fontSize: 12 }}>{record.baseCoinCode}/{record.quoteCoinCode}</span>
      </div>
    ),
  },
  { name: "status", label: "状态", width: 90, render: (v) => statusTag(v) },
  {
    name: "takerFeeRate",
    label: "吃单费率",
    width: 100,
    render: (v) => (v != null ? `${(Number(v) * 100).toFixed(4)}%` : "-"),
  },
  {
    name: "makerFeeRate",
    label: "挂单费率",
    width: 100,
    render: (v) => (v != null ? `${(Number(v) * 100).toFixed(4)}%` : "-"),
  },
  { name: "pricePrecision", label: "价格精度", width: 90 },
  { name: "amountPrecision", label: "数量精度", width: 90 },
  { name: "sortOrder", label: "排序", width: 80 },
  { name: "updatedTime", label: "更新时间", width: 160 },
];

const CoinSubPanel = () => (
  <CrudManagementPanel<CoinRecord, CoinPayload>
    title="币种"
    eyebrow="ASSET CATALOG"
    description="维护币种主档、链网络、精度和充提交易能力。运营侧可以快速切换状态，策略侧可以依赖一致的资产目录。"
    createText="新增币种"
    searchPlaceholder="搜索币种代码"
    searchParam="code"
    fields={coinFields}
    columns={coinColumns}
    statusField="status"
    statusOptions={statusOptions}
    actionWidth={100}
    primaryMetricLabel="币种数量"
    insight={assetInsight}
    filters={[
      { name: "chainName", label: "网络", placeholder: "BTC / ERC20 / TRC20" },
    ]}
    api={{
      list: (query) =>
        fetchCoins({
          pageIndex: query.pageIndex,
          pageSize: query.pageSize,
          code: query.code as string | undefined,
          chainName: query.chainName as string | undefined,
          status: query.status as string | undefined,
        }),
      create: (payload) => createCoin(payload as CoinPayload),
      update: (id, payload) => updateCoin(id, payload as Partial<CoinPayload>),
      remove: (id) => deleteCoin(id),
    }}
  />
);

const PairSubPanel = () => (
  <CrudManagementPanel<CoinPairRecord, CoinPairPayload>
    title="交易对"
    eyebrow="MARKET CATALOG"
    description="维护基础币、计价币、下单范围、精度和 maker/taker 费率，作为交易与风控策略共享的市场配置。"
    createText="新增交易对"
    searchPlaceholder="搜索交易对符号"
    searchParam="symbol"
    fields={pairFields}
    columns={pairColumns}
    statusField="status"
    statusOptions={statusOptions}
    actionWidth={100}
    primaryMetricLabel="市场数量"
    insight={<><strong>市场配置</strong><span>交易对上线前请确认价格精度、数量精度、最小成交额和双边费率已经完成校验。</span></>}
    filters={[
      { name: "baseCoinCode", label: "基础币", placeholder: "BTC" },
      { name: "quoteCoinCode", label: "计价币", placeholder: "USDT" },
    ]}
    api={{
      list: (query) =>
        fetchCoinPairs({
          pageIndex: query.pageIndex,
          pageSize: query.pageSize,
          symbol: query.symbol as string | undefined,
          baseCoinCode: query.baseCoinCode as string | undefined,
          quoteCoinCode: query.quoteCoinCode as string | undefined,
          status: query.status as string | undefined,
        }),
      create: (payload) => createCoinPair(payload as CoinPairPayload),
      update: (id, payload) => updateCoinPair(id, payload as Partial<CoinPairPayload>),
      remove: (id) => deleteCoinPair(id),
    }}
  />
);

export default function CoinPage() {
  return (
    <Tabs
      destroyInactiveTabPane
      items={[
        { key: "coins", label: "币种管理", children: <CoinSubPanel /> },
        { key: "pairs", label: "交易对管理", children: <PairSubPanel /> },
      ]}
    />
  );
}
