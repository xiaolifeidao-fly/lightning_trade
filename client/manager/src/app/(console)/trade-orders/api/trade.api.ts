"use client";

import { getPage } from "@/utils/axios";

export class TradeOrderRecord {
  id!: number;
  platformCode = "";
  tradeCategory = "";
  tradeType = "";
  orderNo = "";
  userId = 0;
  symbol = "";
  baseCoinCode = "";
  quoteCoinCode = "";
  side = "";
  orderType = "";
  price = 0;
  amount = 0;
  total = 0;
  stopPrice = 0;
  filledAmount = 0;
  filledTotal = 0;
  avgFilledPrice = 0;
  feeCoinCode = "";
  feeAmount = 0;
  status = "";
  timeInForce = "";
  source = "";
  clientOrderId = "";
  submittedTime?: string;
  finishedTime?: string;
  cancelReason = "";
  createdTime?: string;
}

export class TradeDetailRecord {
  id!: number;
  platformCode = "";
  tradeCategory = "";
  tradeType = "";
  userId = 0;
  orderNo = "";
  tradeNo = "";
  symbol = "";
  coinCode = "";
  side = "";
  openDirection = "";
  avgOpenPrice = 0;
  liquidationPrice = 0;
  leverage = 0;
  margin = 0;
  userBalanceOpen = 0;
  price = 0;
  amount = 0;
  total = 0;
  fee = 0;
  pnl = 0;
  pnlRate = 0;
  tradeTime?: string;
  createdTime?: string;
}

export class TradeUserSummaryRecord {
  id!: number;
  userId = 0;
  platformCode = "";
  coinCode = "";
  tradeCategory = "";
  tradeDate = "";
  totalOrders = 0;
  buyOrders = 0;
  sellOrders = 0;
  buyAmount = 0;
  sellAmount = 0;
  buyTotal = 0;
  sellTotal = 0;
  totalFee = 0;
  totalVolume = 0;
  createdTime?: string;
}

export class TradeUserPnlRecord {
  id!: number;
  userId = 0;
  platformCode = "";
  coinCode = "";
  tradeCategory = "";
  tradeDate = "";
  realizedPnl = 0;
  unrealizedPnl = 0;
  totalPnl = 0;
  pnlRate = 0;
  positionAmount = 0;
  positionCost = 0;
  positionValue = 0;
  createdTime?: string;
}

export interface TradeOrderQuery {
  pageIndex?: number;
  pageSize?: number;
  userId?: number;
  symbol?: string;
  side?: string;
  status?: string;
  tradeCategory?: string;
  orderNo?: string;
}

export interface TradeDetailQuery {
  pageIndex?: number;
  pageSize?: number;
  userId?: number;
  symbol?: string;
  orderNo?: string;
  tradeCategory?: string;
}

export interface TradeUserSummaryQuery {
  pageIndex?: number;
  pageSize?: number;
  userId?: number;
  platformCode?: string;
  coinCode?: string;
  tradeCategory?: string;
  startDate?: string;
  endDate?: string;
}

export interface TradeUserPnlQuery {
  pageIndex?: number;
  pageSize?: number;
  userId?: number;
  platformCode?: string;
  coinCode?: string;
  tradeCategory?: string;
  startDate?: string;
  endDate?: string;
}

export async function fetchTradeOrders(query: TradeOrderQuery) {
  return getPage(TradeOrderRecord, "/trade-orders", {
    pageIndex: query.pageIndex,
    pageSize: query.pageSize,
    userId: query.userId,
    symbol: query.symbol,
    side: query.side,
    status: query.status,
    tradeCategory: query.tradeCategory,
    orderNo: query.orderNo,
  });
}

export async function fetchTradeDetails(query: TradeDetailQuery) {
  return getPage(TradeDetailRecord, "/trade-details", {
    pageIndex: query.pageIndex,
    pageSize: query.pageSize,
    userId: query.userId,
    symbol: query.symbol,
    orderNo: query.orderNo,
    tradeCategory: query.tradeCategory,
  });
}

export async function fetchUserSummary(query: TradeUserSummaryQuery) {
  return getPage(TradeUserSummaryRecord, "/trade-user-summary", {
    pageIndex: query.pageIndex,
    pageSize: query.pageSize,
    userId: query.userId,
    platformCode: query.platformCode,
    coinCode: query.coinCode,
    tradeCategory: query.tradeCategory,
    startDate: query.startDate,
    endDate: query.endDate,
  });
}

export async function fetchUserPnl(query: TradeUserPnlQuery) {
  return getPage(TradeUserPnlRecord, "/trade-user-pnl", {
    pageIndex: query.pageIndex,
    pageSize: query.pageSize,
    userId: query.userId,
    platformCode: query.platformCode,
    coinCode: query.coinCode,
    tradeCategory: query.tradeCategory,
    startDate: query.startDate,
    endDate: query.endDate,
  });
}
