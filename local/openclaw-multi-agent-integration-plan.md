# OpenClaw Multi-Agent 完整对接实施方案（ClawPanel）

更新时间：2026-03-04  
适用仓库：`/Users/sky/GitHub/ClawPanel`

## 1. 目标与范围

目标：让 ClawPanel 从“单 Agent 管理面板”升级为“可完整管理 OpenClaw 官方多智能体能力”的控制台，覆盖：

1. 多 Agent 配置管理（`agents.list`、default、每 Agent 模型/工具/沙箱）。
2. bindings 规则管理与顺序控制（含路由预览）。
3. 会话与定时任务可按 Agent 操作（不再固定 `main`）。
4. 配置保存时不破坏官方关键字段（尤其 `tools`、`session`）。
5. 上线前具备可回滚和可验证机制。

不在本期范围：

1. 重写 OpenClaw 路由内核（只做面板对接，不替代引擎逻辑）。
2. 改造 OpenClaw 官方 CLI 行为（仅调用和适配）。

官方文档 https://docs.openclaw.ai/zh-CN/concepts/multi-agent 和 https://docs.openclaw.ai/concepts/multi-agent

## 2. 当前差距（需修复）

现状问题（代码证据）：

1. 保存配置时会删除顶层 `tools` / `session`，会破坏多智能体关键配置。
   - `internal/handler/openclaw.go`
2. 运行时模型补丁仅处理 `agents/main/agent/models.json`，非多 Agent 设计。
   - `internal/handler/openclaw.go`
3. 会话页面默认只拉取主 Agent（前端未提供 Agent 切换）。
   - `web/src/pages/Sessions.tsx`
4. 新建 Cron 任务固定 `sessionTarget: "main"`。
   - `web/src/pages/CronJobs.tsx`
5. 配置页只覆盖 `agents.defaults.*`，没有 `agents.list`/bindings 可视化管理。
   - `web/src/pages/SystemConfig.tsx`

## 3. 总体设计

## 3.1 设计原则

1. 配置保真优先：不擅自删除 OpenClaw 官方字段。
2. 向后兼容：默认行为不改变（默认 Agent 仍为 `main`）。
3. 风险隔离：所有“写配置”动作支持 dry-run 预检和备份。
4. 可观测：关键路径输出操作日志（谁改了什么）。

## 3.2 实施阶段

### Phase P0（阻塞项修复，必须先做）

1. 停止在 `GET/PUT /openclaw/config` 中删除 `tools` 和 `session`。
2. `patchModelsJSON` 改为遍历所有 Agent 目录（从 `agents.list` 和磁盘目录双来源兜底）。
3. 配置读写支持 JSON5（含注释、尾逗号），写回保留稳定格式（必要时落盘为标准 JSON，并在 UI 给出提示）。
4. 新增“写入前自动备份”：
   - `openclaw.json` -> `backups/pre-edit-<timestamp>.json`

验收：

1. 含 `tools.agentToAgent` 的配置通过面板保存后不丢失。
2. 多个 Agent 均能生成/更新对应 models 配置。

### Phase P1（后端多智能体控制 API）

新增 API（建议挂在 `/api/openclaw/agents*`）：

1. `GET /api/openclaw/agents`
   - 返回：`defaults`、`list`、`bindings`、统计信息（sessions/lastActive）。
2. `POST /api/openclaw/agents`
   - 新建 Agent（校验 `id`、`workspace`、`agentDir` 唯一性）。
3. `PUT /api/openclaw/agents/:id`
   - 更新 Agent 配置。
4. `DELETE /api/openclaw/agents/:id`
   - 删除 Agent（提供 `preserveSessions=true` 选项）。
5. `GET /api/openclaw/bindings`
6. `PUT /api/openclaw/bindings`
   - 全量替换 bindings（保序）。
7. `POST /api/openclaw/route/preview`
   - 输入 message meta，输出命中 Agent 与匹配规则路径。

已有 API 增强：

1. `GET /api/sessions?agent=all`：聚合所有 Agent 会话并带 `agentId` 字段。
2. `GET /api/sessions/:id?agent=<id>`：保留现有逻辑，补充 agent 参数校验。
3. `PUT /api/system/cron`：保存前校验 `sessionTarget` 是否存在于 Agent 列表。

### Phase P2（前端多智能体 UI）

新增页面：

1. `Agents`（建议路由 `/agents`）：
   - Agent 列表（default 标识、启停状态、会话数、最后活跃时间）。
   - 新建/编辑弹窗（model、tools、sandbox、workspace、agentDir）。
   - bindings 可视化编辑（增删改 + 拖拽排序）。
   - 路由预览器（输入 sender/channel/guild/team/peer，展示命中结果）。

改造现有页面：

1. `Sessions`：
   - 顶部 Agent 下拉：`main` / `all` / 其它 Agent。
2. `CronJobs`：
   - 新建任务增加 `sessionTarget` 选择器，默认 `main`，可选任意 Agent。
3. `SystemConfig`：
   - 增加“高级 JSON 预览（只读）”和“差异预览（写入前）”。

### Phase P3（CLI 对齐与运维能力）

1. 可选提供“CLI 对齐模式”：
   - 后端封装 `openclaw agents list --bindings --json` 读取对照（若 CLI 可用）。
2. 增加配置校验接口：
   - 保存前执行 schema 校验 + 路由规则冲突检查。
3. 新增“重启策略选项”：
   - 仅写配置不重启 / 写配置后重启 gateway。

### Phase P4（测试、发布、回滚）

1. 单元测试：
   - bindings 匹配顺序测试。
   - `agent=all` 会话聚合测试。
   - 配置保真测试（保存前后字段不丢失）。
2. 集成测试：
   - 创建 home/work 双 Agent + 规则路由 + 会话隔离。
3. 回滚机制：
   - 最近 10 次配置快照一键恢复。

## 4. 数据模型建议

后端内部结构（Go）建议增加：

```go
type OpenClawAgentsConfig struct {
    Defaults map[string]any   `json:"defaults"`
    List     []AgentItem      `json:"list"`
    Bindings []BindingRule    `json:"bindings"`
}

type AgentItem struct {
    ID        string         `json:"id"`
    Workspace string         `json:"workspace,omitempty"`
    AgentDir  string         `json:"agentDir,omitempty"`
    Model     map[string]any `json:"model,omitempty"`
    Tools     map[string]any `json:"tools,omitempty"`
    Sandbox   map[string]any `json:"sandbox,omitempty"`
    Default   bool           `json:"default,omitempty"`
}

type BindingRule struct {
    Name      string         `json:"name,omitempty"`
    Match     map[string]any `json:"match"`
    Agent     string         `json:"agent"`
    Enabled   bool           `json:"enabled"`
}
```

说明：

1. 不强行限制官方字段，只对关键字段做最小校验。
2. 非识别字段保留，避免未来版本升级丢配置。

## 5. API 草案（可直接开工）

### 5.1 获取多智能体配置

`GET /api/openclaw/agents`

响应示例：

```json
{
  "ok": true,
  "agents": {
    "defaults": { "model": { "primary": "openai/gpt-4o" } },
    "list": [
      { "id": "main", "default": true, "workspace": "/data/work/main" },
      { "id": "work", "workspace": "/data/work/work" }
    ],
    "bindings": [
      { "name": "work-group", "enabled": true, "match": { "channel": "qq", "groupId": "123" }, "agent": "work" }
    ]
  }
}
```

### 5.2 路由预览

`POST /api/openclaw/route/preview`

请求示例：

```json
{
  "meta": {
    "channel": "qq",
    "peer": "group:123",
    "guildId": "",
    "teamId": "",
    "accountId": "10001"
  }
}
```

响应示例：

```json
{
  "ok": true,
  "result": {
    "agent": "work",
    "matchedBy": "bindings[0].match.peer",
    "trace": [
      "check peer",
      "hit bindings[0]"
    ]
  }
}
```

## 6. 执行清单（按优先级）

P0：

1. 修改 `internal/handler/openclaw.go`（移除 tools/session 删除逻辑）。
2. 修改 `internal/handler/openclaw.go`（models patch 多 Agent 化）。
3. 修改 `internal/config/config.go`（JSON5 读写支持）。
4. 加入配置备份助手函数并接入写入路径。

P1：

1. 新建 `internal/handler/agents.go`（agents/bindings/preview API）。
2. 在 `cmd/clawpanel/main.go` 注册新路由。
3. 扩展 `internal/handler/sessions.go` 支持 `agent=all`。
4. 在 `internal/handler/skills.go` 的 cron 保存增加 `sessionTarget` 校验。

P2：

1. 新建 `web/src/pages/Agents.tsx`。
2. 扩展 `web/src/lib/api.ts` 增加 agents/bindings/preview API。
3. 修改 `web/src/components/Layout.tsx` 增加菜单入口。
4. 修改 `web/src/pages/Sessions.tsx` 和 `web/src/pages/CronJobs.tsx`。

P3/P4：

1. 增加后端与前端测试。
2. 增加 release note 与迁移说明。

## 7. 风险与回滚

风险：

1. 旧配置格式不规范导致解析失败。
2. bindings 新旧语义差异导致路由偏移。
3. 会话聚合导致页面性能下降。

对策：

1. 写入前备份 + dry-run 校验。
2. 路由预览器上线前强制开启“只读模式”一周观察。
3. 会话列表默认分页与时间窗口限制。

回滚：

1. 后端保留 `LEGACY_SINGLE_AGENT=true` 环境变量（出现问题可一键切回旧行为）。
2. 前端通过 feature flag 隐藏 `Agents` 页面。

## 8. 验收标准（DoD）

1. 面板可创建至少 2 个 Agent，并设置默认 Agent。
2. bindings 可排序、启停、保存，重启网关后生效。
3. 路由预览结果与实际消息路由一致。
4. Sessions 支持按 Agent 查看和 all 聚合。
5. Cron 任务可投递到指定 Agent。
6. 保存配置后 `tools/session/agents.list/bindings` 均不丢失。

## 9. 建议实施顺序（最小可用）

1. 先做 P0（配置保真）并发布内部测试版。
2. 再做 P1（后端 API）并写自动化测试。
3. 最后做 P2（前端可视化），一次性打通用户体验。

---

如需，我可以基于本文件继续产出下一份：`local/openclaw-multi-agent-task-breakdown.md`（按文件粒度拆成可直接开发的 task list + 预估工时）。
