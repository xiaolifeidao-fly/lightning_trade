"use client";

import { AuditOutlined } from "@ant-design/icons";
import { Typography } from "antd";
import { COVERAGE_AUDIT_AT, COVERAGE_BUCKETS, COVERAGE_CRITICAL_NOTE, COVERAGE_TOTAL_KEYS } from "../params/coverage";

const { Text } = Typography;

/** 配置面覆盖度：properties 里的 63 个键收敛到 DB 的进度，让人一眼看到还差多少。 */
export function CoveragePanel() {
  const gaps = COVERAGE_BUCKETS.filter((bucket) => bucket.gap).reduce((total, bucket) => total + bucket.count, 0);

  return (
    <div className="manager-argus-panel">
      <div className="manager-argus-panel__head">
        <div className="manager-argus-panel__title">
          <span className="manager-argus-panel__icon">
            <AuditOutlined />
          </span>
          配置面覆盖度
        </div>
        <Text type="secondary" style={{ fontSize: 12.5 }}>
          properties 共 {COVERAGE_TOTAL_KEYS} 个键 · 尚有 {gaps} 项缺口 · 审计于 {COVERAGE_AUDIT_AT}
        </Text>
      </div>

      {COVERAGE_BUCKETS.map((bucket) => (
        <div className="manager-argus-bar" key={bucket.key}>
          <div className="manager-argus-bar__label">
            <div>{bucket.name}</div>
            <div className="manager-argus-bar__detail">{bucket.detail}</div>
          </div>
          <div className="manager-argus-bar__track">
            <div
              className="manager-argus-bar__fill"
              style={{ width: `${(bucket.count / COVERAGE_TOTAL_KEYS) * 100}%`, background: bucket.color }}
            />
          </div>
          <span className="manager-argus-mono manager-argus-bar__value">{bucket.count}</span>
        </div>
      ))}

      <div className="manager-argus-hint manager-argus-hint--danger">
        <span>
          <b>其中一项是功能性故障：</b>
          {COVERAGE_CRITICAL_NOTE}
        </span>
      </div>
      <div className="manager-argus-hint">
        <span>
          这是一份人工审计结论（来源需求大纲 §3.2）。运行时探测不出「DB 有列但代码不读」这种缺口，只能靠逐项比对
          <span className="manager-argus-mono"> runtimeFromSnapshot / runtimeAccount </span>
          的赋值；收敛进度由 r5 推进，变化时同步更新 <span className="manager-argus-mono">params/coverage.ts</span>。
        </span>
      </div>
    </div>
  );
}
