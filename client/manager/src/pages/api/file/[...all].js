// pages/api/[...all].js
import axios from "axios";
import { constants } from "buffer";
import { createProxyMiddleware } from "http-proxy-middleware";
import { headers } from "next/headers";
import formidable from 'formidable';
import FormData from 'form-data';
import fs from 'fs';
import { upstreamAgent, UPSTREAM_TIMEOUT_MS } from '@/utils/upstreamAgent';

require('dotenv').config();

const prefix = process.env.APP_URL_PREFIX;
const target = process.env.SERVER_TARGET;

// Next.js API 路由处理函数

// 添加这个配置来禁用默认的 body 解析
export const config = {
  api: {
    bodyParser: false
  }
}

/**
 * 代理中间件只建一次 —— 与 pages/api/[...all].js 同样的处理，原因也一样。
 *
 * 这里原来是**每个 GET 请求新建一个** createProxyMiddleware。http-proxy-middleware v3
 * 每次创建都往 Node 的 HTTP Server 上挂一个 close 监听器，而且永远不摘，挂得越久越糟。
 * 主代理那条路早就改成惰性单例了，这条是仓库里最后一处漏网的。
 *
 * 顺带修掉两个静默失效的写法：
 *   - headers: req.headers  这是"额外追加的请求头"，不是"转发原请求头"（http-proxy
 *     本来就会转发）。它是唯一逐请求变化的选项，正是它挡住了代理复用。
 *   - onError / onProxyReq / onProxyRes  是 v2 的选项名，v3 已经删掉，改成
 *     on: { error, proxyReq, proxyRes }。写在这里的三个回调**一次都没被调用过**——
 *     所以代理出错时日志里一行都没有，浏览器只看到 net::ERR_EMPTY_RESPONSE。
 */
let sharedFileProxy = null;
function getFileProxy() {
  if (!sharedFileProxy) {
    sharedFileProxy = createProxyMiddleware({
      target: target, // 设置代理目标地址
      changeOrigin: true, // 设置请求头中的 Host 为目标地址的 Host
      pathRewrite: {
        "^/api": prefix, // 将请求中的 /api 前缀替换为空字符串
      },
      // 上游连接封顶与超时，见 utils/upstreamAgent 里的事故记录。
      agent: upstreamAgent,
      proxyTimeout: UPSTREAM_TIMEOUT_MS,
      timeout: UPSTREAM_TIMEOUT_MS,
      on: {
        error: (err, req, res) => {
          console.error(
            `[file-proxy] ${new Date().toISOString()} ${req?.method} ${req?.url} -> ${err?.code || err?.message}`,
          );
          if (res && typeof res.writeHead === 'function' && !res.headersSent) {
            res.writeHead(502, { 'Content-Type': 'application/json' });
            res.end(JSON.stringify({ success: false, message: `proxy error: ${err?.code || 'unknown'}` }));
          }
        },
      },
    });
  }
  return sharedFileProxy;
}

export default async function handler(req, res) {
  if(req.method == 'GET'){
    return getFileProxy()(req, res);
  }
  try {
    const url = getTargetUrl(req.url);
    const response = await request(url, req)
    // 获取目标服务器的响应
    const data = response.data;
    // 将目标服务器的响应返回给客户端
    res.status(response.status).json(data);
  } catch (error) {
      console.error('Error forwarding request:', error);
      res.status(500).json({ error: 'Internal Server Error' });
  }
}

async function request(url, req){
  const method = req.method;
  const headers = req.headers;
  // 与代理那条路共用同一个 agent：axios 默认的 globalAgent 既无连接上限也无超时。
  const opts = { httpAgent: upstreamAgent, timeout: UPSTREAM_TIMEOUT_MS };
  if(method === 'POST'){
    const contentType = headers['content-type'];
    const isMultiPart = contentType?.includes('multipart/form-data');
    if (isMultiPart) {
      // formidable 解析
      const form = formidable({ multiples: true });
      const [fields, files] = await new Promise((resolve, reject) => {
        form.parse(req, (err, fields, files) => {
          if (err) reject(err);
          resolve([fields, files]);
        });
      });

      // 用 form-data 组装
      const formData = new FormData();

      // 添加文件
      for (const [key, value] of Object.entries(files)) {
        if (Array.isArray(value)) {
          value.forEach(file => {
            formData.append(key, fs.createReadStream(file.filepath), file.originalFilename);
          });
        } else {
          formData.append(key, fs.createReadStream(value.filepath), value.originalFilename);
        }
      }

      // 添加其他字段
      for (const [key, value] of Object.entries(fields)) {
        // formidable 解析出来的 value 可能是数组
        if (Array.isArray(value)) {
          value.forEach(v => formData.append(key, v));
        } else {
          formData.append(key, value);
        }
      }

      // 合并 headers
      const formHeaders = formData.getHeaders();
      const mergedHeaders = { ...headers, ...formHeaders };
      // 去掉 content-length，form-data 会自动处理
      delete mergedHeaders['content-length'];
      url = url.replace("/file", "")
      // 发送
      const response = await axios.post(url, formData, { headers: mergedHeaders, ...opts });
      return response;
    }

    // 普通 POST
    const response = await axios.post(url, req.body, { headers, ...opts });
    return response;
  }
  if(method === 'PUT'){
    return await axios.put(url, req.body, { headers, ...opts });
  }
  if(method === 'DELETE'){
    return await axios.delete(url, { params: req.body, headers, ...opts });
  }
  return null;
}

function getTargetUrl(url){
  url = url.replace("/api",prefix)
  return target  + url;
}