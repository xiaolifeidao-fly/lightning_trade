/* ============================================================
   Argus 原型 — Canvas 图表渲染
   正式实现改用 lightweight-charts（蜡烛图 / 标记 / 多副图均为原生能力），
   这里手绘等价视觉，保证原型离线可预览、无远程依赖。
   ============================================================ */

const C = {
  grid: 'rgba(255,255,255,0.045)',
  axis: '#5E6673',
  text: '#848E9C',
  up: '#0ECB81',
  down: '#F6465D',
  gold: '#F0B90B',
  blue: '#4D7EFF',
  purple: '#A78BFA',
  orange: '#FF9F43',
  cross: 'rgba(240,185,11,.55)',
};

/** 一组联动图表共享的十字光标广播 */
const crosshairBus = { subs: [], emit(x) { this.subs.forEach(f => f(x)); } };

class Pane {
  constructor(host, height) {
    this.host = host;
    this.host.classList.add('chart-wrap');
    this.canvas = document.createElement('canvas');
    this.canvas.style.height = height + 'px';
    this.host.appendChild(this.canvas);
    this.ctx = this.canvas.getContext('2d');
    this.h = height;
    this.tip = document.createElement('div');
    this.tip.className = 'tooltip';
    this.host.appendChild(this.tip);
    this.padL = 8; this.padR = 68; this.padT = 10; this.padB = 22;
    this.hoverX = null;
    new ResizeObserver(() => this.resize()).observe(this.host);
    this.canvas.addEventListener('mousemove', e => {
      const r = this.canvas.getBoundingClientRect();
      this.hoverX = e.clientX - r.left;
      crosshairBus.emit(this.hoverX);
      this.onHover && this.onHover(this.hoverX, e.clientY - r.top);
    });
    this.canvas.addEventListener('mouseleave', () => { this.hoverX = null; crosshairBus.emit(null); this.tip.classList.remove('on'); });
    crosshairBus.subs.push(x => { if (x !== this.hoverX) { this.hoverX = x; this.draw(); } });
  }
  resize() {
    const dpr = window.devicePixelRatio || 1;
    const w = this.host.clientWidth;
    this.canvas.width = w * dpr;
    this.canvas.height = this.h * dpr;
    this.canvas.style.width = w + 'px';
    this.ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    this.w = w;
    this.draw();
  }
  get plotW() { return this.w - this.padL - this.padR; }
  get plotH() { return this.h - this.padT - this.padB; }
  xOf(i, n) { return this.padL + (n <= 1 ? this.plotW / 2 : (i / (n - 1)) * this.plotW); }
  iOf(x, n) { return Math.round(((x - this.padL) / this.plotW) * (n - 1)); }
  yOf(v, min, max) { return this.padT + (1 - (v - min) / (max - min || 1)) * this.plotH; }
  clear() {
    const g = this.ctx;
    g.clearRect(0, 0, this.w, this.h);
  }
  gridY(min, max, ticks, fmt) {
    const g = this.ctx;
    g.font = '11px ui-monospace, Menlo, monospace';
    g.textBaseline = 'middle';
    for (let i = 0; i <= ticks; i++) {
      const v = min + (max - min) * (i / ticks);
      const y = this.yOf(v, min, max);
      g.strokeStyle = C.grid; g.lineWidth = 1;
      g.beginPath(); g.moveTo(this.padL, Math.round(y) + .5); g.lineTo(this.padL + this.plotW, Math.round(y) + .5); g.stroke();
      g.fillStyle = C.text; g.textAlign = 'left';
      g.fillText(fmt(v), this.padL + this.plotW + 8, y);
    }
  }
  gridX(times, ticks = 6, fmt) {
    const g = this.ctx, n = times.length;
    const f = fmt || (t => fmtTime(t));
    g.font = '11px ui-monospace, Menlo, monospace';
    g.textBaseline = 'top';
    for (let i = 0; i <= ticks; i++) {
      const idx = Math.round((n - 1) * (i / ticks));
      const x = this.xOf(idx, n);
      g.strokeStyle = C.grid;
      g.beginPath(); g.moveTo(Math.round(x) + .5, this.padT); g.lineTo(Math.round(x) + .5, this.padT + this.plotH); g.stroke();
      g.fillStyle = C.text;
      // 首尾刻度贴边对齐，避免文字被画布裁掉
      g.textAlign = i === 0 ? 'left' : i === ticks ? 'right' : 'center';
      const tx = i === 0 ? this.padL : i === ticks ? this.padL + this.plotW : x;
      g.fillText(f(times[idx]), tx, this.padT + this.plotH + 6);
    }
  }
  crosshair(n) {
    if (this.hoverX == null) return -1;
    const i = Math.max(0, Math.min(n - 1, this.iOf(this.hoverX, n)));
    const x = this.xOf(i, n);
    const g = this.ctx;
    g.save();
    g.strokeStyle = C.cross; g.lineWidth = 1; g.setLineDash([3, 3]);
    g.beginPath(); g.moveTo(Math.round(x) + .5, this.padT); g.lineTo(Math.round(x) + .5, this.padT + this.plotH); g.stroke();
    g.restore();
    return i;
  }
  showTip(html, x, y) {
    this.tip.innerHTML = html;
    this.tip.classList.add('on');
    const tw = this.tip.offsetWidth;
    let left = x + 16;
    if (left + tw > this.w - 6) left = x - tw - 16;
    this.tip.style.left = Math.max(6, left) + 'px';
    this.tip.style.top = Math.max(6, Math.min(y - 10, this.h - this.tip.offsetHeight - 6)) + 'px';
  }
}

/* ---------------- 蜡烛图（含触发点标记 + 第二数据源对比线） ---------------- */
class CandleChart extends Pane {
  constructor(host, opts) {
    super(host, opts.height || 340);
    this.o = opts;
    this.onHover = (x, y) => {
      const n = this.o.candles.length;
      const i = Math.max(0, Math.min(n - 1, this.iOf(x, n)));
      const k = this.o.candles[i];
      const m = (this.o.markers || []).find(m => m.i === i);
      const s2 = this.o.second ? this.o.second[i] : null;
      this.showTip(`
        <b>${fmtTime(k.t, true)}</b>
        <div class="r"><span>开 / 收</span><b>${fmtPx(k.o)} / ${fmtPx(k.c)}</b></div>
        <div class="r"><span>高 / 低</span><b>${fmtPx(k.h)} / ${fmtPx(k.l)}</b></div>
        ${s2 ? `<div class="r"><span style="color:${C.blue}">DeepCoin 收</span><b>${fmtPx(s2.c)}</b></div>
                <div class="r"><span>双源价差</span><b>${signed(k.c - s2.c, 2)}</b></div>` : ''}
        ${m ? `<div class="r" style="margin-top:4px;border-top:1px solid #2B3139;padding-top:4px">
                <span style="color:${m.color}">${m.label}</span><b>${m.detail}</b></div>` : ''}
      `, x, y);
      this.draw();
    };
    this.resize();
  }
  draw() {
    if (!this.w) return;
    const g = this.ctx, ks = this.o.candles, n = ks.length;
    if (!n) return;
    this.clear();
    let min = Infinity, max = -Infinity;
    ks.forEach(k => { min = Math.min(min, k.l); max = Math.max(max, k.h); });
    if (this.o.second) this.o.second.forEach(k => { min = Math.min(min, k.l); max = Math.max(max, k.h); });
    const padV = (max - min) * 0.12;
    min -= padV; max += padV + (max - min) * 0.06;

    this.gridY(min, max, 5, v => fmtPx(v, 0));
    this.gridX(ks.map(k => k.t), 6, this.o.xFmt);

    const bw = Math.max(1.5, Math.min(9, this.plotW / n * 0.62));
    ks.forEach((k, i) => {
      const x = this.xOf(i, n);
      const up = k.c >= k.o;
      g.strokeStyle = g.fillStyle = up ? C.up : C.down;
      g.lineWidth = 1;
      g.beginPath();
      g.moveTo(Math.round(x) + .5, this.yOf(k.h, min, max));
      g.lineTo(Math.round(x) + .5, this.yOf(k.l, min, max));
      g.stroke();
      const y1 = this.yOf(Math.max(k.o, k.c), min, max);
      const y2 = this.yOf(Math.min(k.o, k.c), min, max);
      g.fillRect(x - bw / 2, y1, bw, Math.max(1, y2 - y1));
    });

    if (this.o.second) {
      g.strokeStyle = C.blue; g.lineWidth = 1.4;
      g.setLineDash([4, 3]);
      g.beginPath();
      this.o.second.forEach((k, i) => {
        const x = this.xOf(i, n), y = this.yOf(k.c, min, max);
        i ? g.lineTo(x, y) : g.moveTo(x, y);
      });
      g.stroke(); g.setLineDash([]);
    }

    (this.o.markers || []).forEach(m => {
      const k = ks[m.i]; if (!k) return;
      const x = this.xOf(m.i, n);
      const above = m.pos === 'above';
      const y = above ? this.yOf(k.h, min, max) - 9 : this.yOf(k.l, min, max) + 9;
      g.fillStyle = m.color;
      g.beginPath();
      if (above) { g.moveTo(x, y + 6); g.lineTo(x - 5, y - 2); g.lineTo(x + 5, y - 2); }
      else { g.moveTo(x, y - 6); g.lineTo(x - 5, y + 2); g.lineTo(x + 5, y + 2); }
      g.closePath(); g.fill();
      if (m.selected) {
        g.strokeStyle = '#fff'; g.lineWidth = 1.4; g.stroke();
        g.strokeStyle = m.color; g.setLineDash([2, 3]);
        g.beginPath(); g.moveTo(Math.round(x) + .5, this.padT); g.lineTo(Math.round(x) + .5, this.padT + this.plotH); g.stroke();
        g.setLineDash([]);
      }
    });

    const i = this.crosshair(n);
    if (i >= 0) {
      const k = ks[i];
      const y = this.yOf(k.c, min, max);
      g.fillStyle = C.gold;
      g.fillRect(this.padL + this.plotW + 2, y - 9, this.padR - 6, 18);
      g.fillStyle = '#0B0E11'; g.font = '700 11px ui-monospace, Menlo, monospace';
      g.textAlign = 'center'; g.textBaseline = 'middle';
      g.fillText(fmtPx(k.c, 1), this.padL + this.plotW + 2 + (this.padR - 6) / 2, y);
    }
  }
}

/* ---------------- 偏离副图（gapBp + 阈值带） ---------------- */
class DeviationChart extends Pane {
  constructor(host, opts) {
    super(host, opts.height || 130);
    this.o = opts;
    this.onHover = (x, y) => {
      const n = this.o.points.length;
      const i = Math.max(0, Math.min(n - 1, this.iOf(x, n)));
      const p = this.o.points[i];
      const over = Math.abs(p.bp) >= this.o.threshold;
      this.showTip(`<b>${fmtTime(p.t, true)}</b>
        <div class="r"><span>last-vs-mark</span><b style="color:${over ? C.gold : '#EAECEF'}">${signed(p.bp, 2, ' bp')}</b></div>
        <div class="r"><span>信号阈值</span><b>±${this.o.threshold} bp</b></div>
        <div class="r"><span>是否越阈</span><b style="color:${over ? C.up : C.text}">${over ? '是 · 触发判定' : '否'}</b></div>`, x, y);
      this.draw();
    };
    this.resize();
  }
  draw() {
    if (!this.w) return;
    const g = this.ctx, ps = this.o.points, n = ps.length;
    if (!n) return;
    this.clear();
    let m = 2;
    ps.forEach(p => m = Math.max(m, Math.abs(p.bp)));
    m = Math.ceil(m * 1.15);
    const min = -m, max = m;
    this.gridY(min, max, 4, v => v.toFixed(0));

    const th = this.o.threshold;
    g.fillStyle = 'rgba(240,185,11,.06)';
    g.fillRect(this.padL, this.yOf(max, min, max), this.plotW, this.yOf(th, min, max) - this.yOf(max, min, max));
    g.fillRect(this.padL, this.yOf(-th, min, max), this.plotW, this.yOf(min, min, max) - this.yOf(-th, min, max));
    g.strokeStyle = 'rgba(240,185,11,.5)'; g.setLineDash([4, 3]); g.lineWidth = 1;
    [th, -th].forEach(v => {
      const y = Math.round(this.yOf(v, min, max)) + .5;
      g.beginPath(); g.moveTo(this.padL, y); g.lineTo(this.padL + this.plotW, y); g.stroke();
    });
    g.setLineDash([]);
    g.strokeStyle = 'rgba(255,255,255,.14)';
    const y0 = Math.round(this.yOf(0, min, max)) + .5;
    g.beginPath(); g.moveTo(this.padL, y0); g.lineTo(this.padL + this.plotW, y0); g.stroke();

    g.strokeStyle = '#B7BDC6'; g.lineWidth = 1.2;
    g.beginPath();
    ps.forEach((p, i) => {
      const x = this.xOf(i, n), y = this.yOf(p.bp, min, max);
      i ? g.lineTo(x, y) : g.moveTo(x, y);
    });
    g.stroke();
    ps.forEach((p, i) => {
      if (Math.abs(p.bp) < th) return;
      g.fillStyle = C.gold;
      g.beginPath(); g.arc(this.xOf(i, n), this.yOf(p.bp, min, max), 2.6, 0, 7); g.fill();
    });
    this.crosshair(n);
  }
}

/* ---------------- 净持仓阶梯图 ---------------- */
class StepChart extends Pane {
  constructor(host, opts) {
    super(host, opts.height || 96);
    this.o = opts;
    this.onHover = (x, y) => {
      const n = this.o.points.length;
      const i = Math.max(0, Math.min(n - 1, this.iOf(x, n)));
      const p = this.o.points[i];
      this.showTip(`<b>${fmtTime(p.t, true)}</b>
        <div class="r"><span>净持仓</span><b>${p.v} 张</b></div>
        <div class="r"><span>仓位上限</span><b>${this.o.cap} 张</b></div>`, x, y);
      this.draw();
    };
    this.resize();
  }
  draw() {
    if (!this.w) return;
    const g = this.ctx, ps = this.o.points, n = ps.length;
    if (!n) return;
    this.clear();
    const max = Math.max(this.o.cap * 1.12, ...ps.map(p => p.v)) || 1;
    this.gridY(0, max, 2, v => v.toFixed(0) + '张');
    const yCap = Math.round(this.yOf(this.o.cap, 0, max)) + .5;
    g.strokeStyle = 'rgba(255,159,67,.6)'; g.setLineDash([5, 4]);
    g.beginPath(); g.moveTo(this.padL, yCap); g.lineTo(this.padL + this.plotW, yCap); g.stroke();
    g.setLineDash([]);
    g.fillStyle = C.orange; g.font = '10px ui-monospace, Menlo, monospace'; g.textAlign = 'left'; g.textBaseline = 'bottom';
    g.fillText('上限 ' + this.o.cap, this.padL + 4, yCap - 2);

    g.beginPath();
    g.moveTo(this.xOf(0, n), this.yOf(0, 0, max));
    ps.forEach((p, i) => {
      const x = this.xOf(i, n), y = this.yOf(p.v, 0, max);
      g.lineTo(x, y);
      if (i < n - 1) g.lineTo(this.xOf(i + 1, n), y);
    });
    g.lineTo(this.xOf(n - 1, n), this.yOf(0, 0, max));
    g.closePath();
    const grad = g.createLinearGradient(0, this.padT, 0, this.padT + this.plotH);
    grad.addColorStop(0, 'rgba(240,185,11,.34)');
    grad.addColorStop(1, 'rgba(240,185,11,.02)');
    g.fillStyle = grad; g.fill();
    g.strokeStyle = C.gold; g.lineWidth = 1.4;
    g.beginPath();
    ps.forEach((p, i) => {
      const x = this.xOf(i, n), y = this.yOf(p.v, 0, max);
      i ? g.lineTo(x, y) : g.moveTo(x, y);
      if (i < n - 1) g.lineTo(this.xOf(i + 1, n), y);
    });
    g.stroke();
    this.crosshair(n);
  }
}

/* ---------------- 多序列折线（权益 / 净值曲线） ---------------- */
class LineChart extends Pane {
  constructor(host, opts) {
    super(host, opts.height || 240);
    this.o = opts;
    this.padR = opts.padR ?? 68;
    this.onHover = (x, y) => {
      const n = this.o.series[0].points.length;
      const i = Math.max(0, Math.min(n - 1, this.iOf(x, n)));
      const rows = this.o.series.map(s => `<div class="r"><span style="color:${s.color}">${s.name}</span><b>${this.o.fmt(s.points[i].v)}</b></div>`).join('');
      const p0 = this.o.series[0].points[i];
      const label = this.o.xFmt ? this.o.xFmt(p0.t) : fmtTime(p0.t, true);
      this.showTip(`<b>${label}</b>${rows}`, x, y);
      this.draw();
    };
    this.resize();
  }
  draw() {
    if (!this.w) return;
    const g = this.ctx;
    const all = this.o.series.flatMap(s => s.points.map(p => p.v));
    if (!all.length) return;
    this.clear();
    let min = Math.min(...all), max = Math.max(...all);
    const pad = (max - min) * .16 || 1;
    min -= pad; max += pad;
    if (this.o.zeroBase) min = Math.min(0, min);
    this.gridY(min, max, 4, this.o.fmt);
    this.gridX(this.o.series[0].points.map(p => p.t), this.o.xTicks || 5, this.o.xFmt);
    this.o.series.forEach(s => {
      const n = s.points.length;
      if (s.fill) {
        g.beginPath();
        g.moveTo(this.xOf(0, n), this.yOf(min, min, max));
        s.points.forEach((p, i) => g.lineTo(this.xOf(i, n), this.yOf(p.v, min, max)));
        g.lineTo(this.xOf(n - 1, n), this.yOf(min, min, max));
        g.closePath();
        const grad = g.createLinearGradient(0, this.padT, 0, this.padT + this.plotH);
        grad.addColorStop(0, s.fill); grad.addColorStop(1, 'rgba(0,0,0,0)');
        g.fillStyle = grad; g.fill();
      }
      g.strokeStyle = s.color; g.lineWidth = s.width || 1.7;
      if (s.dash) g.setLineDash(s.dash);
      g.beginPath();
      s.points.forEach((p, i) => {
        const x = this.xOf(i, n), y = this.yOf(p.v, min, max);
        i ? g.lineTo(x, y) : g.moveTo(x, y);
      });
      g.stroke(); g.setLineDash([]);
    });
    this.crosshair(this.o.series[0].points.length);
  }
}

/* ---------------- 秒级切片图（信号前后 ±1min 双源） ---------------- */
function drawSliceChart(canvas, slice, signalIdx) {
  const dpr = window.devicePixelRatio || 1;
  const w = canvas.parentElement.clientWidth, h = 190;
  canvas.width = w * dpr; canvas.height = h * dpr;
  canvas.style.width = w + 'px'; canvas.style.height = h + 'px';
  const g = canvas.getContext('2d');
  g.setTransform(dpr, 0, 0, dpr, 0, 0);
  const padL = 8, padR = 62, padT = 12, padB = 20;
  const pw = w - padL - padR, ph = h - padT - padB;
  const all = slice.flatMap(p => [p.bin, p.dcLast, p.dcMark]);
  let min = Math.min(...all), max = Math.max(...all);
  const pad = (max - min) * .2 || 1; min -= pad; max += pad;
  const X = i => padL + (i / (slice.length - 1)) * pw;
  const Y = v => padT + (1 - (v - min) / (max - min)) * ph;

  g.font = '11px ui-monospace, Menlo, monospace'; g.textBaseline = 'middle';
  for (let i = 0; i <= 4; i++) {
    const v = min + (max - min) * (i / 4), y = Y(v);
    g.strokeStyle = C.grid; g.beginPath(); g.moveTo(padL, y + .5); g.lineTo(padL + pw, y + .5); g.stroke();
    g.fillStyle = C.text; g.textAlign = 'left'; g.fillText(fmtPx(v, 1), padL + pw + 8, y);
  }
  const xs = X(signalIdx);
  g.fillStyle = 'rgba(240,185,11,.09)';
  g.fillRect(xs - 2, padT, 4, ph);
  g.strokeStyle = C.gold; g.setLineDash([3, 3]); g.lineWidth = 1;
  g.beginPath(); g.moveTo(xs + .5, padT); g.lineTo(xs + .5, padT + ph); g.stroke(); g.setLineDash([]);
  g.fillStyle = C.gold; g.textAlign = 'center'; g.textBaseline = 'top'; g.font = '700 10px ui-monospace, Menlo, monospace';
  g.fillText('T0 触发', xs, padT + 2);

  // dcLast 与 dcMark 的偏离才是信号源；bin（币安）只作参照
  [['bin', C.text, [4, 3], 1.1], ['dcMark', C.blue, [], 1.5], ['dcLast', C.gold, [], 1.7]].forEach(([key, color, dash, lw]) => {
    g.strokeStyle = color; g.lineWidth = lw; g.setLineDash(dash);
    g.beginPath();
    slice.forEach((p, i) => { const x = X(i), y = Y(p[key]); i ? g.lineTo(x, y) : g.moveTo(x, y); });
    g.stroke(); g.setLineDash([]);
  });
  g.fillStyle = C.text; g.textAlign = 'center'; g.textBaseline = 'top'; g.font = '11px ui-monospace, Menlo, monospace';
  [0, Math.floor(slice.length / 2), slice.length - 1].forEach(i => {
    const rel = i - signalIdx;
    g.fillText((rel > 0 ? '+' : '') + rel + 's', X(i), padT + ph + 5);
  });
}

/* ---------------- 环形图 ---------------- */
function drawDonut(canvas, segments, centerTop, centerSub) {
  const dpr = window.devicePixelRatio || 1;
  const size = Math.min(canvas.parentElement.clientWidth, 210);
  canvas.width = size * dpr; canvas.height = size * dpr;
  canvas.style.width = size + 'px'; canvas.style.height = size + 'px';
  const g = canvas.getContext('2d');
  g.setTransform(dpr, 0, 0, dpr, 0, 0);
  const total = segments.reduce((s, x) => s + x.value, 0) || 1;
  const cx = size / 2, cy = size / 2, r = size / 2 - 6, inner = r * 0.66;
  let a = -Math.PI / 2;
  segments.forEach(s => {
    const sweep = (s.value / total) * Math.PI * 2;
    g.beginPath();
    g.arc(cx, cy, r, a, a + sweep);
    g.arc(cx, cy, inner, a + sweep, a, true);
    g.closePath();
    g.fillStyle = s.color; g.fill();
    g.strokeStyle = '#181A20'; g.lineWidth = 2; g.stroke();
    a += sweep;
  });
  g.fillStyle = '#EAECEF'; g.textAlign = 'center'; g.textBaseline = 'middle';
  g.font = '800 24px Inter, sans-serif';
  g.fillText(centerTop, cx, cy - 8);
  g.font = '12px Inter, sans-serif'; g.fillStyle = '#848E9C';
  g.fillText(centerSub, cx, cy + 14);
}

/* ---------------- 迷你走势 ---------------- */
function drawSpark(canvas, values, color) {
  const dpr = window.devicePixelRatio || 1;
  const w = canvas.parentElement.clientWidth, h = 38;
  canvas.width = w * dpr; canvas.height = h * dpr;
  canvas.style.width = w + 'px'; canvas.style.height = h + 'px';
  const g = canvas.getContext('2d');
  g.setTransform(dpr, 0, 0, dpr, 0, 0);
  const min = Math.min(...values), max = Math.max(...values);
  const X = i => (i / (values.length - 1)) * w;
  const Y = v => 4 + (1 - (v - min) / (max - min || 1)) * (h - 8);
  g.beginPath();
  g.moveTo(0, h);
  values.forEach((v, i) => g.lineTo(X(i), Y(v)));
  g.lineTo(w, h); g.closePath();
  const grad = g.createLinearGradient(0, 0, 0, h);
  grad.addColorStop(0, color.replace('rgb', 'rgba').replace(')', ',.3)'));
  grad.addColorStop(1, 'rgba(0,0,0,0)');
  g.fillStyle = grad; g.fill();
  g.strokeStyle = color; g.lineWidth = 1.6;
  g.beginPath();
  values.forEach((v, i) => i ? g.lineTo(X(i), Y(v)) : g.moveTo(X(i), Y(v)));
  g.stroke();
}
