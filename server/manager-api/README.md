## IP模块
```
新建表 参考Java中的模块
ip容器 对照 参考Java manager-api的 com.manager-api.business.query.tools.QueryIpService#getIp
```

## 设备模块
```
表已有 参考Java中的模块
设备容器 参考 Java的manager-api  com.manager-api.business.query.tools.QueryDeviceSupport#afterPropertiesSet
```

## 代理IP模块
```
使httpclient支持代理连接
```

// TODO: 下面获取webDeviceDTO的逻辑要抽取出来一个模块，从里面获取一个设备，每次尽可能获取不一样的设备，轮询使用
// TODO: web_device 表增加IP字段, 一个web_device 对应一个IP，这个IP失效的话，重新更新
// TODO: 获取IP的逻辑要抽取出来一个模块，从里面获取一个IP，每次尽可能获取不一样的IP，轮询使用
