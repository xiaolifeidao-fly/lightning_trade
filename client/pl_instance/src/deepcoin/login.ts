import { BrowserContext, Page, Request, Response } from 'playwright';
import { DeepCoinEngine } from './engine';
import { generateGoogleAuthCode } from './totp';

const DEFAULT_LOGIN_URL = 'https://www.deepcoin.com/turbo/zh/login';
const DEFAULT_SWAP_TARGET = '/swap/BTCUSDT';

const TIMEOUTS = {
  login: 120_000,
  action: 3_000,
  nav: 30_000,
  locatorProbe: 300,
  dashboardAuth: 8_000,
  dashboardNav: 20_000,
  dashboardProbe: 15_000,
  dashboardPostReload: 3_000,
  afterLogin: 5_000,
};

export interface LoginRequest {
  username: string;
  password: string;
  loginURL?: string;
  googleAuthKey?: string;
  headless?: boolean;
  userDataBaseDir?: string;
  /** Go 侧已验证 cookie 无效时设为 true，跳过 dashboard 探测，直接打开登录页 */
  skipSessionCheck?: boolean;
}

export interface LoginResult {
  resourceId: string;
  loginURL: string;
  finalURL: string;
  cookie: string;
  token: string;
  oToken: string;
  sentryRelease: string;
  sentryPublicKey: string;
  baggage: string;
  storage: Record<string, string>;
  sessionStorage: Record<string, string>;
}

interface AuthState {
  token: string;
  oToken: string;
  baggage: string;
  sentryRelease: string;
  sentryPublicKey: string;
  userStatusCode: number | null;
  userStatusMsg: string;
}

export async function deepCoinLogin(req: LoginRequest): Promise<LoginResult> {
  if (!req.username || !req.password) throw new Error('DeepCoin 登录缺少账号或密码');

  const loginURL = buildLoginURL(req.loginURL, Date.now());
  console.log(`[login] ▶ 开始登录 account=${req.username} headless=false skipSessionCheck=${req.skipSessionCheck ?? false}`);
  console.log(`[login] loginURL=${loginURL}`);

  const engine = new DeepCoinEngine({
    resourceId: req.username,
    headless: true,
    userDataBaseDir: req.userDataBaseDir,
  });

  const ctx = await engine.getOrCreateContext();
  const page = await getOrCreatePage(ctx);
  console.log(`[login] 获取页面 currentURL=${page.url()} isClosed=${page.isClosed()}`);

  page.setDefaultTimeout(TIMEOUTS.action);
  page.setDefaultNavigationTimeout(TIMEOUTS.nav);

  const authState: AuthState = {
    token: '',
    oToken: '',
    baggage: '',
    sentryRelease: '',
    sentryPublicKey: '',
    userStatusCode: null,
    userStatusMsg: '',
  };

  page.on('request', (r: Request) => captureRequest(r, authState));
  page.on('response', (r: Response) => captureResponse(r, authState));

  const activePage = await withTimeout(
    ensureAuthenticated(ctx, page, loginURL, req, authState),
    TIMEOUTS.login,
    'DeepCoin 登录超时，可能卡在验证码或页面加载阶段',
  );

  console.log(`[login] 登录流程完成，开始采集结果 finalURL=${activePage.url()}`);
  await logHeadedEnvFingerprint(activePage);
  const result = await collectResult(ctx, activePage, engine.resourceId, loginURL, authState);

  console.log(`[login] ✅ 登录完成 account=${req.username} cookieLen=${result.cookie.length} tokenLen=${result.token.length} oTokenLen=${result.oToken.length} finalURL=${result.finalURL}`);
  return result;
}

async function ensureAuthenticated(
  ctx: BrowserContext,
  page: Page,
  loginURL: string,
  req: LoginRequest,
  authState: AuthState,
): Promise<Page> {
  if (req.skipSessionCheck) {
    console.log(`[login] skipSessionCheck=true，跳过 dashboard 探测，直接执行登录 account=${req.username}`);
    return performLogin(ctx, page, loginURL, req);
  }

  console.log(`[login] 登录前检查已有 session account=${req.username}`);
  const { page: activePage, reused } = await tryRefreshExistingSession(ctx, page, req, authState);
  if (reused) {
    console.log(`[login] 已复用现有登录态 account=${req.username} url=${activePage.url()}`);
    return activePage;
  }
  console.log(`[login] 未检测到可复用登录态，执行账号密码登录 account=${req.username}`);
  return performLogin(ctx, activePage, loginURL, req);
}

async function tryRefreshExistingSession(
  ctx: BrowserContext,
  page: Page,
  req: LoginRequest,
  authState: AuthState,
): Promise<{ page: Page; reused: boolean }> {
  const dashboardURL = buildDashboardURL(req.loginURL);
  console.log(`[login] 打开 dashboard 检查登录态 url=${dashboardURL}`);

  try {
    await page.goto(dashboardURL, { waitUntil: 'commit', timeout: TIMEOUTS.dashboardNav });
  } catch (err) {
    console.log(`[login] dashboard Goto 完成 err=${err} currentURL=${page.url()}`);
  }

  const { loggedIn, decided } = await waitForDashboardAuthDecision(page, authState, TIMEOUTS.dashboardProbe);
  if (!decided || !loggedIn) {
    console.log(`[login] dashboard 登录态检查未通过 decided=${decided} loggedIn=${loggedIn} url=${page.url()}`);
    return { page, reused: false };
  }

  console.log(`[login] dashboard 登录态有效，刷新页面补充请求头 url=${page.url()}`);
  try {
    await page.reload({ waitUntil: 'commit' });
  } catch { /* ignore */ }

  // 等待更长时间，让页面完成 API 请求（token 会出现在请求头中）
  await page.waitForTimeout(TIMEOUTS.dashboardPostReload);
  await waitForDashboardAuthDecision(page, authState, 8_000);

  // 如果还没有 token，主动跳一次 swap 页触发带 token 的 API 请求
  if (!authState.token && !authState.oToken) {
    console.log(`[login] 未从 dashboard 请求头捕获 token，跳转 swap 页触发认证请求`);
    try {
      await page.goto(buildSwapURL(req.loginURL), { waitUntil: 'commit', timeout: 15_000 });
      await page.waitForTimeout(5_000);
    } catch { /* ignore */ }
  }

  return { page, reused: true };
}

async function performLogin(
  ctx: BrowserContext,
  page: Page,
  loginURL: string,
  req: LoginRequest,
): Promise<Page> {
  // 无头模式下即使 URL 已是登录页，DOM 也可能尚未渲染，必须强制重新导航
  console.log(`[login] 导航到登录页 url=${loginURL} currentURL=${page.url()}`);
  const gotoStart = Date.now();
  try {
    await page.goto(loginURL, { waitUntil: 'domcontentloaded', timeout: 30_000 });
  } catch (err) {
    console.log(`[login] login Goto 完成 err=${err} currentURL=${page.url()}`);
  }
  console.log(`[login] 登录页加载完成 elapsed=${Date.now() - gotoStart}ms url=${page.url()} title=${await page.title().catch(() => '(获取失败)')}`);

  const emailSelectors = [
    `input[placeholder*="邮箱"]`,
    `input[placeholder*="email"]`,
    `input[type="email"]`,
    `input[name="email"]`,
    `input[autocomplete="username"]`,
    `input[type="text"]`,
  ];

  console.log(`[login] 等待账号输入框出现 selectors=${emailSelectors.length}个`);
  await waitForAnyVisible(page, emailSelectors, 25_000);
  console.log(`[login] 账号输入框已出现 url=${page.url()}`);
  await randomSleep(page, '填写账号前');
  await typeIntoFirstVisible(page, emailSelectors, req.username);
  console.log(`[login] 账号已填充 username=${req.username}，点击下一步`);

  await randomSleep(page, '点击下一步前');
  await clickFirstEnabled(page, [
    `button:has-text("下一步")`,
    `text=下一步`,
    `[role="button"]:has-text("下一步")`,
    `button[type="button"]:has-text("下一步")`,
  ]);
  console.log(`[login] 下一步已点击，等待密码输入框 url=${page.url()}`);

  const passwordSelectors = [
    `input[placeholder*="密码"]`,
    `input[placeholder*="password"]`,
    `input[type="password"]`,
    `input[name="password"]`,
    `input[autocomplete="current-password"]`,
  ];

  console.log(`[login] 等待密码输入框出现`);
  await waitForAnyVisible(page, passwordSelectors, 15_000);
  console.log(`[login] 密码输入框已出现 url=${page.url()}`);
  await randomSleep(page, '填写密码前');
  await typeIntoFirstVisible(page, passwordSelectors, req.password);
  console.log(`[login] 密码已填充，点击登录`);

  await randomSleep(page, '点击登录按钮前');
  const loginClicked = await tryClickFirstEnabled(page, [
    `button.dc-btn-primary-primary:has-text("登录")`,
    `button.dc-btn-primary:has-text("登录")`,
    `button:has(span.text-base:text("登录"))`,
    `button:has-text("登录")`,
    `[role="button"]:has-text("登录")`,
    `button:has-text("Next")`,
    `button:has-text("Login")`,
    `[role="button"]:has-text("Next")`,
    `[role="button"]:has-text("Login")`,
  ]);
  if (loginClicked) {
    console.log(`[login] 登录按钮已点击 url=${page.url()}`);
  } else {
    console.log(`[login] 未找到登录按钮，改用 Enter 键提交 url=${page.url()}`);
    await pressEnterOnFirstVisible(page, passwordSelectors);
  }
  console.log(`[login] 登录动作已触发，检查二次验证 url=${page.url()}`);

  await tryHandleGoogleAuth(page, req.googleAuthKey);

  console.log(`[login] 二次验证处理完毕，等待页面稳定 wait=${TIMEOUTS.afterLogin}ms`);
  await page.waitForTimeout(TIMEOUTS.afterLogin);
  console.log(`[login] 页面稳定完成 finalURL=${page.url()} title=${await page.title().catch(() => '(获取失败)')}`);

  return page;
}

async function waitForDashboardAuthDecision(
  page: Page,
  authState: AuthState,
  timeoutMs: number,
): Promise<{ loggedIn: boolean; decided: boolean }> {
  const deadline = Date.now() + timeoutMs;
  let lastLoggedURL = '';

  while (Date.now() < deadline) {
    const currentURL = page.url();
    if (currentURL !== lastLoggedURL) {
      console.log(`[login] dashboard 探测 currentURL=${currentURL}`);
      lastLoggedURL = currentURL;
    }

    if (authState.userStatusCode !== null) {
      const loggedIn = authState.userStatusCode === 0 &&
        authState.userStatusMsg.trim().toLowerCase() === 'ok';
      console.log(`[login] dashboard 收到 user-status code=${authState.userStatusCode} msg=${authState.userStatusMsg}`);
      return { loggedIn, decided: true };
    }

    if (isLoginURL(currentURL)) {
      console.log(`[login] dashboard 已跳转到登录页，判定为未登录`);
      return { loggedIn: false, decided: true };
    }

    await page.waitForTimeout(250);
  }

  if (isLoginURL(page.url())) {
    return { loggedIn: false, decided: true };
  }

  console.log(`[login] dashboard 探测超时，未拿到明确登录态 url=${page.url()}`);
  return { loggedIn: false, decided: false };
}

async function tryHandleGoogleAuth(page: Page, googleAuthKey?: string): Promise<void> {
  if (!googleAuthKey?.trim()) {
    console.log(`[login] 未配置 GoogleAuthKey，跳过二次验证 url=${page.url()}`);
    return;
  }
  // 精确定位：外层容器 .google-auth-container 内的验证码输入框
  const codeInputSelectors = [
    `.google-auth-container input[placeholder="谷歌验证码"]`,
    `.google-auth-container input[type="number"]`,
    `input[placeholder="谷歌验证码"]`,
    `input[placeholder*="谷歌"]`,
    `input[placeholder*="验证码"]`,
  ];

  // 精确定位：.auth-submit-item > .button-container 内 type="submit" 且文字为"提交"的按钮
  const submitSelectors = [
    `.auth-submit-item .button-container button[type="submit"]:has-text("提交")`,
    `.auth-submit-item button[type="submit"]:has-text("提交")`,
    `.button-container button[type="submit"]:has-text("提交")`,
    `button[type="submit"]:has-text("提交")`,
  ];

  console.log(`[login] 等待 Google 验证码输入框出现 url=${page.url()}`);
  let hasInput = false;
  const waitDeadline = Date.now() + 15_000;
  while (Date.now() < waitDeadline) {
    hasInput = await hasAnyVisible(page, codeInputSelectors);
    if (hasInput) break;
    await page.waitForTimeout(300);
  }

  if (!hasInput) {
    console.log(`[login] 未检测到 Google 验证码输入框，跳过 url=${page.url()}`);
    return;
  }

  console.log(`[login] 检测到 Google 验证码输入框`);

  for (let attempt = 1; attempt <= 3; attempt++) {
    console.log(`[login] Google 验证码第 ${attempt} 次尝试`);

    // TOTP 窗口剩余不足 3s 则等到下一个窗口，避免提交时恰好过期
    const nowSec = Math.floor(Date.now() / 1000);
    const remainInWindow = 30 - (nowSec % 30);
    if (remainInWindow < 3) {
      console.log(`[login] TOTP 窗口剩余 ${remainInWindow}s，等待下一个窗口`);
      await page.waitForTimeout(remainInWindow * 1000 + 200);
    }

    const code = generateGoogleAuthCode(googleAuthKey, new Date());
    console.log(`[login] 生成 Google 验证码 code=${code}`);

    await randomSleep(page, '填写 Google 验证码前');
    await typeIntoFirstVisible(page, codeInputSelectors, code);

    // 监听 /auth/check 接口响应，判断验证结果
    let authCheckCode: number | null = null;
    const onResponse = async (resp: Response): Promise<void> => {
      if (!resp.url().includes('/auth/check')) return;
      try {
        const body = await resp.text();
        const payload = JSON.parse(body) as { code: number; msg: string };
        authCheckCode = payload.code;
        console.log(`[login] auth/check code=${payload.code} msg=${payload.msg}`);
      } catch { /* ignore */ }
    };
    page.on('response', onResponse);

    await randomSleep(page, '点击提交验证码前');
    const clicked = await tryClickFirstEnabled(page, submitSelectors, 8_000);
    if (!clicked) {
      page.off('response', onResponse);
      console.log(`[login] 未找到提交按钮，放弃 attempt=${attempt}`);
      break;
    }
    console.log(`[login] 提交按钮已点击，等待 auth/check 响应`);

    // 等待接口返回，最多 10s
    const checkDeadline = Date.now() + 10_000;
    while (Date.now() < checkDeadline && authCheckCode === null) {
      await page.waitForTimeout(200);
    }
    page.off('response', onResponse);

    if (authCheckCode === 0) {
      console.log(`[login] Google 验证通过 attempt=${attempt}`);
      return;
    }

    if (authCheckCode === 19003) {
      console.log(`[login] Google 验证失败 code=19003，重新生成验证码重试`);
      continue;
    }

    // 未收到接口响应时，检查输入框是否消失（页面跳转即视为成功）
    const stillOnVerify = await hasAnyVisible(page, codeInputSelectors);
    if (!stillOnVerify) {
      console.log(`[login] Google 验证码验证通过（页面已跳转）attempt=${attempt}`);
      return;
    }

    console.log(`[login] auth/check 未收到响应，重试 attempt=${attempt}`);
  }

  console.log(`[login] Google 验证码 3 次尝试均未通过`);
}

/** 采集并打印有头模式下浏览器暴露给网站的真实环境参数，切换无头时用于对齐 */
async function logHeadedEnvFingerprint(page: Page): Promise<void> {
  try {
    const env = await page.evaluate(() => {
      const nav = window.navigator as Navigator & Record<string, unknown>;
      const screen = window.screen;
      return {
        // User-Agent
        userAgent: nav.userAgent,
        // Navigator 基础属性
        platform:          nav.platform,
        language:          nav.language,
        languages:         nav.languages?.join(', '),
        hardwareConcurrency: nav.hardwareConcurrency,
        deviceMemory:      (nav as Record<string, unknown>)['deviceMemory'],
        maxTouchPoints:    nav.maxTouchPoints,
        cookieEnabled:     nav.cookieEnabled,
        doNotTrack:        nav.doNotTrack,
        vendor:            nav.vendor,
        // 屏幕信息
        screenWidth:       screen.width,
        screenHeight:      screen.height,
        screenColorDepth:  screen.colorDepth,
        screenPixelDepth:  screen.pixelDepth,
        availWidth:        screen.availWidth,
        availHeight:       screen.availHeight,
        devicePixelRatio:  window.devicePixelRatio,
        // 窗口大小
        innerWidth:        window.innerWidth,
        innerHeight:       window.innerHeight,
        outerWidth:        window.outerWidth,
        outerHeight:       window.outerHeight,
        // 时区
        timezone:          Intl.DateTimeFormat().resolvedOptions().timeZone,
        timezoneOffset:    new Date().getTimezoneOffset(),
        // WebGL 渲染器（反检测关键）
        webglRenderer: (() => {
          try {
            const canvas = document.createElement('canvas');
            const gl = canvas.getContext('webgl') || canvas.getContext('experimental-webgl');
            if (!gl) return '(无法获取)';
            const ext = (gl as WebGLRenderingContext).getExtension('WEBGL_debug_renderer_info');
            if (!ext) return '(无扩展)';
            return (gl as WebGLRenderingContext).getParameter(ext.UNMASKED_RENDERER_WEBGL);
          } catch { return '(异常)'; }
        })(),
        webglVendor: (() => {
          try {
            const canvas = document.createElement('canvas');
            const gl = canvas.getContext('webgl') || canvas.getContext('experimental-webgl');
            if (!gl) return '(无法获取)';
            const ext = (gl as WebGLRenderingContext).getExtension('WEBGL_debug_renderer_info');
            if (!ext) return '(无扩展)';
            return (gl as WebGLRenderingContext).getParameter(ext.UNMASKED_VENDOR_WEBGL);
          } catch { return '(异常)'; }
        })(),
        // Chrome 特征
        chromeRuntime:     typeof (window as unknown as Record<string, unknown>)['chrome'] !== 'undefined',
        // Automation 检测
        webdriver:         nav.webdriver,
      };
    });

    console.log('[env] ══════════ 有头模式浏览器环境参数（无头切换时对齐用）══════════');
    console.log(`[env] userAgent          = ${env.userAgent}`);
    console.log(`[env] platform           = ${env.platform}`);
    console.log(`[env] vendor             = ${env.vendor}`);
    console.log(`[env] language           = ${env.language}`);
    console.log(`[env] languages          = ${env.languages}`);
    console.log(`[env] hardwareConcurrency= ${env.hardwareConcurrency}`);
    console.log(`[env] deviceMemory       = ${env.deviceMemory} GB`);
    console.log(`[env] maxTouchPoints     = ${env.maxTouchPoints}`);
    console.log(`[env] cookieEnabled      = ${env.cookieEnabled}`);
    console.log(`[env] doNotTrack         = ${env.doNotTrack}`);
    console.log(`[env] webdriver          = ${env.webdriver}  ← 无头下必须为 false/undefined`);
    console.log(`[env] chromeRuntime      = ${env.chromeRuntime}  ← 无头下必须为 true`);
    console.log(`[env] screen             = ${env.screenWidth}x${env.screenHeight} colorDepth=${env.screenColorDepth} pixelDepth=${env.screenPixelDepth}`);
    console.log(`[env] availScreen        = ${env.availWidth}x${env.availHeight}`);
    console.log(`[env] devicePixelRatio   = ${env.devicePixelRatio}`);
    console.log(`[env] window.inner       = ${env.innerWidth}x${env.innerHeight}`);
    console.log(`[env] window.outer       = ${env.outerWidth}x${env.outerHeight}`);
    console.log(`[env] timezone           = ${env.timezone}  offset=${env.timezoneOffset}min`);
    console.log(`[env] webglVendor        = ${env.webglVendor}`);
    console.log(`[env] webglRenderer      = ${env.webglRenderer}`);
    console.log('[env] ════════════════════════════════════════════════════════════════');
  } catch (err) {
    console.warn(`[env] 环境参数采集失败: ${err}`);
  }
}

async function collectResult(
  ctx: BrowserContext,
  page: Page,
  resourceId: string,
  loginURL: string,
  authState: AuthState,
): Promise<LoginResult> {
  const cookies = await ctx.cookies();
  const cookieParts = cookies
    .filter(c => c.name)
    .map(c => `${c.name}=${c.value}`);
  const cookieString = cookieParts.join('; ');


  const storage = await page.evaluate<Record<string, string>>(`
    (() => {
      const r = {};
      for (let i = 0; i < localStorage.length; i++) {
        const k = localStorage.key(i);
        r[k] = localStorage.getItem(k) ?? '';
      }
      return r;
    })()
  `);

  const sessionStorage = await page.evaluate<Record<string, string>>(`
    (() => {
      const r = {};
      for (let i = 0; i < sessionStorage.length; i++) {
        const k = sessionStorage.key(i);
        r[k] = sessionStorage.getItem(k) ?? '';
      }
      return r;
    })()
  `);


  const result: LoginResult = {
    resourceId,
    loginURL,
    finalURL: page.url(),
    cookie: cookieString,
    token: authState.token,
    oToken: authState.oToken,
    sentryRelease: authState.sentryRelease,
    sentryPublicKey: authState.sentryPublicKey,
    baggage: authState.baggage,
    storage,
    sessionStorage,
  };

  enrichFromStorage(result);
  console.log(`[collect] enrichFromStorage 完成 token=${result.token ? '✅' : '❌'} oToken=${result.oToken ? '✅' : '❌'} sentryRelease=${result.sentryRelease || '(空)'}`);

  // 尝试从 cookie 中直接解析 otoken（部分站点会将 token 写入非 httpOnly cookie）
  if (!result.token && !result.oToken) {
    const oTokenCookie = cookies.find(c =>
      c.name.toLowerCase() === 'otoken' ||
      c.name.toLowerCase() === 'token' ||
      c.name.toLowerCase() === 'access_token'
    );
    if (oTokenCookie) {
      result.oToken = oTokenCookie.value;
      result.token = oTokenCookie.value;
    }
  }

  console.log(`[collect] 采集完毕 cookieLen=${result.cookie.length} tokenLen=${result.token.length} oTokenLen=${result.oToken.length} storageKeys=${Object.keys(result.storage).length} sessionStorageKeys=${Object.keys(result.sessionStorage).length}`);

  if (!result.cookie) {
    throw new Error('登录态采集完成，但未采集到 cookie；大概率仍停留在验证码或二次校验阶段');
  }

  if (!result.token && !result.oToken) {
    console.warn(`[collect] ⚠️  未采集到 otoken/token，后续 Web 下单可能受影响`);
  }

  return result;
}

// ────────────────────────────────────────────────
// Auth state capture helpers
// ────────────────────────────────────────────────

function captureRequest(request: Request, state: AuthState): void {
  const headers = request.headers();
  if (!headers) return;

  // HTTP/2 所有 header 名强制小写
  const oToken = headers['otoken'];
  if (oToken) {
    console.log(`[capture] otoken header captured url=${request.url().slice(0, 80)}`);
    state.oToken = oToken;
  }

  const token = headers['token'];
  if (token) {
    console.log(`[capture] token header captured url=${request.url().slice(0, 80)}`);
    state.token = token;
  }

  // Authorization: Bearer xxx
  const auth = headers['authorization'];
  if (auth?.startsWith('Bearer ')) {
    const bearer = auth.slice('Bearer '.length).trim();
    if (bearer) {
      console.log(`[capture] Authorization Bearer captured url=${request.url().slice(0, 80)}`);
      state.token = state.token || bearer;
      state.oToken = state.oToken || bearer;
    }
  }

  const baggage = headers['baggage'];
  if (baggage) {
    state.baggage = baggage;
    const { release, publicKey } = parseSentryFields(baggage);
    if (release) state.sentryRelease = release;
    if (publicKey) state.sentryPublicKey = publicKey;
  }
}

async function captureResponse(response: Response, state: AuthState): Promise<void> {
  if (!response.url().includes('/wealth/myb/user-status')) return;
  try {
    const body = await response.text();
    const payload = JSON.parse(body) as { code: number; msg: string };
    state.userStatusCode = payload.code;
    state.userStatusMsg = payload.msg;
  } catch { /* ignore */ }
}

// ────────────────────────────────────────────────
// Playwright interaction helpers
// ────────────────────────────────────────────────

async function getOrCreatePage(ctx: BrowserContext): Promise<Page> {
  const pages = ctx.pages();

  for (const p of pages) {
    if (!p.isClosed() && p.url().includes('deepcoin.com')) return p;
  }
  for (const p of pages) {
    if (!p.isClosed()) return p;
  }

  return ctx.newPage();
}

async function waitForAnyVisible(page: Page, selectors: string[], timeoutMs: number): Promise<void> {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    for (const selector of selectors) {
      const locator = page.locator(selector).first();
      try {
        const visible = await locator.isVisible({ timeout: TIMEOUTS.locatorProbe });
        if (visible) return;
      } catch { /* continue */ }
    }
    await page.waitForTimeout(250);
  }
  throw new Error(`等待元素超时 selectors=${selectors.join(',')}`);
}

async function hasAnyVisible(page: Page, selectors: string[]): Promise<boolean> {
  for (const selector of selectors) {
    try {
      const visible = await page.locator(selector).first().isVisible({ timeout: TIMEOUTS.locatorProbe });
      if (visible) return true;
    } catch { /* continue */ }
  }
  return false;
}

async function typeIntoFirstVisible(page: Page, selectors: string[], value: string): Promise<void> {
  for (const selector of selectors) {
    console.log(`[login] 尝试输入框 selector=${selector}`);
    try {
      const locator = page.locator(selector).first();
      const visible = await locator.isVisible({ timeout: TIMEOUTS.locatorProbe });
      if (!visible) continue;

      console.log(`[login] 命中输入框 selector=${selector}`);
      await locator.click();
      await locator.press('ControlOrMeta+A');
      await locator.press('Backspace');
      await locator.pressSequentially(value);
      return;
    } catch (err) {
      console.log(`[login] 输入框失败 selector=${selector} err=${err}`);
    }
  }
  throw new Error('未找到可用输入框');
}

async function tryClickFirstEnabled(page: Page, selectors: string[], timeoutMs = 8_000): Promise<boolean> {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    for (const selector of selectors) {
      try {
        const locator = page.locator(selector).first();
        const visible = await locator.isVisible({ timeout: TIMEOUTS.locatorProbe });
        if (!visible) continue;

        const enabled = await locator.isEnabled({ timeout: TIMEOUTS.locatorProbe });
        if (!enabled) continue;

        console.log(`[login] 命中按钮 selector=${selector}`);
        await locator.click();
        return true;
      } catch { /* continue */ }
    }
    await page.waitForTimeout(250);
  }
  return false;
}

async function clickFirstEnabled(page: Page, selectors: string[]): Promise<void> {
  const clicked = await tryClickFirstEnabled(page, selectors, 15_000);
  if (!clicked) throw new Error('未找到可点击按钮');
}

async function pressEnterOnFirstVisible(page: Page, selectors: string[]): Promise<void> {
  for (const selector of selectors) {
    try {
      const locator = page.locator(selector).first();
      const visible = await locator.isVisible({ timeout: TIMEOUTS.locatorProbe });
      if (!visible) continue;
      console.log(`[login] Enter 键提交 selector=${selector}`);
      await locator.press('Enter');
      return;
    } catch { /* continue */ }
  }
  console.log(`[login] 未找到密码框兜底，直接按全局 Enter`);
  await page.keyboard.press('Enter');
}

// ────────────────────────────────────────────────
// URL / Storage helpers
// ────────────────────────────────────────────────

function buildLoginURL(raw: string | undefined, nowMs: number): string {
  const base = raw?.trim() || DEFAULT_LOGIN_URL;
  try {
    const u = new URL(base);
    u.searchParams.set('status', 'login');
    u.searchParams.set('timeStamp', String(nowMs));
    if (!u.searchParams.get('target')) u.searchParams.set('target', DEFAULT_SWAP_TARGET);
    return u.toString();
  } catch {
    return base;
  }
}

function buildSwapURL(raw: string | undefined): string {
  const base = raw?.trim() || DEFAULT_LOGIN_URL;
  try {
    const u = new URL(base);
    u.pathname = DEFAULT_SWAP_TARGET;
    u.search = '';
    u.hash = '';
    return u.toString();
  } catch {
    return 'https://www.deepcoin.com/swap/BTCUSDT';
  }
}

function buildDashboardURL(raw: string | undefined): string {
  const base = raw?.trim() || DEFAULT_LOGIN_URL;
  try {
    const u = new URL(base);
    u.pathname = '/turbo/zh/my/dashboard';
    u.search = '';
    u.hash = '';
    return u.toString();
  } catch {
    return 'https://www.deepcoin.com/turbo/zh/my/dashboard';
  }
}

function isLoginURL(url: string): boolean {
  try {
    return new URL(url).pathname.includes('/login');
  } catch {
    return url.includes('/login');
  }
}

function parseSentryFields(baggage: string): { release: string; publicKey: string } {
  let release = '';
  let publicKey = '';
  for (const part of baggage.split(',')) {
    const piece = part.trim();
    if (piece.startsWith('sentry-release=')) release = piece.slice('sentry-release='.length);
    if (piece.startsWith('sentry-public_key=')) publicKey = piece.slice('sentry-public_key='.length);
  }
  return { release, publicKey };
}

function firstNonEmpty(...values: string[]): string {
  return values.find(v => v.trim()) ?? '';
}

function enrichFromStorage(result: LoginResult): void {
  const lookup = (...keys: string[]): string => {
    for (const key of keys) {
      const v = result.storage[key]?.trim();
      if (v) return v;
      const sv = result.sessionStorage[key]?.trim();
      if (sv) return sv;
    }
    return '';
  };

  result.token = firstNonEmpty(result.token, lookup('otoken', 'token', 'TOKEN', 'accessToken', 'access_token'));
  result.oToken = firstNonEmpty(result.oToken, lookup('otoken', 'token', 'TOKEN', 'accessToken', 'access_token'), result.token);
  result.sentryRelease = firstNonEmpty(result.sentryRelease, lookup('sentryRelease', 'SentryRelease', 'sentry-release'));
  result.sentryPublicKey = firstNonEmpty(result.sentryPublicKey, lookup('SentryPublicKey', 'sentryPublicKey', 'sentry-public_key'));

  if (!result.baggage) result.baggage = lookup('baggage');
  if (result.baggage) {
    const { release, publicKey } = parseSentryFields(result.baggage);
    result.sentryRelease = firstNonEmpty(result.sentryRelease, release);
    result.sentryPublicKey = firstNonEmpty(result.sentryPublicKey, publicKey);
  }
}

// ────────────────────────────────────────────────
// Utility
// ────────────────────────────────────────────────

function randomSleepMs(minMs = 1_000, maxMs = 4_000): number {
  return Math.floor(Math.random() * (maxMs - minMs + 1)) + minMs;
}

async function randomSleep(page: Page, label: string): Promise<void> {
  const ms = randomSleepMs();
  console.log(`[login] 随机等待 ${label} delay=${ms}ms`);
  await page.waitForTimeout(ms);
}

function withTimeout<T>(promise: Promise<T>, ms: number, msg: string): Promise<T> {
  return new Promise((resolve, reject) => {
    const timer = setTimeout(() => reject(new Error(msg)), ms);
    promise.then(
      v => { clearTimeout(timer); resolve(v); },
      e => { clearTimeout(timer); reject(e); },
    );
  });
}
