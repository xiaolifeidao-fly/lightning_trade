"use client";

import {
  AppstoreOutlined,
  BarChartOutlined,
  BellOutlined,
  CompassOutlined,
  GlobalOutlined,
  LogoutOutlined,
  SafetyCertificateOutlined,
  TeamOutlined,
  WalletOutlined,
} from "@ant-design/icons";
import { Avatar, Badge, Button, Layout, Menu, Space, Tag, Typography } from "antd";
import type { MenuProps } from "antd";
import { usePathname, useRouter } from "next/navigation";
import { PropsWithChildren, useEffect, useMemo, useState } from "react";
import { clearAuthToken } from "@/utils/auth";

const { Content, Header, Sider } = Layout;
const { Text } = Typography;

interface ManagerShellProps extends PropsWithChildren {}

type MenuItem = Required<MenuProps>["items"][number];

const pageTitleMap: Record<string, string> = {
  "/manager-dashboard": "数据总览",
  "/user": "用户管理",
  "/permission": "角色管理",
  "/platform": "平台管理",
  "/coin": "币种管理",
  "/coin-user": "币用户管理",
  "/trade-orders": "真实交易",
  "/trade-simulation-analysis": "模拟盘分析",
  "/trade-strategy-backtest": "策略回测",
};

function getOpenKeys(pathname: string) {
  if (pathname.startsWith("/user") || pathname.startsWith("/permission")) {
    return ["/system-group"];
  }
  if (pathname.startsWith("/platform") || pathname.startsWith("/coin")) {
    return ["/exchange-group"];
  }
  if (
    pathname.startsWith("/trade-orders") ||
    pathname.startsWith("/trade-simulation-analysis") ||
    pathname.startsWith("/trade-strategy-backtest")
  ) {
    return ["/trade-group"];
  }
  return [];
}

export function ManagerShell({ children }: ManagerShellProps) {
  const pathname = usePathname();
  const router = useRouter();
  const activePath = pathname ?? "/manager-dashboard";
  const [openKeys, setOpenKeys] = useState<string[]>(() => getOpenKeys(activePath));
  const quickActions = useMemo(
    () => [
      {
        key: "/manager-dashboard",
        label: "总览",
        icon: <AppstoreOutlined />,
      },
      {
        key: "/coin-user",
        label: "币用户",
        icon: <WalletOutlined />,
      },
      {
        key: "/trade-orders",
        label: "真实交易",
        icon: <BarChartOutlined />,
      },
      {
        key: "/platform",
        label: "平台",
        icon: <GlobalOutlined />,
      },
      {
        key: "/coin",
        label: "币种",
        icon: <GlobalOutlined />,
      },
    ],
    [],
  );
  const items = useMemo<MenuItem[]>(
    () => [
      {
        key: "/manager-dashboard",
        icon: <AppstoreOutlined />,
        label: "数据总览",
      },
      {
        key: "/coin-user",
        icon: <WalletOutlined />,
        label: "币用户管理",
      },
      {
        key: "/trade-group",
        icon: <BarChartOutlined />,
        label: "交易管理",
        children: [
          {
            key: "/trade-orders",
            label: "真实交易",
          },
          {
            key: "/trade-simulation-analysis",
            label: "模拟盘分析",
          },
          {
            key: "/trade-strategy-backtest",
            label: "策略回测",
          },
        ],
      },
      {
        key: "/exchange-group",
        icon: <GlobalOutlined />,
        label: "交易所管理",
        children: [
          {
            key: "/platform",
            label: "平台管理",
          },
          {
            key: "/coin",
            label: "币种管理",
          },
        ],
      },
      {
        key: "/system-group",
        icon: <SafetyCertificateOutlined />,
        label: "系统设置",
        children: [
          {
            key: "/user",
            icon: <TeamOutlined />,
            label: "用户管理",
          },
          {
            key: "/permission",
            label: "角色管理",
          },
        ],
      },
    ],
    [],
  );
  const selectedKey = activePath;

  useEffect(() => {
    const pathOpenKeys = getOpenKeys(activePath);
    if (pathOpenKeys.length === 0) {
      return;
    }
    setOpenKeys((currentKeys) => Array.from(new Set([...currentKeys, ...pathOpenKeys])));
  }, [activePath]);

  const handleLogout = () => {
    clearAuthToken();
    router.replace("/login");
  };

  const pageTitle =
    Object.entries(pageTitleMap).find(([path]) => activePath.startsWith(path))?.[1] ??
    "管理工作台";

  return (
    <div className="manager-app-frame">
      <div className="manager-shell-surface">
        <Layout
          style={{
            minHeight: "100vh",
            background: "transparent",
          }}
        >
          <Sider
            width={248}
            style={{
              background: "transparent",
            }}
          >
            <div
              className="manager-sidebar-card manager-stagger-1"
              style={{
                height: "100%",
                padding: "24px 16px",
                display: "flex",
                flexDirection: "column",
                gap: 18,
              }}
            >
              <div>
                <div className="manager-brand-kicker">Lightning Trade</div>
                <Space align="start" size={12} style={{ marginTop: 18 }}>
                  <div className="manager-crest" />
                  <div className="manager-wordmark">
                    <strong style={{ color: "#fff" }}>闪电量化</strong>
                    <span style={{ color: "rgba(255,255,255,0.66)" }}>Crypto Futures Console</span>
                  </div>
                </Space>
              </div>

              <Menu
                className="manager-shell-menu"
                mode="inline"
                selectedKeys={[selectedKey]}
                openKeys={openKeys}
                onOpenChange={(keys) => setOpenKeys(keys as string[])}
                items={items}
                onClick={({ key }) => {
                  if (typeof key === "string" && key.startsWith("/")) {
                    router.push(key);
                  }
                }}
                style={{
                  fontSize: 15,
                  marginTop: 8,
                }}
              />
              <div className="manager-sidebar-foot">
                <span>当前模式</span>
                <strong>实盘 · 合约交易</strong>
                <Tag bordered={false}>已加密 · 多签授权</Tag>
              </div>
            </div>
          </Sider>

          <Layout style={{ background: "transparent" }}>
            <Header
              className="manager-stagger-2"
              style={{
                height: "auto",
                lineHeight: "normal",
                padding: 0,
                background: "transparent",
              }}
            >
              <div
                className="manager-shell-card"
                style={{
                  borderRadius: 0,
                  padding: "0 28px 0 30px",
                  minHeight: 76,
                  display: "grid",
                  gridTemplateColumns: "minmax(0, 1fr) auto",
                  gap: 20,
                  alignItems: "center",
                }}
              >
                <div style={{ minWidth: 0 }}>
                  <Space size={10} align="center" style={{ marginBottom: 8 }}>
                    <CompassOutlined style={{ color: "var(--manager-primary)" }} />
                    <Text style={{ color: "var(--manager-text-soft)", fontWeight: 700 }}>
                      {pageTitle}
                    </Text>
                  </Space>
                  <Space size={10} wrap style={{ width: "100%" }}>
                    {quickActions.map((action) => {
                      const isActive = activePath === action.key;

                      return (
                        <Button
                          key={action.key}
                          type={isActive ? "primary" : "default"}
                          icon={action.icon}
                          className={isActive ? "manager-soft-button" : undefined}
                          onClick={() => router.push(action.key)}
                          style={{
                            height: 38,
                            paddingInline: 14,
                            borderRadius: 8,
                            fontWeight: 700,
                          }}
                        >
                          {action.label}
                        </Button>
                      );
                    })}
                  </Space>
                </div>

                <Space size={12} wrap>
                  <Badge dot offset={[-2, 2]}>
                    <div
                      className="manager-icon-button"
                      style={{
                        width: 46,
                        height: 46,
                      }}
                    >
                      <BellOutlined style={{ color: "var(--manager-text-soft)", fontSize: 18 }} />
                    </div>
                  </Badge>
                  <div
                    style={{
                      padding: "8px 12px 8px 8px",
                      borderRadius: 10,
                      border: "1px solid var(--manager-border)",
                      background: "#1E2329",
                    }}
                  >
                    <Space size={12}>
                      <Avatar
                        style={{
                          width: 38,
                          height: 38,
                          background: "linear-gradient(135deg, #FCD535, #F0B90B)",
                          color: "#0B0E11",
                          fontWeight: 800,
                        }}
                      >
                        A
                      </Avatar>
                      <div>
                        <div style={{ fontWeight: 700, color: "var(--manager-text)" }}>Admin</div>
                        <Text style={{ color: "var(--manager-text-soft)" }}>超级管理员</Text>
                      </div>
                      <Button
                        type="text"
                        onClick={handleLogout}
                        icon={<LogoutOutlined />}
                        style={{
                          color: "var(--manager-text-soft)",
                          fontWeight: 600,
                        }}
                      >
                        退出
                      </Button>
                    </Space>
                  </div>
                </Space>
              </div>
            </Header>

            <Content style={{ padding: "22px 28px 40px" }}>
              <div className="manager-stagger-3">{children}</div>
            </Content>
          </Layout>
        </Layout>
      </div>
    </div>
  );
}
