# ClawPanel Lite Release Checklist

## 目标

这份清单用于避免 Lite 发版时再次出现“包能安装、面板能启动，但 OpenClaw runtime 实际不可用”的倒退问题。

Lite 的发布底线不是“能打包”，而是：

- 能安装
- 能启动面板
- 能启动 OpenClaw
- 能启动网关
- 能正常进入配置与通道页面

## 发布原则

### 1. 先保证能跑，再考虑瘦身

- 未完成完整 smoke test 前，不要裁剪 OpenClaw runtime
- 不要为了减小包体，先删 `node_modules`、内置扩展或运行时资源，再寄希望于线上补救

### 2. Lite 的核心是 bundled runtime

Lite 是否可发布，首先看：

- `runtime/openclaw`
- `runtime/node`
- `bin/clawlite-openclaw`

而不是先看前端页面是否正常。

### 3. 能打包不等于能发布

必须以“全新机器安装成功并运行正常”为准，不能只因为：

- `tar.gz` 成功生成
- GitHub Actions 绿了
- 前端 build 通过

就认为 Lite 可以发布。

### 4. Linux 稳定链路优先

- Linux Lite 是当前正式可用链路
- Windows / macOS Lite 处于预览推进阶段
- 推进 Windows/macOS 时，不要破坏 Linux 已稳定链路

### 5. 运行参数属于发行物的一部分

Lite 运行所需的关键环境变量必须写进安装脚本/服务定义，例如：

- `CLAWPANEL_EDITION=lite`
- `CLAWPANEL_DATA=/opt/clawpanel-lite/data`
- `NODE_OPTIONS=--max-old-space-size=2048`

不能依赖线上临时热修。

## 禁止事项

### 不要随意裁掉这些内容

- `runtime/openclaw/node_modules`
- `runtime/openclaw/dist`
- `runtime/openclaw/extensions/memory-core`
- Lite 默认依赖的其他 bundled 扩展

尤其是：

- `memory-core`

它不是“可选插件”，而是默认 memory slot 可能依赖的组件。删掉后会直接导致配置校验失败。

### 不要把跨平台 Linux 拼包当正式方案

- 不要继续用 Linux 主机硬拼 Windows/macOS Lite runtime 作为正式发布方案
- Windows/macOS Lite 应优先使用 GitHub Actions 原生 runner 构建

## 发版前必须检查

### A. 包结构检查

确认 Lite Core 包至少包含：

- `clawpanel-lite` / `clawpanel-lite.exe`
- `bin/clawlite-openclaw` / `bin/clawlite-openclaw.cmd`
- `runtime/node`
- `runtime/openclaw`
- `data/openclaw-config/openclaw.json`

### B. Runtime 完整性检查

检查这些关键项：

- `runtime/openclaw/dist/entry.js` 或 `entry.mjs` 存在
- `runtime/openclaw/node_modules` 存在并可解析关键依赖
- `runtime/openclaw/extensions/memory-core` 存在
- 默认 bundled 插件目录结构完整

### C. 命令行 smoke test

发版前至少跑：

```bash
clawlite-openclaw --version
clawlite-openclaw gateway
```

要求：

- 不出现 `ERR_MODULE_NOT_FOUND`
- 不出现 `missing dist/entry.(m)js`
- 不出现 `plugin not found: memory-core`

### D. 安装后健康检查

安装完成后至少验证：

- `systemctl status clawpanel-lite` / `launchd` / Windows Service 正常
- 面板 `/api/status` 返回：
  - `configured=true`
  - `processRunning=true`
  - `gatewayRunning=true`
  - `state=healthy`

### E. 全新机器验证

每次正式 Lite 发布前，至少在一台全新环境验证：

- 安装
- 首次打开面板
- 配置 API Key
- 打开一个通道
- 发送一条消息

## 更新策略要求

Lite 更新继续遵循：

- 整包更新
- 保留 `data/`
- 替换：
  - 主程序
  - `bin/`
  - `runtime/`
- 失败自动回滚

不要把 Lite 再退回到“只更新面板二进制”的模式。

## 当前已知经验

### 已踩过的坑

1. 为了减小包体，裁掉 runtime 依赖，导致缺 `chalk`
2. 裁掉 `memory-core`，导致 `plugins.slots.memory` 校验失败
3. 未给 gateway 足够内存，导致 Node OOM
4. 把“GitHub Actions 成功打包”误当成“可正式发布”

### 当前推荐做法

- Linux Lite：正式链路，严格 smoke test 后发版
- Windows/macOS Lite：先由 GitHub Actions 原生 runner 构建和验证，再决定是否正式推广

## 发布责任判断标准

只有同时满足下面四条，Lite 才算“可发布”：

1. 包结构完整
2. `clawlite-openclaw` 命令可运行
3. OpenClaw + 网关能正常启动
4. 面板安装后主流程（配置 API Key / 配通道 / 发消息）可用
