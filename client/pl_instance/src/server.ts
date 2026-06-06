import express, { Request, Response, NextFunction } from 'express';
import { deepCoinLogin, LoginRequest } from './deepcoin/login';
import { DeepCoinEngine } from './deepcoin/engine';

const PORT = Number(process.env.PORT ?? 8765);
const app = express();
app.use(express.json());

// ────────────────────────────────────────────────
// Routes
// ────────────────────────────────────────────────

app.get('/health', (_req: Request, res: Response) => {
  res.json({ ok: true, ts: new Date().toISOString() });
});

/**
 * POST /api/deepcoin/login
 *
 * Body:
 *   {
 *     "username":     "user@example.com",   // required
 *     "password":     "p@ssw0rd",           // required
 *     "loginURL":     "https://...",        // optional
 *     "googleAuthKey":"BASE32SECRET",       // optional
 *     "headless":     false                 // optional, default false
 *   }
 *
 * Response (success):
 *   {
 *     "ok": true,
 *     "data": { cookie, token, oToken, sentryRelease, sentryPublicKey, baggage, storage, sessionStorage, ... }
 *   }
 *
 * Response (error):
 *   { "ok": false, "error": "..." }
 */
app.post('/api/deepcoin/login', async (req: Request, res: Response) => {
  const body = req.body as Partial<LoginRequest>;

  if (!body.username || !body.password) {
    res.status(400).json({ ok: false, error: '缺少 username 或 password' });
    return;
  }

  const loginReq: LoginRequest = {
    username: body.username,
    password: body.password,
    loginURL: body.loginURL,
    googleAuthKey: body.googleAuthKey,
    headless: body.headless ?? false,
    userDataBaseDir: body.userDataBaseDir,
    skipSessionCheck: body.skipSessionCheck ?? false,
  };

  try {
    console.log(`[server] DeepCoin 登录请求 username=${loginReq.username}`);
    const result = await deepCoinLogin(loginReq);
    res.json({ ok: true, data: result });
  } catch (err) {
    const msg = err instanceof Error ? err.message : String(err);
    console.error(`[server] DeepCoin 登录失败 username=${loginReq.username} err=${msg}`);
    res.status(500).json({ ok: false, error: msg });
  }
});

/**
 * POST /api/deepcoin/close-context
 *
 * Body: { "username": "user@example.com", "headless": false }
 * 关闭指定账号的 Playwright context（清理浏览器实例）
 */
app.post('/api/deepcoin/close-context', async (req: Request, res: Response) => {
  const { username, headless } = req.body as { username?: string; headless?: boolean };
  if (!username) {
    res.status(400).json({ ok: false, error: '缺少 username' });
    return;
  }
  try {
    const engine = new DeepCoinEngine({ resourceId: username, headless: headless ?? false });
    await engine.closeContext();
    console.log(`[server] DeepCoin context 已关闭 username=${username}`);
    res.json({ ok: true });
  } catch (err) {
    const msg = err instanceof Error ? err.message : String(err);
    res.status(500).json({ ok: false, error: msg });
  }
});

// ────────────────────────────────────────────────
// Error handler
// ────────────────────────────────────────────────

app.use((err: Error, _req: Request, res: Response, _next: NextFunction) => {
  console.error('[server] 未捕获错误:', err);
  res.status(500).json({ ok: false, error: err.message });
});

// ────────────────────────────────────────────────
// Start
// ────────────────────────────────────────────────

app.listen(PORT, () => {
  console.log(`[server] pl-instance 启动 port=${PORT}`);
  console.log(`[server] POST /api/deepcoin/login`);
  console.log(`[server] POST /api/deepcoin/close-context`);
  console.log(`[server] GET  /health`);
});
