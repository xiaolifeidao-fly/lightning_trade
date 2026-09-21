import http from "http";

/**
 * 到 manager-api 的上游连接池。**全进程只有这一个 agent。**
 *
 * 为什么需要它（2026-09-21 线上事故的直接产物）：
 *
 * 那天这台机器上所有服务的网络一起瘫了——ping 通、TCP 握手偶尔成功、但握手后
 * 一个字节都收不到。根因不在 nginx，在我们自己的 next-server：
 *
 *   next-server → 127.0.0.1:8491 的 ESTAB 连接   445 条
 *   其中 Recv-Q 非空                             413 条（单条最大 4.7MB，合计 113MB）
 *   manager-api 那一侧却只认                      48 条  ← 约 400 条是半开的孤儿
 *   没有任何 timer 的                             387 条  ← 永远不会被回收
 *   内核 tcp_mem 上限 150MB，实际占用              162MB  ← 长期超标
 *
 * 一旦 TCP 内存超过 tcp_mem 的 max，内核就拒绝给**任何** socket 分配缓冲区，
 * 于是整台机器看起来像消失了。重启 next-server 后：连接 445→0、内核 TCP 162MB→1MB、
 * 可用内存 51MB→410MB。
 *
 * 机制是：响应堆在接收队列里没人读 → 接收窗口为 0 → 对端的 FIN 根本塞不进来 →
 * 内核这边永远停在 ESTAB。**没有超时能救它，只有重启。**
 *
 * strace 抓到的触发者是「数据总览」的刷新批次（每轮 8 个接口），其中这一个是大头：
 *
 *   /api/argus-event/timeline?interval=1m&start=<30天前>&end=<现在>
 *                            &platformCode=deepcoin&comparePlatformCode=binance
 *
 * 30 天 × 1440 分钟 × 2 个平台 ≈ 4.3 万个点，**单次响应 4.7MB**。一轮刷新还没传完，
 * 下一轮又压上来（trace 里同一批请求重复出现两次），旧的那份就被丢在 socket 里。
 *
 * 所以这里是**兜底，不是根治**——根治要把 timeline 的粒度/窗口降下来，
 * 像 equity-curve 那样按 bucketSeconds 聚合，别把 4.3 万个点整包传给浏览器。
 * 在那之前，这几个参数负责把最坏情况变成有限且能自愈的：
 *
 *   - maxSockets  硬上限。出事那次要是有这一条，最多 24 条而不是 445 条。
 *                 不能定太小：总览一轮并发 8 个请求，多开两个标签页就顶到了。
 *   - timeout     socket 上的**不活动**超时；proxyTimeout 才是真正会销毁请求的那个
 *                 （见两个路由文件）。取 2 分钟：4.7MB 走 loopback 传输只要几百毫秒，
 *                 慢的是 manager-api 那边生成，2 分钟足够，同时保证**有限**——
 *                 原来是无限，那才是事故的根。
 *   - keepAlive   复用连接，并给空闲池一个上限。
 *
 * 最坏从"445 条永久泄漏、吃光内核 150MB 预算"变成"最多 24 条（约 110MB）、2 分钟自愈"。
 * 24 这个数字仍然不舒服，正说明必须去改 timeline —— 兜底兜的是事故，不是设计。
 */
export const upstreamAgent = new http.Agent({
  keepAlive: true,
  keepAliveMsecs: 15_000,
  maxSockets: 24,
  maxFreeSockets: 8,
  timeout: 120_000,
});

/**
 * 上游请求超时（毫秒）。
 *
 * 给 http-proxy 的 proxyTimeout/timeout 和 axios 用同一个值，免得两条路径的行为不一致。
 * 取 2 分钟而不是常见的 30 秒：回测、timeline 这类接口本来就慢，宁可放宽也不要误杀；
 * 关键是**有限**。proxyTimeout 超时会销毁上游请求，这正是 445 条孤儿连接缺的那一下。
 */
export const UPSTREAM_TIMEOUT_MS = 120_000;
