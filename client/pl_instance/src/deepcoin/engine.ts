import * as fs from 'fs';
import * as path from 'path';
import * as os from 'os';
import { chromium, BrowserContext } from 'playwright';

const DEFAULT_WIDTH = 1440;
const DEFAULT_HEIGHT = 900;

// key -> BrowserContext
const contextMap = new Map<string, BrowserContext>();

export interface EngineOptions {
  resourceId: string;
  headless?: boolean;
  chromePath?: string;
  width?: number;
  height?: number;
  userDataBaseDir?: string;
}

export class DeepCoinEngine {
  readonly resourceId: string;
  private readonly headless: boolean;
  private readonly chromePath: string;
  private readonly width: number;
  private readonly height: number;
  private readonly userDataBaseDir: string;

  constructor(opts: EngineOptions) {
    this.resourceId = normalizeResourceId(opts.resourceId);
    this.headless = true;
    this.chromePath = opts.chromePath ?? resolveChromePath();
    this.width = opts.width ?? DEFAULT_WIDTH;
    this.height = opts.height ?? DEFAULT_HEIGHT;
    this.userDataBaseDir = opts.userDataBaseDir ?? path.join(process.cwd(), 'configs', 'playwright');
  }

  async getOrCreateContext(): Promise<BrowserContext> {
    const key = this.contextKey();
    const cached = contextMap.get(key);
    if (cached) {
      try {
        await cached.cookies(); // liveness check
        console.log(`[engine] 复用已有 context resourceId=${this.resourceId} headless=${this.headless}`);
        return cached;
      } catch {
        contextMap.delete(key);
        console.warn(`[engine] 检测到已失效 context，重建 resourceId=${this.resourceId}`);
      }
    }
    return this.createContext(key);
  }

  async closeContext(): Promise<void> {
    const key = this.contextKey();
    const ctx = contextMap.get(key);
    contextMap.delete(key);
    if (ctx) await ctx.close();
  }

  private async createContext(key: string): Promise<BrowserContext> {
    const userDataDir = this.userDataDir();
    fs.mkdirSync(userDataDir, { recursive: true });
    clearChromeLockFiles(userDataDir);

    // ── 有头模式启动参数（当前生效）─────────────────────────────────────
    // headless: false
    // args 无额外反检测参数
    // ── 无头模式切换时，将下方 headless 改为 true，并追加 headlessArgs ──
    // const headlessArgs = [
    //   '--headless=new',                          // 新版无头引擎，渲染与有头一致
    //   '--disable-gpu',                           // 无头必加，避免 GPU 初始化崩溃
    //   '--hide-scrollbars',                       // 隐藏滚动条，避免截图差异
    //   '--mute-audio',                            // 静音
    //   '--blink-settings=imagesEnabled=true',     // 保持图片加载（登录页需要）
    //   '--disable-features=VizDisplayCompositor', // 无头下关闭合成器
    // ];
    // ─────────────────────────────────────────────────────────────────────

    // 有头模式与无头模式共用的 UA（来自有头实测）
    const REAL_UA = 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/148.0.0.0 Safari/537.36';

    const launchOpts: Parameters<typeof chromium.launchPersistentContext>[1] = {
      headless: this.headless,
      locale: 'zh-CN',
      timezoneId: 'Asia/Shanghai',
      viewport: { width: this.width, height: this.height },
      bypassCSP: true,
      userAgent: REAL_UA,
      // 无头模式注入真实的 HTTP 请求头，绕过 CloudFront/WAF 的特征拦截
      // 有头模式下这些头浏览器会自动生成，但无头下需要手动补全
      extraHTTPHeaders: this.headless ? {
        'accept-language':          'zh-CN,zh;q=0.9',
        'sec-ch-ua':                '"Chromium";v="148","Google Chrome";v="148","Not-A.Brand";v="99"',
        'sec-ch-ua-mobile':         '?0',
        'sec-ch-ua-platform':       '"macOS"',
        'sec-fetch-dest':           'document',
        'sec-fetch-mode':           'navigate',
        'sec-fetch-site':           'none',
        'sec-fetch-user':           '?1',
        'upgrade-insecure-requests':'1',
      } : {},
      args: [
        '--no-sandbox',
        '--disable-setuid-sandbox',
        '--disable-dev-shm-usage',
        '--disable-blink-features=AutomationControlled',
        '--disable-background-timer-throttling',
        '--disable-renderer-backgrounding',
        '--disable-backgrounding-occluded-windows',
        '--no-first-run',
        '--no-default-browser-check',
        '--disable-default-apps',
        `--window-size=${this.width},${this.height}`,
        // 切换无头时在此展开 headlessArgs：...headlessArgs
      ],
    };
    if (this.chromePath) {
      launchOpts.executablePath = this.chromePath;
    }

    console.log(`[engine] 启动浏览器 headless=${this.headless} resourceId=${this.resourceId}`);
    console.log(`[engine] userDataDir=${userDataDir}`);
    console.log(`[engine] chromePath=${launchOpts.executablePath ?? '(playwright内置)'}`);
    console.log(`[engine] viewport=${this.width}x${this.height} locale=${launchOpts.locale} tz=${launchOpts.timezoneId}`);
    console.log(`[engine] args=${(launchOpts.args ?? []).join(' ')}`);

    const ctx = await chromium.launchPersistentContext(userDataDir, launchOpts);

    // 无头模式：注入有头环境的真实指纹，覆盖无头下暴露的自动化特征
    if (this.headless) {
      await ctx.addInitScript(() => {
        // ── 以下所有值均来自有头模式实测采集 ──────────────────────────────

        // Navigator
        const NAV_UA               = 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/148.0.0.0 Safari/537.36';
        const NAV_PLATFORM         = 'MacIntel';
        const NAV_VENDOR           = 'Google Inc.';
        const NAV_LANGUAGE         = 'zh-CN';
        const NAV_LANGUAGES        = ['zh-CN'];
        const NAV_HW_CONCURRENCY   = 14;
        const NAV_DEVICE_MEMORY    = 32;
        const NAV_MAX_TOUCH_POINTS = 0;
        // Screen
        const SCREEN_WIDTH         = 1440;
        const SCREEN_HEIGHT        = 900;
        const SCREEN_AVAIL_WIDTH   = 1440;
        const SCREEN_AVAIL_HEIGHT  = 900;
        const SCREEN_COLOR_DEPTH   = 30;
        const SCREEN_PIXEL_DEPTH   = 30;
        // Window
        const WIN_DEVICE_PIXEL_RATIO = 1;
        const WIN_INNER_WIDTH      = 1440;
        const WIN_INNER_HEIGHT     = 900;
        const WIN_OUTER_WIDTH      = 1442;
        const WIN_OUTER_HEIGHT     = 1021;
        // WebGL
        const WEBGL_VENDOR         = 'Google Inc. (Apple)';
        const WEBGL_RENDERER       = 'ANGLE (Apple, ANGLE Metal Renderer: Apple M4 Pro, Unspecified Version)';

        // ── Navigator 覆盖 ──────────────────────────────────────────────
        const navProto = Object.getPrototypeOf(navigator);
        const def = (key: string, value: unknown) =>
          Object.defineProperty(navProto, key, { get: () => value, configurable: true });

        def('userAgent',           NAV_UA);
        def('platform',            NAV_PLATFORM);
        def('vendor',              NAV_VENDOR);
        def('language',            NAV_LANGUAGE);
        def('languages',           NAV_LANGUAGES);
        def('hardwareConcurrency', NAV_HW_CONCURRENCY);
        def('deviceMemory',        NAV_DEVICE_MEMORY);
        def('maxTouchPoints',      NAV_MAX_TOUCH_POINTS);
        def('webdriver',           false);  // 无头核心：消除自动化标记

        // ── window.chrome（无头下缺失，网站以此检测）──────────────────
        if (!(window as unknown as Record<string, unknown>)['chrome']) {
          Object.defineProperty(window, 'chrome', {
            value: { runtime: {} },
            configurable: true,
            writable: true,
          });
        }

        // ── Screen 覆盖 ─────────────────────────────────────────────────
        const screenDef = (key: string, value: number) =>
          Object.defineProperty(screen, key, { get: () => value, configurable: true });

        screenDef('width',       SCREEN_WIDTH);
        screenDef('height',      SCREEN_HEIGHT);
        screenDef('availWidth',  SCREEN_AVAIL_WIDTH);
        screenDef('availHeight', SCREEN_AVAIL_HEIGHT);
        screenDef('colorDepth',  SCREEN_COLOR_DEPTH);
        screenDef('pixelDepth',  SCREEN_PIXEL_DEPTH);

        // ── window 属性覆盖 ─────────────────────────────────────────────
        Object.defineProperty(window, 'devicePixelRatio', { get: () => WIN_DEVICE_PIXEL_RATIO, configurable: true });
        Object.defineProperty(window, 'innerWidth',       { get: () => WIN_INNER_WIDTH,        configurable: true });
        Object.defineProperty(window, 'innerHeight',      { get: () => WIN_INNER_HEIGHT,       configurable: true });
        Object.defineProperty(window, 'outerWidth',       { get: () => WIN_OUTER_WIDTH,        configurable: true });
        Object.defineProperty(window, 'outerHeight',      { get: () => WIN_OUTER_HEIGHT,       configurable: true });

        // ── WebGL UNMASKED 覆盖 ─────────────────────────────────────────
        const origGetParam = WebGLRenderingContext.prototype.getParameter;
        WebGLRenderingContext.prototype.getParameter = function (this: WebGLRenderingContext, pname: number): unknown {
          const ext = this.getExtension('WEBGL_debug_renderer_info');
          if (ext) {
            if (pname === ext.UNMASKED_VENDOR_WEBGL)   return WEBGL_VENDOR;
            if (pname === ext.UNMASKED_RENDERER_WEBGL) return WEBGL_RENDERER;
          }
          return origGetParam.call(this, pname);
        };
      });
      console.log(`[engine] 🛡️  无头指纹注入完成 resourceId=${this.resourceId}`);
    }

    contextMap.set(key, ctx);
    console.log(`[engine] ✅ 持久化 context 已创建 resourceId=${this.resourceId} userDataDir=${userDataDir}`);
    return ctx;
  }

  private contextKey(): string {
    return `${this.headless}|deepcoin|${this.resourceId}|${this.chromePath}`;
  }

  private userDataDir(): string {
    const mode = this.headless ? 'headless' : 'headed';
    return path.join(this.userDataBaseDir, 'userDataDir', 'deepcoin', mode, this.resourceId);
  }

}

function clearChromeLockFiles(dir: string): void {
  for (const name of ['lockfile', 'SingletonLock', 'SingletonCookie', 'SingletonSocket']) {
    try { fs.unlinkSync(path.join(dir, name)); } catch { /* ignore */ }
  }
}

function resolveChromePath(): string {
  const envPath = process.env.CHROME_PATH?.trim();
  if (envPath && fs.existsSync(envPath)) return envPath;

  const home = os.homedir();
  const candidates: string[] = [];

  switch (process.platform) {
    case 'darwin':
      candidates.push(
        '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
        '/Applications/Google Chrome Beta.app/Contents/MacOS/Google Chrome Beta',
        '/Applications/Google Chrome Dev.app/Contents/MacOS/Google Chrome Dev',
        '/Applications/Google Chrome Canary.app/Contents/MacOS/Google Chrome Canary',
      );
      break;
    case 'win32':
      candidates.push(
        'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe',
        'C:\\Program Files (x86)\\Google\\Chrome\\Application\\chrome.exe',
        path.join(home, 'AppData', 'Local', 'Google', 'Chrome', 'Application', 'chrome.exe'),
      );
      break;
    default:
      candidates.push(
        '/usr/bin/google-chrome',
        '/usr/bin/google-chrome-stable',
        '/usr/bin/chromium-browser',
        '/usr/bin/chromium',
      );
  }

  return candidates.find(c => fs.existsSync(c)) ?? '';
}

function normalizeResourceId(id: string): string {
  return id.trim().replace(/[/\\:*?"<>| ]/g, '_') || 'default';
}
