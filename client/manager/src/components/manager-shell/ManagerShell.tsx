"use client";

import {
  AppstoreOutlined,
  BankOutlined,
  BellOutlined,
  CompassOutlined,
  DatabaseOutlined,
  LogoutOutlined,
  SafetyCertificateOutlined,
  TeamOutlined,
  UsergroupAddOutlined,
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
  "/permission": "权限管理",
  "/grain/stations": "粮站列表",
  "/grain/config": "基础设置",
  "/grain/payment-methods": "付款方式",
  "/grain/farmers": "农户管理",
  "/app-user": "业务员管理",
  "/grain/entries": "收粮明细",
};

function getOpenKeys(pathname: string) {
  if (pathname.startsWith("/activation-code")) {
    return ["/activation-code"];
  }
  if (pathname.startsWith("/product")) {
    return ["/product"];
  }
  if (pathname.startsWith("/collect")) {
    return ["/collect"];
  }
  if (pathname.startsWith("/shop")) {
    return ["/shop"];
  }
  if (pathname.startsWith("/app-user")) {
    return ["/grain-farmer-group"];
  }
  if (pathname.startsWith("/permission")) {
    return ["/system-group"];
  }
  if (pathname.startsWith("/grain")) {
    if (
      pathname.startsWith("/grain/stations") ||
      pathname.startsWith("/grain/config") ||
      pathname.startsWith("/grain/payment-methods")
    ) {
      return ["/grain-station-group"];
    }
    if (pathname.startsWith("/grain/farmers")) {
      return ["/grain-farmer-group"];
    }
    if (pathname.startsWith("/grain/entries")) {
      return ["/grain-purchase-group"];
    }
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
        label: "数据总览",
        icon: <AppstoreOutlined />,
      },
      {
        key: "/user",
        label: "用户管理",
        icon: <TeamOutlined />,
      },
      {
        key: "/permission",
        label: "权限管理",
        icon: <SafetyCertificateOutlined />,
      },
      {
        key: "/app-user",
        label: "业务员管理",
        icon: <TeamOutlined />,
      },
      {
        key: "/grain/entries",
        label: "收粮明细",
        icon: <DatabaseOutlined />,
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
        key: "/user",
        icon: <TeamOutlined />,
        label: "用户管理",
      },
      {
        key: "/system-group",
        icon: <SafetyCertificateOutlined />,
        label: "系统设置",
        children: [
          {
            key: "/permission",
            label: "角色资源",
          },
        ],
      },
      {
        key: "/grain-station-group",
        icon: <BankOutlined />,
        label: "粮站管理",
        children: [
          {
            key: "/grain/stations",
            label: "粮站列表",
          },
          {
            key: "/grain/config",
            label: "基础设置",
          },
          {
            key: "/grain/payment-methods",
            label: "付款方式",
          },
        ],
      },
      {
        key: "/grain-farmer-group",
        icon: <UsergroupAddOutlined />,
        label: "粮户管理",
        children: [
          {
            key: "/grain/farmers",
            label: "农户管理",
          },
          {
            key: "/app-user",
            label: "业务员管理",
          },
        ],
      },
      {
        key: "/grain-purchase-group",
        icon: <DatabaseOutlined />,
        label: "收粮管理",
        children: [
          {
            key: "/grain/entries",
            label: "收粮明细",
          },
        ],
      },
    ],
    [],
  );
  const selectedKey = activePath === "/activation-code" ? "/activation-code/admin" : activePath;

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
                <div className="manager-brand-kicker">管理控制台</div>
                <Space align="start" size={12} style={{ marginTop: 18 }}>
                  <div className="manager-crest" />
                  <div className="manager-wordmark">
                    <strong style={{ color: "#fff" }}>收粮管理端</strong>
                    <span style={{ color: "rgba(255,255,255,0.66)" }}>Shennong Admin</span>
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
                <span>权限模式</span>
                <strong>系统角色权限</strong>
                <Tag bordered={false}>按角色授权</Tag>
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
                      borderRadius: 8,
                      border: "1px solid var(--manager-border)",
                      background: "#ffffff",
                    }}
                  >
                    <Space size={12}>
                      <Avatar
                        style={{
                          width: 38,
                          height: 38,
                          background: "linear-gradient(135deg, var(--manager-primary), #4f8f5f)",
                          color: "#fff",
                          fontWeight: 700,
                        }}
                      >
                        A
                      </Avatar>
                      <div>
                        <div style={{ fontWeight: 700, color: "var(--manager-text)" }}>林安</div>
                        <Text style={{ color: "var(--manager-text-soft)" }}>系统管理员</Text>
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
