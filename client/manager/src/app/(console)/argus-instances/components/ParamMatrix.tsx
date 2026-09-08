"use client";

import { useMemo, useState } from "react";
import { Checkbox, Empty, Table, Tag, Typography } from "antd";
import type { ColumnsType } from "antd/es/table";
import type { ArgusInstanceRuntime } from "@/components/argus/argus-instance.api";
import type { ArgusConfigSnapshot } from "../../argus-config/api/argus-config.api";
import { PARAM_GROUPS, formatParamValue, readParam, type ParamContext, type ParamDef } from "../../argus-config/params/catalog";

const { Text } = Typography;

interface ParamMatrixProps {
  instances: ArgusInstanceRuntime[];
  snapshots: Record<string, ArgusConfigSnapshot | null>;
  loading: boolean;
}

interface MatrixRow {
  key: string;
  group: string;
  param: ParamDef;
  values: Record<string, string>;
  different: boolean;
}

/**
 * 把每个实例的已发布快照平铺成矩阵。账户级参数不会只偷看第一个账户：一个单元有
 * champion / challenger 两行时，单元格会携带账户名，以免 26+8 被错误压缩成一个数字。
 */
export function ParamMatrix({ instances, snapshots, loading }: ParamMatrixProps) {
  const [onlyDifferent, setOnlyDifferent] = useState(false);
  const rows = useMemo(() => buildRows(instances, snapshots), [instances, snapshots]);
  const visibleRows = onlyDifferent ? rows.filter((item) => item.different) : rows;
  const columns = useMemo<ColumnsType<MatrixRow>>(() => [
    {
      title: "配置域 / 参数",
      width: 270,
      render: (_, row) => <div className="manager-instances-parameter"><span>{row.group}</span><b>{row.param.label}</b><small className="manager-argus-mono">{row.param.propertyKey}</small></div>,
    },
    ...instances.map((instance) => ({
      title: <div className="manager-instances-matrix__head"><b>{instance.instanceName || instance.instanceKey}</b><small className="manager-argus-mono">{instance.instanceKey}</small></div>,
      width: 230,
      render: (_: unknown, row: MatrixRow) => <div className={row.different ? "manager-instances-matrix__cell manager-instances-matrix__cell--different" : "manager-instances-matrix__cell"}>{row.values[instance.instanceKey] || "未发布"}</div>,
    })),
  ], [instances]);

  return (
    <section className="manager-argus-panel manager-instances-matrix">
      <div className="manager-argus-panel__head">
        <div>
          <div className="manager-argus-panel__title">参数对比矩阵</div>
          <Text type="secondary">只读已发布快照；黄色单元格代表实例间的值或作用账户不同</Text>
        </div>
        <Checkbox checked={onlyDifferent} onChange={(event) => setOnlyDifferent(event.target.checked)}>只看有差异的参数</Checkbox>
      </div>
      {visibleRows.length === 0 && !loading ? <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={onlyDifferent ? "没有可比较的参数差异" : "暂无已发布配置快照"} /> : <Table<MatrixRow> rowKey="key" columns={columns} dataSource={visibleRows} loading={loading} pagination={false} scroll={{ x: Math.max(920, 270 + instances.length * 230), y: 680 }} size="middle" />}
    </section>
  );
}

function buildRows(instances: ArgusInstanceRuntime[], snapshots: Record<string, ArgusConfigSnapshot | null>): MatrixRow[] {
  const rows: MatrixRow[] = [];
  for (const group of PARAM_GROUPS) {
    for (const param of group.params) {
      const values: Record<string, string> = {};
      for (const instance of instances) values[instance.instanceKey] = formatCell(param, snapshots[instance.instanceKey]);
      const comparable = instances.map((item) => values[item.instanceKey]).filter((value) => value && value !== "未发布");
      rows.push({ key: `${group.key}:${param.key}`, group: group.name, param, values, different: new Set(comparable).size > 1 });
    }
  }
  return rows;
}

function formatCell(param: ParamDef, snapshot: ArgusConfigSnapshot | null | undefined): string {
  if (!snapshot) return "未发布";
  if (param.scope === "global") return formatParamValue(param, readParam(param, snapshot, defaultContext));
  if (param.scope === "symbol") {
    const symbols = snapshot.monitorSymbols ?? [];
    return symbols.length ? symbols.map((symbol, symbolIndex) => `${symbol.symbol || `symbol${symbolIndex + 1}`}: ${formatParamValue(param, readParam(param, snapshot, { symbolIndex, riskIndex: 0 }))}`).join(" · ") : "未配置币种";
  }
  const risks = snapshot.accountRisks ?? [];
  return risks.length ? risks.map((risk, riskIndex) => `${accountName(snapshot, risk.accountId, riskIndex)}: ${formatParamValue(param, readParam(param, snapshot, { symbolIndex: 0, riskIndex }))}`).join(" · ") : "未配置账户";
}

const defaultContext: ParamContext = { symbolIndex: 0, riskIndex: 0 };

function accountName(snapshot: ArgusConfigSnapshot, accountId: number, riskIndex: number): string {
  return snapshot.accounts?.find((account) => account.id === accountId)?.accountName || `account${riskIndex + 1}`;
}
