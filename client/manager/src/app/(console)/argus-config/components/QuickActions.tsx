"use client";

import { ThunderboltFilled } from "@ant-design/icons";
import { Button, Tooltip, Typography } from "antd";
import { QUICK_ACTIONS, findParam, type QuickAction } from "../params/catalog";

const { Text } = Typography;

interface QuickActionsProps {
  disabled: boolean;
  onApply: (action: QuickAction) => void;
}

/**
 * 极端行情快速下发：跳过完整表单，直接把关键项填进草稿。它只填草稿，不绕过
 * 二次确认与发布说明——「快」指的是少点几下，不是少一道闸。
 */
export function QuickActions({ disabled, onApply }: QuickActionsProps) {
  return (
    <div className="manager-argus-panel manager-argus-panel--alert">
      <div className="manager-argus-panel__head">
        <div className="manager-argus-panel__title">
          <span className="manager-argus-panel__icon">
            <ThunderboltFilled />
          </span>
          极端行情快速下发
        </div>
        <Text type="secondary" style={{ fontSize: 12.5 }}>
          只填草稿，仍需确认后发布
        </Text>
      </div>
      <div className="manager-argus-quickrow">
        {QUICK_ACTIONS.map((action) => {
          const param = findParam(action.paramKey);
          return (
            <Tooltip key={action.key} title={`${action.note}（${param?.storeKey ?? action.paramKey}）`}>
              <Button size="small" danger={action.danger} disabled={disabled} onClick={() => onApply(action)}>
                {action.label}
              </Button>
            </Tooltip>
          );
        })}
      </div>
      <div className="manager-argus-hint manager-argus-hint--danger">
        <span>
          <b>护栏：</b>关键参数（止损 / 上限 / 风险预算 / 门控 / 信号阈值 / 下单张数）改动必须二次确认；发布后要等心跳回报确认程序真读到新版本，未确认前页面不会显示「已生效」。
        </span>
      </div>
      <div className="manager-argus-hint">
        <span>
          <b>为什么没有「停止进程」按钮：</b>停进程后持仓仍挂在交易所，但移动止盈、兜底止损、平仓监控全部失效——比不停更危险。真正的紧急刹车是
          <b>「暂停新开仓」</b>（<span className="manager-argus-mono">order_size = 0</span>）：停止开新仓，但继续看管已有持仓。进程启停属于发版与故障处置，留在运维侧（SSH + script/control.sh）。
        </span>
      </div>
    </div>
  );
}
