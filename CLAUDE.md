# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 项目概述

ClawPanel 是 OpenClaw（多模型 AI 助手引擎）的全栈管理面板。Go 后端 + React 前端，构建为单一静态二进制文件（前端通过 `go:embed` 嵌入）。主要面向中文用户，支持中英双语。

## 常用命令

### 构建

```bash
make build          # 完整构建（前端 + 后端），输出到 build/
make frontend       # 仅构建前端（npm install + vite build，输出到 cmd/clawpanel/frontend/dist/）
make backend        # 构建后端（含前端嵌入）
make backend-only   # 仅构建后端（假设前端已构建）
make cross          # 交叉编译 5 个平台（linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64）
make release        # 构建所有发布产物
make clean          # 清理构建产物
```

### 开发模式

前端和后端需分别启动：

```bash
# 前端（端口 5173，自动代理 /api 和 /ws 到 :19527）
cd web && npm run dev

# 后端（端口 19527）
go run ./cmd/clawpanel/
```

### 前端

```bash
cd web
npm install         # 安装依赖
npm run dev         # 开发服务器
npm run build       # TypeScript 类型检查 + Vite 生产构建
```

### 注意事项

- 构建使用 `CGO_ENABLED=0`（纯 Go SQLite 驱动 `modernc.org/sqlite`，无需 CGO）
- 版本号通过 ldflags 注入：`-X main.Version=x.x.x`
- 无测试文件、无 lint 配置、无 CI/CD

## 架构

### 技术栈

| 层 | 技术 |
|---|---|
| 后端 | Go 1.24, Gin, SQLite (WAL), gorilla/websocket, golang-jwt |
| 前端 | React 18, TypeScript, TailwindCSS, Vite 6, react-router-dom v6 |
| 部署 | 单二进制（`go:embed` 嵌入前端），默认端口 19527 |

### 后端结构

- **`cmd/clawpanel/main.go`** — 入口，Gin 路由注册，服务器启动，通过 `//go:embed all:frontend/dist` 嵌入前端
- **`internal/config/`** — 配置加载（环境变量 + `clawpanel.json`），OpenClaw 路径自动检测
- **`internal/model/`** — SQLite 数据库初始化，Event/Settings CRUD
- **`internal/handler/`** — 18 个 Gin handler 文件，按功能划分（auth、openclaw、process、plugin、bot 等）
- **`internal/middleware/`** — JWT 认证、CORS、请求日志
- **`internal/websocket/`** — WebSocket hub，广播到所有客户端（日志、事件、状态）
- **`internal/process/`** — OpenClaw 进程生命周期管理（启动/停止/重启、日志捕获、守护进程检测）
- **`internal/plugin/`** — 插件管理（npm/git/archive/本地安装）
- **`internal/update/`** — 面板自更新（下载 + SHA256 校验 + 替换二进制）
- **`internal/updater/`** — 独立更新服务（端口 19528），进程隔离以确保更新期间可靠完成
- **`internal/eventlog/`** — OneBot11 事件监听 + 系统事件记录
- **`internal/monitor/`** — NapCat 连接状态监控
- **`internal/taskman/`** — 异步任务管理器（软件安装等）

### 前端结构

- **`web/src/App.tsx`** — 路由定义（9 个页面：Dashboard、Logs、Channels、Skills、Plugins、CronJobs、Sessions、Workspace、Config）
- **`web/src/lib/api.ts`** — API 客户端（fetch 封装，所有 REST 端点）
- **`web/src/lib/mockApi.ts`** — Demo 模式 mock 数据（`VITE_DEMO=true` 启用）
- **`web/src/hooks/useWebSocket.ts`** — WebSocket hook（指数退避重连）
- **`web/src/hooks/useAuth.ts`** — 认证 hook（localStorage token）
- **`web/src/i18n/`** — 国际化（zh-CN、en）
- **`web/src/components/Layout.tsx`** — 导航侧栏、暗色/亮色主题、语言切换

### 关键设计模式

1. **单二进制部署**：前端 `vite build` 输出到 `cmd/clawpanel/frontend/dist/`，由 Go 通过 `go:embed` 嵌入，运行时作为静态文件服务，SPA 路由 fallback 到 `index.html`
2. **单管理员认证**：JWT HS256，7 天过期，无多用户，密码存储在 SQLite settings 表
3. **WebSocket 广播**：单 hub 向所有客户端广播进程日志、事件、NapCat 状态、任务进度
4. **进程管理**：OpenClaw 使用 daemon 模式，manager 通过端口监控检测 daemon fork，支持自动重启
5. **Windows 服务支持**：通过 `//go:build windows` / `//go:build !windows` 构建标签分离平台代码
6. **OpenClaw 配置自动修补**：进程启动前自动修补 `openclaw.json` 确保 gateway.mode、QQ 通道、插件配置正确

### API 路由结构

所有 REST 端点在 `/api/` 下，主要分组：
- `/api/auth/*` — 登录、改密（login 公开）
- `/api/openclaw/*` — OpenClaw 配置、模型、通道
- `/api/process/*` — 进程管理
- `/api/plugins/*` — 插件中心
- `/api/bot/*` — Bot 操作（群组、好友、发送）
- `/api/napcat/*` — NapCat QQ 登录、状态
- `/api/workspace/*` — 文件管理器
- `/api/sessions/*` — 会话管理
- `/api/system/*` — 系统环境、备份、更新
- `/api/panel/*` — 面板自更新
- `/ws` — WebSocket

### 配置

主配置文件 `clawpanel.json` 位于数据目录（默认 `./data`），可通过环境变量覆盖：
- `CLAWPANEL_PORT`（默认 19527）
- `CLAWPANEL_DATA`（默认 `./data`）
- `OPENCLAW_DIR`（默认 `~/.openclaw`）
- `ADMIN_TOKEN`（默认 `clawpanel`）
- `CLAWPANEL_SECRET`（JWT 密钥）
- `CLAWPANEL_DEBUG`

### 插件系统

插件注册表在 `plugins/registry.json`，支持安装来源：npm 包、Git 仓库、压缩包、本地目录。插件开发规范见 `docs/plugin-dev/`。
