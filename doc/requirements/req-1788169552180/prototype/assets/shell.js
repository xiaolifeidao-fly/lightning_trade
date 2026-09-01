/* ============================================================
   Argus 可视化管理端 原型 — 公共骨架与模拟数据
   说明：原型不依赖任何远程资源。正式实现的 K 线图将使用
   lightweight-charts；此处用 Canvas 手绘等价视觉以便离线预览。
   ============================================================ */

/* ------------------------------------------------------------------
   实例（部署单元）。字段取自 server/argus_single/configs/*.properties
   与 script/deploy*.sh，只保留非敏感项——密钥/Cookie/Token 一律不进原型。
   ------------------------------------------------------------------ */
const INSTANCES = [
  { key: 'argus-single-1', name: '实例1 · 基线', host: '服务器 A',
    config: 'configs/application.properties', deploy: 'script/deploy.sh',
    accounts: [{ short: '账户A', label: 'account1', cap: 15 }],
    threshold: 5.0, orderSize: 1, cap: 15, budget: 20, stop: 300, trend: '未配置',
    variants: ['champion/default'],
    online: true, version: 37, hbSec: 3, signals: 6, net: 5, equityPct: 4.2, pnl: 128 },

  { key: 'argus-single-2', name: '实例2 · 双账户实验', host: '服务器 A',
    config: 'configs/application_1.properties', deploy: 'script/deploy_1.sh',
    accounts: [{ short: '账户A', label: 'account1', cap: 26 }, { short: '账户B', label: 'account2', cap: 8 }],
    threshold: 3.0, orderSize: 1, cap: 26, budget: 13.3, stop: 400, trend: '24h / 5%',
    variants: ['champion/S400_cap26_gate8', 'challenger/S400_cap8_gate8'],
    online: true, version: 37, hbSec: 5, signals: 23, net: 19, equityPct: 12.9, pnl: 1480 },

  { key: 'argus-single-3', name: '实例3 · 大仓', host: '服务器 B',
    config: 'configs/application_2.properties', deploy: 'script/deploy_2.sh',
    accounts: [{ short: '账户C', label: 'account1', cap: 246 }],
    threshold: 3.0, orderSize: 10, cap: 246, budget: 13.3, stop: 400, trend: '24h / 5%',
    variants: ['champion/S400_cap246_gate8'],
    online: true, version: 36, hbSec: 8, signals: 21, net: 164, equityPct: -3.6, pnl: -392 },
];

const INSTANCE_STORE = 'argus.prototype.instance';
function currentInstanceKey() {
  try { return localStorage.getItem(INSTANCE_STORE) || 'all'; } catch (e) { return 'all'; }
}
function currentInstance() { return INSTANCES.find(i => i.key === currentInstanceKey()) || null; }
function setInstance(key) {
  try { localStorage.setItem(INSTANCE_STORE, key); } catch (e) {}
  location.reload();
}

const NAV = [
  { group: '', items: [
    { key: 'dashboard', href: 'index.html', icon: '▦', label: 'Argus 总览', badge: '新' },
  ]},
  { group: 'Argus 管理', items: [
    { key: 'instances', href: 'instances.html', icon: '🖧', label: '实例与参数对比', badge: '新' },
    { key: 'market',   href: 'market.html',   icon: '📈', label: '行情与触发点', badge: '新' },
    { key: 'signals',  href: 'signals.html',  icon: '🎯', label: '信号复盘',     badge: '新' },
    { key: 'backtest', href: 'backtest.html', icon: '🧪', label: '信号回测',     badge: '新' },
    { key: 'optimizer', href: 'optimizer.html', icon: '🎚', label: '后台自动寻优', badge: '新' },
    { key: 'params',   href: 'params.html',   icon: '🎛', label: '参数与运行控制' },
  ]},
  { group: '既有模块（不在本次重构范围）', items: [
    { key: 'x1', href: '#', icon: '📊', label: '真实交易' },
    { key: 'x2', href: '#', icon: '🧮', label: '模拟盘分析' },
    { key: 'x3', href: '#', icon: '🌐', label: '平台 / 币种管理' },
    { key: 'x4', href: '#', icon: '🛡', label: '用户 / 角色管理' },
  ]},
];

const TITLES = {
  dashboard: 'Argus 总览',
  instances: '实例与参数对比',
  market: '历史行情与触发点',
  signals: '信号复盘与持仓生命周期',
  backtest: '盘口信号回测',
  optimizer: '后台自动寻优（参数联合搜索）',
  params: 'Argus 参数与运行控制',
};

const QUICK = [
  { key: 'dashboard', href: 'index.html', icon: '▦', label: '总览' },
  { key: 'instances', href: 'instances.html', icon: '🖧', label: '实例' },
  { key: 'market', href: 'market.html', icon: '📈', label: '行情触发点' },
  { key: 'signals', href: 'signals.html', icon: '🎯', label: '信号复盘' },
  { key: 'backtest', href: 'backtest.html', icon: '🧪', label: '回测' },
  { key: 'optimizer', href: 'optimizer.html', icon: '🎚', label: '寻优' },
  { key: 'params', href: 'params.html', icon: '🎛', label: '参数' },
];

/** allowAll=false 的页面（参数编辑）不允许「全部实例」，必须先选定一个实例 */
function instanceSelector(allowAll) {
  const cur = currentInstanceKey();
  const opts = (allowAll ? [{ key: 'all', name: '全部实例', host: `${INSTANCES.length} 个部署单元` }] : [])
    .concat(INSTANCES);
  return `<div class="inst-picker ${allowAll ? '' : 'strict'}">
    <span class="inst-ico">🖧</span>
    <select id="instPicker">
      ${opts.map(o => `<option value="${o.key}" ${o.key === cur ? 'selected' : ''}>${o.name}${o.host ? ' · ' + o.host : ''}</option>`).join('')}
    </select>
  </div>`;
}

function renderShell(active, allowAll = true) {
  // 参数编辑页不允许「全部实例」：进入时若还停在 all，静默落到第一个实例（不 reload，
  // 否则后续脚本会拿到 undefined 的容器）
  if (!allowAll && currentInstanceKey() === 'all') {
    try { localStorage.setItem(INSTANCE_STORE, INSTANCES[0].key); } catch (e) {}
  }
  const nav = NAV.map(g => {
    const label = g.group ? `<div class="nav-group-label">${g.group}</div>` : '';
    const items = g.items.map(i => `
      <a class="nav-item ${i.key === active ? 'active' : ''}" href="${i.href}"
         ${i.href === '#' ? 'onclick="toast(\'该模块不在本次重构范围内\',\'warn\');return false;"' : ''}>
        <span class="ico">${i.icon}</span><span>${i.label}</span>
        ${i.badge ? `<span class="badge-new">${i.badge}</span>` : ''}
      </a>`).join('');
    return label + items;
  }).join('');

  const quick = QUICK.map(q => `
    <a class="btn ${q.key === active ? 'primary' : ''}" href="${q.href}">
      <span>${q.icon}</span>${q.label}
    </a>`).join('');

  document.body.insertAdjacentHTML('afterbegin', `
    <div class="app">
      <aside class="sidebar">
        <div>
          <div class="brand-kicker">Lightning Trade</div>
          <div class="brand">
            <div class="crest"></div>
            <div class="wordmark"><strong>闪电量化</strong><span>Crypto Futures Console</span></div>
          </div>
        </div>
        <nav class="nav">${nav}</nav>
        <div class="sidebar-foot">
          <span>当前作用域</span>
          <strong>${currentInstance() ? currentInstance().name : `全部实例（${INSTANCES.length}）`}</strong>
          <span class="chip ${currentInstance() ? (currentInstance().version === 37 ? 'ok' : 'warn') : 'info'}">
            <i class="dot"></i>${currentInstance()
              ? `参数 v${currentInstance().version} ${currentInstance().version === 37 ? '已生效' : '落后'}`
              : '实盘 · BTCUSDT 合约'}
          </span>
        </div>
      </aside>
      <div class="main">
        <header class="topbar">
          <div style="min-width:0">
            <div class="topbar-title"><span class="dot">◈</span><b>${TITLES[active] || '管理工作台'}</b></div>
            <div class="quick">${quick}</div>
          </div>
          <div class="row">
            ${instanceSelector(allowAll)}
            <div class="icon-btn" title="告警">🔔</div>
            <div class="user-chip">
              <div class="avatar">A</div>
              <div>
                <div style="font-weight:700">Admin</div>
                <div style="color:var(--manager-text-soft);font-size:12px">超级管理员</div>
              </div>
            </div>
          </div>
        </header>
        <main class="content" id="page"></main>
      </div>
    </div>
    <div class="toasts" id="toasts"></div>
  `);
  const picker = document.getElementById('instPicker');
  if (picker) picker.onchange = e => setInstance(e.target.value);
  return document.getElementById('page');
}

/** 实例徽标：名称 + 机器 + 心跳/版本状态 */
function instChip(inst, showVersion = true) {
  const stale = inst.hbSec > 15;
  return `<span class="chip ${stale ? 'err' : 'ok'}"><i class="dot"></i>${inst.name}</span>
    <span class="chip mute mono">${inst.host}</span>
    ${showVersion ? `<span class="chip ${inst.version === 37 ? 'mute' : 'warn'} mono">参数 v${inst.version}</span>` : ''}`;
}

/* ---------- 反馈组件 ---------- */
function toast(msg, kind = 'ok') {
  const el = document.createElement('div');
  el.className = 'toast ' + kind;
  el.innerHTML = `<span>${kind === 'ok' ? '✅' : kind === 'warn' ? '⚠️' : '⛔'}</span><span>${msg}</span>`;
  document.getElementById('toasts').appendChild(el);
  setTimeout(() => { el.style.opacity = '0'; el.style.transform = 'translateX(20px)'; el.style.transition = '.25s'; }, 2600);
  setTimeout(() => el.remove(), 2950);
}

function ensureMask() {
  let m = document.getElementById('mask');
  if (!m) {
    m = document.createElement('div');
    m.id = 'mask'; m.className = 'mask';
    m.onclick = closeOverlays;
    document.body.appendChild(m);
  }
  return m;
}
function openOverlay(id) {
  ensureMask().classList.add('on');
  document.getElementById(id).classList.add('on');
}
function closeOverlays() {
  ensureMask().classList.remove('on');
  document.querySelectorAll('.drawer.on, .modal.on').forEach(e => e.classList.remove('on'));
}
document.addEventListener('keydown', e => { if (e.key === 'Escape') closeOverlays(); });

/* ---------- 确定性伪随机（保证每次打开原型数据一致） ---------- */
function rng(seed) {
  let s = seed >>> 0;
  return () => {
    s = (s * 1664525 + 1013904223) >>> 0;
    return s / 4294967296;
  };
}

/* ---------- 模拟数据 ---------- */
const ACCOUNTS = [
  { name: '账户A-1394537246@qq.com', short: '账户A', uid: '9542187', cap: 26, variant: 'champion/S400_cap26_gate8' },
  { name: '账户B-mortypeng@gmail.com', short: '账户B', uid: '9613044', cap: 8, variant: 'challenger/S400_cap8' },
];

const GATE_KINDS = {
  gate_block:  { label: '反向门控', chip: 'purple', tri: '#A78BFA' },
  cap_skip:    { label: '上限跳过', chip: 'orange', tri: '#FF9F43' },
  trend_skip:  { label: '趋势闸',   chip: 'info',   tri: '#4D7EFF' },
  open:        { label: '成功开仓', chip: 'ok',     tri: '#0ECB81' },
};

/** 生成 K 线：确定性随机游走，贴近 BTC 7.8 万量级 */
function genCandles({ seed = 7, count = 180, start = Date.UTC(2026, 7, 31, 8, 0, 0), stepMs = 60000, base = 78200 }) {
  const r = rng(seed);
  const out = [];
  let px = base;
  for (let i = 0; i < count; i++) {
    const drift = (r() - 0.47) * 46;
    const o = px;
    const c = px + drift;
    const h = Math.max(o, c) + r() * 26;
    const l = Math.min(o, c) - r() * 26;
    out.push({ t: start + i * stepMs, o, h, l, c, v: 40 + r() * 160 });
    px = c;
  }
  return out;
}

/** 第二数据源：在同一基准上加微小偏离，模拟 DC 与币安的价差 */
function deriveSecondSource(candles, seed = 21) {
  const r = rng(seed);
  return candles.map(k => {
    const d = (r() - 0.5) * 30;
    return { t: k.t, o: k.o + d, h: k.h + d, l: k.l + d, c: k.c + d, v: k.v };
  });
}

/** 偏离序列（bp，带符号）：>0 = UP 信号方向 */
function genDeviation(candles, seed = 33) {
  const r = rng(seed);
  return candles.map(k => {
    let bp = (r() - 0.5) * 6;
    if (r() > 0.93) bp += (r() > 0.5 ? 1 : -1) * (4 + r() * 6);
    return { t: k.t, bp };
  });
}

/** 触发点：从偏离序列里挑超阈时刻，随机分派四种结果 */
function genSignals(candles, dev, seed = 51) {
  const r = rng(seed);
  const out = [];
  let id = 4180;
  let net = 0;
  dev.forEach((d, i) => {
    if (Math.abs(d.bp) < 5) return;
    if (r() > 0.86) return;
    const k = candles[i];
    const dir = d.bp > 0 ? 'UP' : 'DOWN';
    const side = d.bp > 0 ? 'long' : 'short';
    const roll = r();
    let kind, reason = '', gate = null;
    if (roll < 0.52) { kind = 'open'; net += 1; }
    else if (roll < 0.72) { kind = 'gate_block'; const roi = -(40 + r() * 120); gate = { name: '盈利不足', actual: `ROI=${roi.toFixed(1)}%`, op: '<', threshold: '8%' }; reason = `盈利不足 ROI=${roi.toFixed(1)}% < 8%`; }
    else if (roll < 0.88) { kind = 'cap_skip'; gate = { name: '仓位上限', actual: '26 张', op: '≥', threshold: '26 张' }; reason = '已达仓位上限 26 张'; }
    else { kind = 'trend_skip'; const mom = -(5.2 + r() * 6); gate = { name: '趋势闸', actual: mom.toFixed(2) + '%', op: '<', threshold: '-5.00%' }; reason = `24h 窗口动量 ${mom.toFixed(2)}% 逆势`; }
    out.push({
      id: 'sig-' + (id++),
      t: k.t,
      price: k.c,
      dir, side, kind, reason, gate,
      gapBp: d.bp,
      account: ACCOUNTS[r() > 0.45 ? 0 : 1],
      size: kind === 'open' ? 1 : 0,
      net: net,
      configVersion: k.t > Date.UTC(2026, 7, 31, 12, 0, 0) ? 37 : 36,
      source: r() > 0.5 ? 'deepcoin' : 'deepcoin',
    });
  });
  return out;
}

/* ---------- 格式化 ---------- */
const pad = n => String(n).padStart(2, '0');
function fmtTime(ms, withDate = false) {
  const d = new Date(ms);
  const hm = `${pad(d.getUTCHours())}:${pad(d.getUTCMinutes())}:${pad(d.getUTCSeconds())}`;
  return withDate ? `${d.getUTCFullYear()}-${pad(d.getUTCMonth() + 1)}-${pad(d.getUTCDate())} ${hm}` : hm;
}
function fmtPx(v, d = 2) { return v.toLocaleString('en-US', { minimumFractionDigits: d, maximumFractionDigits: d }); }
/** 跨天区间的横轴标签：MM-DD HH:00 */
function fmtDayHour(ms) {
  const d = new Date(ms);
  return `${pad(d.getUTCMonth() + 1)}-${pad(d.getUTCDate())} ${pad(d.getUTCHours())}:00`;
}
function signed(v, d = 2, suffix = '') {
  const s = v >= 0 ? '+' : '';
  return `${s}${v.toFixed(d)}${suffix}`;
}
