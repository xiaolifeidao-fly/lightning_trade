import type { Metadata } from "next";
import type { ReactNode } from "react";
import { AntdRegistry } from "@ant-design/nextjs-registry";
import { ConfigProvider } from "antd";
import zhCN from "antd/locale/zh_CN";
import { modernTheme } from "@/styles/theme";
import "./globals.css";

export const metadata: Metadata = {
  title: "Lightning Trade · 量化合约管理台",
  description: "Lightning Trade 数字资产量化合约管理控制台",
};

export default function RootLayout({
  children,
}: Readonly<{
  children: ReactNode;
}>) {
  return (
    <html lang="zh-CN">
      <body>
        <AntdRegistry>
          <ConfigProvider theme={modernTheme} locale={zhCN}>
            {children}
          </ConfigProvider>
        </AntdRegistry>
      </body>
    </html>
  );
}
