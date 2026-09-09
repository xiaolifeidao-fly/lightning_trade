/**
 * Argus 配置里的比例字段 → 展示单位换算。
 *
 * `monitor_symbol.signal_threshold` / `spread_threshold` 在库里存的是**比例**，
 * 不是 bp：argus_single 的 `DefaultSignalThreshold = 0.0005` 注释就写着「5bp」
 * （pkg/monitor/deviation_rule.go），而 gap_bp 的定义是
 * `(last − mark) / mark × 10000`。所以 1 bp = 0.0001，换算必须乘 10000。
 *
 * 之所以单独抽出来：这个坑已经踩过两次——总览页把 0.0003 直接拼成
 * "0.0003 bp"（应为 3 bp），行情主视图把比例当 bp 喂给阈值参考线，
 * 导致本该画在 ±3bp 的两条线贴在 0 上，而副图的 gap_bp 实际在 ±5bp 量级。
 */

/** 一个 bp 对应的比例值。 */
export const BP = 0.0001;

/**
 * 比例 → bp。null/undefined/NaN 一律透传成 null，交给调用方决定怎么显示「没有」。
 *
 * 末尾那次 1e-6 归一化不是洁癖：`0.0003 * 10000` 在 IEEE754 下等于
 * 2.9999999999999996（除以 0.0001 也一样），不归一化的话生产那个 3bp 阈值
 * 会显示成 "3.00 bp"，而寻优扫出来的 4.5bp / 1.23bp 仍需保留小数，
 * 所以不能简单取整。
 */
export function ratioToBp(ratio: number | null | undefined): number | null {
  if (ratio === null || ratio === undefined || Number.isNaN(ratio)) return null;
  return Math.round((ratio / BP) * 1e6) / 1e6;
}

/**
 * 比例 → 「N bp」展示串。digits 默认 2 位：生产阈值是 3bp / 5bp 这种整数，
 * 但寻优扫出来的格子会有 4.5bp 之类的小数。
 */
export function formatRatioAsBp(ratio: number | null | undefined, digits = 2, empty = "—"): string {
  const bp = ratioToBp(ratio);
  if (bp === null) return empty;
  // 整数就不拖小数点，3bp 不要显示成 3.00bp。
  const text = Number.isInteger(bp) ? String(bp) : bp.toFixed(digits);
  return `${text} bp`;
}
