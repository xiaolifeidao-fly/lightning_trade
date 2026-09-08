"use client";

import { ClusterOutlined } from "@ant-design/icons";
import { Select, Tooltip } from "antd";
import { useEffect, useMemo, useState } from "react";
import { fetchArgusInstanceOverview, type ArgusInstanceRuntime } from "./argus-instance.api";
import { ALL_INSTANCES, useArgusInstanceScope } from "./instanceScope";

interface ArgusInstanceSelectorProps {
  /**
   * 是否允许「全部实例」。参数编辑页与行情主视图必须落到具体实例：
   * 前者一次发布波及多个实例就毁掉 champion/challenger 对照组，
   * 后者的阈值线与净持仓阶梯跨实例混排读不出任何结论。
   */
  allowAll: boolean;
}

/**
 * 顶栏的全局实例选择器，只在 Argus 页面出现。
 *
 * 它读的是 /argus-config/instance-overview 而不是 /argus-config/instances——多一个
 * 心跳状态就能在选项里直接标出哪个实例掉线、哪个参数还没生效，省得进页面才发现。
 */
export function ArgusInstanceSelector({ allowAll }: ArgusInstanceSelectorProps) {
  const [scope, setScope] = useArgusInstanceScope();
  const [instances, setInstances] = useState<ArgusInstanceRuntime[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;
    void fetchArgusInstanceOverview()
      .then((overview) => {
        if (cancelled) return;
        setInstances(overview.instances ?? []);
      })
      .catch(() => {
        // 顶栏读不到实例不该弹全局错误：页面自己会报更具体的原因。
        if (!cancelled) setInstances([]);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  // 不允许「全部实例」的页面，进来时若还停在全部实例就静默落到第一个实例，
  // 并把这个选择写回全局作用域——否则回到总览页会看到与刚才不同的口径。
  useEffect(() => {
    if (allowAll || instances.length === 0) return;
    const matched = instances.some((item) => item.instanceKey === scope);
    if (!matched) setScope(instances[0].instanceKey);
  }, [allowAll, instances, scope, setScope]);

  const options = useMemo(() => {
    const list = instances.map((item) => {
      const stale = !item.online;
      const drift = item.effectState === "drift" || item.effectState === "awaiting";
      const suffix = stale ? " · 心跳超时" : drift ? " · 参数未生效" : "";
      return {
        value: item.instanceKey,
        label: `${item.instanceName || item.instanceKey}${suffix}`,
      };
    });
    if (!allowAll) return list;
    return [{ value: ALL_INSTANCES, label: `全部实例（${instances.length} 个部署单元）` }, ...list];
  }, [allowAll, instances]);

  return (
    <Tooltip
      title={
        allowAll
          ? "全局实例作用域，切换后所有 Argus 页面同步生效"
          : "本页必须落到具体实例：参数与阈值线按实例分域，跨实例混排会读出错误结论"
      }
    >
      <div className="manager-argus-scope manager-argus-scope--shell">
        <ClusterOutlined className="manager-argus-scope__icon" />
        <Select
          size="middle"
          variant="borderless"
          loading={loading}
          value={allowAll || scope !== ALL_INSTANCES ? scope : undefined}
          placeholder={loading ? "读取实例…" : "请选择实例"}
          options={options}
          style={{ minWidth: 210 }}
          onChange={setScope}
        />
      </div>
    </Tooltip>
  );
}
