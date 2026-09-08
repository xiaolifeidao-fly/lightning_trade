const argEnvIndex = process.argv.indexOf('--env')
let argEnv = (argEnvIndex !== -1 && process.argv[argEnvIndex + 1]) || ''

// prod 档按实际部署机收敛：那台机器总内存 1.7G、可用 ~1.3G，且 argus_single
// 常驻其上。原来的 instances:4 + 1000M 会直接把机器打爆，连带拖垮实盘进程。
// 单实例 fork 模式足够撑管理端流量，需要扩容时先加内存再调这里。
const RUN_ENV_MAP = {
  local: {
    instances: 2,
    max_memory_restart: '250M'
  },
  dev: {
    instances: 2,
    max_memory_restart: '250M'
  },
  prod: {
    instances: 1,
    max_memory_restart: '400M'
  }
}

if (!(argEnv in RUN_ENV_MAP)) {
  argEnv = 'prod'
}

module.exports = {
  apps: [
    {
      name: 'next-admin',
      // next.config.mjs 用 output:'standalone'，产物自带 server.js，不再走
      // node_modules/next/dist/bin/next。端口由 PORT 环境变量给，不是 -p 参数。
      script: 'server.js',
      instances: RUN_ENV_MAP[argEnv].instances,
      exec_mode: RUN_ENV_MAP[argEnv].instances > 1 ? 'cluster' : 'fork',
      watch: false,
      max_memory_restart: RUN_ENV_MAP[argEnv].max_memory_restart,
      env: {
        PORT: 9701,
        // 绑 0.0.0.0，否则 standalone 默认只听 localhost，对外访问不到。
        HOSTNAME: '0.0.0.0'
      },
      env_local: {
        APP_ENV: 'local'
      },
      env_dev: {
        APP_ENV: 'dev'
      },
      env_prod: {
        APP_ENV: 'prod'
      }
    }
  ]
}