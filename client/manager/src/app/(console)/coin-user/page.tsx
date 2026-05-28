"use client";

import { useState } from "react";
import { Button, Tag, Tooltip } from "antd";
import { EyeOutlined } from "@ant-design/icons";
import { CrudManagementPanel } from "../components/CrudManagementPanel";
import type { CrudField, CrudTableColumn } from "../components/CrudManagementPanel";
import {
  fetchCoinUsers,
  createCoinUser,
  updateCoinUser,
  deleteCoinUser,
  type CoinUserRecord,
  type CoinUserCreatePayload,
  type CoinUserUpdatePayload,
} from "./api/coin-user.api";
import { CoinUserDetailDrawer } from "./components/CoinUserDetailDrawer";

type CoinUserPayload = CoinUserCreatePayload & CoinUserUpdatePayload;

const kycStatusOptions = [
  { label: "待审核", value: "pending" },
  { label: "已认证", value: "approved" },
  { label: "已拒绝", value: "rejected" },
];

const userStatusOptions = [
  { label: "正常", value: "active" },
  { label: "锁定", value: "locked" },
  { label: "冻结", value: "frozen" },
];

const twoFaOptions = [
  { label: "未开启", value: 0 },
  { label: "已开启", value: 1 },
];

function kycTag(value: unknown) {
  const v = String(value ?? "");
  const color = v === "approved" ? "green" : v === "rejected" ? "red" : "orange";
  const label = v === "approved" ? "已认证" : v === "rejected" ? "已拒绝" : "待审核";
  return <Tag color={color}>{label}</Tag>;
}

function userStatusTag(value: unknown) {
  const v = String(value ?? "");
  const color = v === "active" ? "green" : v === "locked" ? "orange" : "red";
  const label = v === "active" ? "正常" : v === "locked" ? "锁定" : "冻结";
  return <Tag color={color}>{label}</Tag>;
}

const fields: CrudField<CoinUserRecord>[] = [
  { name: "username", label: "用户名", required: true, placeholder: "登录用户名", disabledOnEdit: true },
  { name: "nickname", label: "昵称", placeholder: "显示昵称" },
  { name: "email", label: "邮箱", placeholder: "邮箱地址" },
  { name: "phone", label: "手机号", placeholder: "手机号码" },
  { name: "password", label: "密码", type: "password", placeholder: "初始密码", hiddenOnEdit: true },
  { name: "platformCode", label: "平台代码", placeholder: "如 binance / okx" },
  { name: "balance", label: "余额(USDT)", type: "number", min: 0, precision: 4, placeholder: "0" },
  { name: "country", label: "国家", placeholder: "如 CN / US" },
  { name: "inviteCode", label: "邀请码", placeholder: "邀请码", hiddenOnEdit: true },
  { name: "kycStatus", label: "KYC状态", type: "select", options: kycStatusOptions, hiddenOnCreate: true },
  { name: "status", label: "账户状态", type: "select", options: userStatusOptions, hiddenOnCreate: true },
  { name: "twoFaEnabled", label: "2FA", type: "select", options: twoFaOptions, hiddenOnCreate: true },
  { name: "remark", label: "备注", type: "textarea", placeholder: "备注信息" },
];

const columns: CrudTableColumn<CoinUserRecord>[] = [
  { name: "username", label: "用户名", width: 130 },
  { name: "nickname", label: "昵称", width: 110 },
  { name: "email", label: "邮箱", width: 180 },
  { name: "phone", label: "手机", width: 130 },
  { name: "platformCode", label: "平台", width: 100 },
  {
    name: "balance",
    label: "余额",
    width: 110,
    render: (v) => (
      <strong style={{ color: "var(--manager-primary)" }}>{Number(v).toFixed(2)}</strong>
    ),
  },
  { name: "kycStatus", label: "KYC", width: 90, render: (v) => kycTag(v) },
  { name: "status", label: "状态", width: 90, render: (v) => userStatusTag(v) },
  { name: "lastLoginIp", label: "最后登录IP", width: 130 },
  { name: "createdTime", label: "注册时间", width: 160 },
];

export default function CoinUserPage() {
  const [detailUser, setDetailUser] = useState<CoinUserRecord | null>(null);
  const [drawerOpen, setDrawerOpen] = useState(false);

  return (
    <>
      <CrudManagementPanel<CoinUserRecord, CoinUserPayload>
        title="币用户"
        createText="新增用户"
        searchPlaceholder="搜索用户名/邮箱/手机"
        searchParam="search"
        fields={fields}
        columns={columns}
        statusField="status"
        statusOptions={userStatusOptions}
        filters={[
          { name: "platformCode", label: "平台代码", placeholder: "平台代码" },
          { name: "kycStatus", label: "KYC状态", type: "select", options: kycStatusOptions, placeholder: "KYC状态" },
        ]}
        actionWidth={152}
        rowActions={(record, _ctx) => (
          <Tooltip title="查看详情">
            <Button
              type="text"
              icon={<EyeOutlined />}
              onClick={() => {
                setDetailUser(record);
                setDrawerOpen(true);
              }}
            />
          </Tooltip>
        )}
        api={{
          list: (query) =>
            fetchCoinUsers({
              pageIndex: query.pageIndex,
              pageSize: query.pageSize,
              search: query.search as string | undefined,
              platformCode: query.platformCode as string | undefined,
              kycStatus: query.kycStatus as string | undefined,
              status: query.status as string | undefined,
            }),
          create: (payload) => createCoinUser(payload as CoinUserCreatePayload),
          update: (id, payload) => updateCoinUser(id, payload as Partial<CoinUserUpdatePayload>),
          remove: (id) => deleteCoinUser(id),
        }}
      />

      <CoinUserDetailDrawer
        user={detailUser}
        open={drawerOpen}
        onClose={() => {
          setDrawerOpen(false);
          setDetailUser(null);
        }}
      />
    </>
  );
}
