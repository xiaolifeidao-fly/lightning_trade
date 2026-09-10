"use client";

import { ReloadOutlined } from "@ant-design/icons";
import { Button, Result, Spin, Typography } from "antd";
import { useEffect, useState } from "react";

const { Paragraph, Text } = Typography;

/** 同一标签页在这个时间窗内只自愈一次，避免"刷新→又失败→再刷新"的死循环。 */
const RELOAD_GUARD_MS = 30_000;
const RELOAD_FLAG = "manager:asset-reload-at";

/**
 * 全站错误边界。
 *
 * 直接动因：前端每次换版，webpack 产出的 chunk 文件名都会变（实测同一个
 * /argus-instances 页面，换版前是 page-fd90cd08040776b1.js，换版后是
 * page-749106afb8652138.js，旧文件在新部署里直接 404）。换版前打开的标签页
 * 仍按旧清单去取 chunk，一点导航就抛 ChunkLoadError。
 *
 * 此前仓库里**没有任何 error 边界**，于是这种情况落到 Next 自带的默认错误页，
 * 只有一句 "Application error: a client-side exception has occurred" —— 用户
 * 看不出这其实只是"页面旧了、刷一下就好"，会当成功能坏了来报。
 *
 * 这里把两类错误分开，绝不混为一谈：
 *   · **取不到静态资源** → 自动硬刷新一次，把新版本拉下来；
 *   · **真正的代码异常** → 给中文说明、重试入口和 digest，绝不自动刷新——
 *     一刷就好的假象会把真 bug 藏起来，比报错本身更难查。
 */
export default function AppError({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  const [staleBuild, setStaleBuild] = useState(false);

  useEffect(() => {
    if (!isAssetLoadError(error)) return;
    setStaleBuild(true);

    let last = 0;
    try {
      last = Number(window.sessionStorage.getItem(RELOAD_FLAG) ?? 0);
      if (Date.now() - last < RELOAD_GUARD_MS) return;
      window.sessionStorage.setItem(RELOAD_FLAG, String(Date.now()));
    } catch {
      // 隐私模式下 sessionStorage 会直接抛。这时退化成"只提示、不自动刷"：
      // 没有防抖手段还自动刷新，等于给用户一个刷不停的页面。
      return;
    }
    window.location.reload();
  }, [error]);

  if (staleBuild) {
    return (
      <div style={{ minHeight: "100vh", display: "grid", placeItems: "center" }}>
        <Result
          icon={<Spin size="large" />}
          title="管理端已更新，正在加载新版本"
          subTitle="这个标签页还在用换版前的前端资源。正在自动刷新；若没有自动跳转，请手动刷新页面。"
          extra={
            <Button type="primary" icon={<ReloadOutlined />} onClick={() => window.location.reload()}>
              立即刷新
            </Button>
          }
        />
      </div>
    );
  }

  return (
    <div style={{ minHeight: "100vh", display: "grid", placeItems: "center", padding: 24 }}>
      <Result
        status="error"
        title="页面出错了"
        subTitle="这一页的前端代码抛了异常，不是数据没取到。下面的错误信息请一并反馈。"
        extra={[
          <Button type="primary" key="retry" onClick={reset}>
            重试本页
          </Button>,
          <Button key="home" onClick={() => window.location.assign("/manager-dashboard")}>
            回数据总览
          </Button>,
        ]}
      >
        <Paragraph>
          <Text code copyable={{ text: describe(error) }}>
            {describe(error)}
          </Text>
        </Paragraph>
      </Result>
    </div>
  );
}

/**
 * 判定是不是「取不到静态资源」。
 *
 * 只认加载类的明确信号。**不能**因为 message 里出现 "chunk" 这种模糊词就兜底判成
 * 旧版本：把真正的代码 bug 误判成换版，会被一次自动刷新掩盖过去，然后以
 * "偶尔白屏"的形式反复出现，比直接报错难查得多。
 */
function isAssetLoadError(error: Error): boolean {
  if (error?.name === "ChunkLoadError") return true;
  const message = error?.message ?? "";
  return (
    /Loading chunk [\w-]+ failed/i.test(message) ||
    /Loading CSS chunk [\w-]+ failed/i.test(message) ||
    /Failed to fetch dynamically imported module/i.test(message) ||
    /error loading dynamically imported module/i.test(message) ||
    /Importing a module script failed/i.test(message)
  );
}

/** digest 是生产构建里唯一能和服务端日志对上的线索，必须显示出来。 */
function describe(error: Error & { digest?: string }): string {
  const name = error?.name || "Error";
  const message = error?.message || "(无错误信息)";
  return error?.digest ? `${name}: ${message} [digest ${error.digest}]` : `${name}: ${message}`;
}
