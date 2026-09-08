"use client";

import { InputNumber, Select, Tag, Tooltip } from "antd";
import { PARAM_GROUPS, type ParamDef } from "../params/catalog";

interface ParamFormProps {
  /** 当前显示值（含未发布改动） */
  values: Record<string, number>;
  /** 已发布基线值，用于高亮改动 */
  baseline: Record<string, number>;
  disabled: boolean;
  onChange: (param: ParamDef, value: number) => void;
}

function ParamBadges({ param }: { param: ParamDef }) {
  return (
    <>
      {param.critical ? (
        <Tooltip title="关键参数：直接影响实盘下单与风控，发布前需要二次确认">
          <Tag color="red" className="manager-argus-badge">
            关键
          </Tag>
        </Tooltip>
      ) : null}
      {param.hot ? null : (
        <Tooltip title="结构性参数，发布后不会热生效，需要重启 argus_single">
          <Tag color="gold" className="manager-argus-badge">
            需重启
          </Tag>
        </Tooltip>
      )}
      {param.pendingRuntime ? (
        <Tooltip title="DB 侧可写，但 argus_single 运行时还没消费这个值；消费侧由 r5「配置面完整收敛到 DB」打通">
          <Tag color="blue" className="manager-argus-badge">
            运行时待打通
          </Tag>
        </Tooltip>
      ) : null}
    </>
  );
}

export function ParamForm({ values, baseline, disabled, onChange }: ParamFormProps) {
  return (
    <div className="manager-argus-paramform">
      {PARAM_GROUPS.map((group) => (
        <div className="manager-argus-paramgroup" key={group.key}>
          <div className="manager-argus-paramgroup__head">
            <span className="manager-argus-paramgroup__name">{group.name}</span>
            {group.desc ? <span className="manager-argus-paramgroup__desc">{group.desc}</span> : null}
          </div>
          {group.note ? <div className="manager-argus-hint">{group.note}</div> : null}
          <div className="manager-argus-paramgrid">
            {group.params.map((param) => {
              const value = values[param.key] ?? 0;
              const changed = value !== (baseline[param.key] ?? 0);
              return (
                <div className={`manager-argus-param${changed ? " manager-argus-param--changed" : ""}`} key={param.key}>
                  <div className="manager-argus-param__label">
                    <span>{param.label}</span>
                    {param.unit ? <span className="manager-argus-param__unit">（{param.unit}）</span> : null}
                    <ParamBadges param={param} />
                  </div>
                  {param.type === "switch" ? (
                    <Select
                      size="small"
                      value={value === 1 ? 1 : 0}
                      disabled={disabled}
                      onChange={(next: number) => onChange(param, next)}
                      options={[
                        { value: 1, label: "开" },
                        { value: 0, label: "关" },
                      ]}
                      style={{ width: "100%" }}
                    />
                  ) : (
                    <InputNumber
                      size="small"
                      className="manager-argus-mono"
                      value={value}
                      step={param.step}
                      min={param.min}
                      max={param.max}
                      precision={param.precision}
                      disabled={disabled}
                      onChange={(next) => onChange(param, typeof next === "number" ? next : 0)}
                      style={{ width: "100%" }}
                    />
                  )}
                  <div className="manager-argus-param__keys">
                    <Tooltip title="部署 properties 里的真实参数键">
                      <span className="manager-argus-mono">{param.propertyKey}</span>
                    </Tooltip>
                    <Tooltip title="配置版本快照里的实际落点">
                      <span className="manager-argus-mono manager-argus-param__store">{param.storeKey}</span>
                    </Tooltip>
                  </div>
                  {param.hint ? <div className="manager-argus-param__hint">{param.hint}</div> : null}
                </div>
              );
            })}
          </div>
        </div>
      ))}
    </div>
  );
}
