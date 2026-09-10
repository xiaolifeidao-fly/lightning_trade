"use client";

import {
  AppstoreOutlined,
  BarChartOutlined,
  BellOutlined,
  CompassOutlined,
  ControlOutlined,
  DashboardOutlined,
  ExperimentOutlined,
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
import { ArgusInstanceSelector } from "@/components/argus/ArgusInstanceSelector";
import { clearAuthToken } from "@/utils/auth";

const { Content, Header, Sider } = Layout;
const { Text } = Typography;

interface ManagerShellProps extends PropsWithChildren {}

type MenuItem = Required<MenuProps>["items"][number];

/**
 * Argus 七个页面的路由表。allowAll 决定顶栏的全局实例选择器给不给「全部实例」：
 * 参数编辑与行情主视图必须落到具体实例，前者一次发布会波及多个实例、毁掉
 * champion/challenger 对照组，后者的阈值线与净持仓阶梯跨实例混排读不出结论。
 */
const argusRoutes: { path: string; title: string; allowAll: boolean }[] = [
  { path: "/argus-dashboard", title: "Argus 总览", allowAll: true },
  { path: "/argus-instances", title: "实例与参数对比", allowAll: true },
  { path: "/argus-market", title: "历史行情与触发点", allowAll: false },
  { path: "/argus-signals", title: "信号复盘与持仓生命周期", allowAll: true },
  { path: "/argus-backtest", title: "盘口信号回测", allowAll: false },
  { path: "/argus-optimizer", title: "自动参数寻优", allowAll: false },
  { path: "/argus-config", title: "Argus 参数与运行控制", allowAll: false },
];

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
  "/trade-strategy": "策略管理",
  "/trade-backtest-runs": "回测对比",
  ...Object.fromEntries(argusRoutes.map((route) => [route.path, route.title])),
};

function matchArgusRoute(pathname: string) {
  return argusRoutes.find((route) => pathname.startsWith(route.path)) ?? null;
}

/**
 * 顶栏实例选择器出现的路由。除 Argus 七页之外，数据总览也吃同一份作用域——
 * 它每一个数字都来自 argus_instance / strategy_event / balance_sample，
 * 多实例上线后「权益合计」若不能落到单个部署单元，出了问题没法追到是谁。
 *
 * 它不并进 argusRoutes：那张表还兼着菜单分组与页面标题，把数据总览塞进去，
 * 侧边栏会在打开工作台时展开 Argus 分组。
 */
const scopeRoutes: { path: string; allowAll: boolean }[] = [
  { path: "/manager-dashboard", allowAll: true },
  ...argusRoutes.map(({ path, allowAll }) => ({ path, allowAll })),
];

function matchScopeRoute(pathname: string) {
  return scopeRoutes.find((route) => pathname.startsWith(route.path)) ?? null;
}

function getOpenKeys(pathname: string) {
  if (pathname.startsWith("/user") || pathname.startsWith("/permission")) {
    return ["/system-group"];
  }
  if (pathname.startsWith("/platform") || pathname.startsWith("/coin")) {
    return ["/exchange-group"];
  }
  if (matchArgusRoute(pathname)) {
    return ["/argus-group"];
  }
  if (
    pathname.startsWith("/trade-orders") ||
    pathname.startsWith("/trade-simulation-analysis") ||
    pathname.startsWith("/trade-strategy-backtest") ||
    pathname.startsWith("/trade-strategy") ||
    pathname.startsWith("/trade-backtest-runs")
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
        key: "/argus-dashboard",
        label: "Argus 总览",
        icon: <DashboardOutlined />,
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
        key: "/argus-group",
        icon: <ControlOutlined />,
        label: "Argus 管理",
        children: [
          {
            key: "/argus-dashboard",
            icon: <DashboardOutlined />,
            label: "Argus 总览",
          },
          {
            key: "/argus-instances",
            label: "实例与参数对比",
          },
          {
            key: "/argus-market",
            label: "历史行情与触发点",
          },
          {
            key: "/argus-signals",
            label: "信号复盘",
          },
          {
            key: "/argus-backtest",
            icon: <BarChartOutlined />,
            label: "盘口信号回测",
          },
          {
            key: "/argus-optimizer",
            icon: <ExperimentOutlined />,
            label: "自动参数寻优",
          },
          {
            key: "/argus-config",
            label: "参数与运行控制",
          },
        ],
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
          {
            key: "/trade-strategy",
            label: "策略管理",
          },
          {
            key: "/trade-backtest-runs",
            label: "回测对比",
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
  // 实例选择器只在有实例维度的页面出现（Argus 七页 + 数据总览）：
  // 用户、权限、币种这些模块没有实例维度，常驻一个空选择器只会误导。
  const scopeRoute = matchScopeRoute(activePath);

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
                  {scopeRoute ? <ArgusInstanceSelector allowAll={scopeRoute.allowAll} /> : null}
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
