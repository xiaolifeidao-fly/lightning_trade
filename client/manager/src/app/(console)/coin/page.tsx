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
  { name: "code", label: "代码", width: 100, copyable: true },
  { name: "name", label: "名称", width: 120 },
  { name: "chainName", label: "主链", width: 100 },
  { name: "status", label: "状态", width: 90, render: (v) => statusTag(v) },
  { name: "tradeEnable", label: "交易", width: 80, render: (v) => enableTag(v) },
  { name: "depositEnable", label: "充值", width: 80, render: (v) => enableTag(v) },
  { name: "withdrawEnable", label: "提现", width: 80, render: (v) => enableTag(v) },
  { name: "decimals", label: "精度", width: 80 },
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
  { name: "symbol", label: "交易对", width: 130, copyable: true },
  { name: "baseCoinCode", label: "基础币", width: 100 },
  { name: "quoteCoinCode", label: "计价币", width: 100 },
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
    createText="新增币种"
    searchPlaceholder="搜索币种代码或名称"
    searchParam="code"
    fields={coinFields}
    columns={coinColumns}
    statusField="status"
    statusOptions={statusOptions}
    actionWidth={100}
    api={{
      list: (query) =>
        fetchCoins({
          pageIndex: query.pageIndex,
          pageSize: query.pageSize,
          code: query.code as string | undefined,
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
    createText="新增交易对"
    searchPlaceholder="搜索交易对符号"
    searchParam="symbol"
    fields={pairFields}
    columns={pairColumns}
    statusField="status"
    statusOptions={statusOptions}
    actionWidth={100}
    api={{
      list: (query) =>
        fetchCoinPairs({
          pageIndex: query.pageIndex,
          pageSize: query.pageSize,
          symbol: query.symbol as string | undefined,
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
