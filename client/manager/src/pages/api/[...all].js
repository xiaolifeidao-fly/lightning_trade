// pages/api/[...all].js
import axios from "axios";
import { constants } from "buffer";
import { createProxyMiddleware } from "http-proxy-middleware";
import { headers } from "next/headers";
import formidable from 'formidable';
import FormData from 'form-data';
import fs from 'fs';

require('dotenv').config();

const prefix = process.env.APP_URL_PREFIX;
const target = process.env.SERVER_TARGET;


/**
 * 代理中间件只建一次。
 *
 * 原来是**每个 GET 请求新建一个** createProxyMiddleware。http-proxy-middleware v3
 * 每次创建都会往 Node 的 HTTP Server 上挂一个 close 监听器，而这个监听器永远不会
 * 被摘掉——生产日志里的
 *   `MaxListenersExceededWarning: 11 close listeners added to [Server]`
 * 就是它。本地实测 15 个请求就复现。
 *
 * 总览页 10 秒一轮、每轮 8 个请求，一个标签页挂一小时就是 2880 个请求，也就是
 * 2880 个常驻监听器（每个还闭包持有那次请求的 options 与 headers）。挂得越久越糟，
 * 正好对上「一开始好好的，放一会就开始 ERR_EMPTY_RESPONSE」。
 *
 * 用惰性单例而不是模块顶层直接建：target 来自环境变量，构建期 import 这个模块时
 * 它可能还是空的，顶层建会在构建阶段就抛。
 */
let sharedProxy = null;
function getProxy() {
  if (!sharedProxy) {
    sharedProxy = createProxyMiddleware({
      target: target, // 设置代理目标地址
      changeOrigin: true, // 设置请求头中的 Host 为目标地址的 Host
      pathRewrite: {
        "^/api": prefix, // 将请求中的 /api 前缀替换为空字符串
      },
      // 不再传 headers: req.headers —— http-proxy 本来就会把原请求头原样转发给
      // 上游，这个选项是"额外追加的头"，传进去只是把同样的值再设一遍；而它是
      // 唯一逐请求变化的选项，去掉它代理才能复用。
      // http-proxy-middleware v3 **删掉了**顶层的 onError / onProxyReq / onProxyRes，
      // 改成 on: { error, proxyReq, proxyRes }。之前那份写法在 v3 下是一个被忽略的
      // 无效选项——结果是代理出错时日志里一行都没有，浏览器只拿到
      // net::ERR_EMPTY_RESPONSE，事后完全没法定位。这里按 v3 的写法重新挂上，
      // 至少把「哪个 URL、什么错误码」落到 pm2 日志里。
      on: {
        error: (err, req, res) => {
          console.error(
            `[proxy] ${new Date().toISOString()} ${req?.method} ${req?.url} -> ${err?.code || err?.message}`,
          );
          if (res && typeof res.writeHead === 'function' && !res.headersSent) {
            res.writeHead(502, { 'Content-Type': 'application/json' });
            res.end(JSON.stringify({ success: false, message: `proxy error: ${err?.code || 'unknown'}` }));
          }
        },
      },
    });
  }
  return sharedProxy;
}

export default async function handler(req, res) {
  if(req.method == 'GET'){
    return getProxy()(req, res);
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
  if(method === 'POST'){
    // 普通 POST
    console.log("request url is ", url);
    const response = await axios.post(url, req.body, { headers });
    return response;
  }
  if(method === 'PUT'){
    return await axios.put(url, req.body, {  headers});
  }
  if(method === 'DELETE'){
    return await axios.delete(url, { params: req.body, headers});
  }
  return null;
}

function getTargetUrl(url){
  url = url.replace("/api",prefix)
  return target  + url;
}