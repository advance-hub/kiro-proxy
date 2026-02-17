# kiro-launcher — 技术规格文档

## 项目定位

kiro-launcher 是 kiro-proxy 的桌面 GUI 管理端，基于 Wails v2（Go + React）构建为单一可执行文件。它负责凭据获取与刷新、kiro-rs 代理进程的生命周期管理、FRP 内网穿透、以及自动同步配置到 Droid/OpenCode/Claude Code 等 AI 编程工具。

## 技术栈

| 层 | 技术 |
|---|---|
| 桌面框架 | Wails v2.11（Go 后端 + WebView 前端） |
| 后端 | Go 1.24 |
| 前端 | React 18 + TypeScript 5.5 + Vite 5.4 + Semi Design UI |
| 穿透 | FRP v0.67.0（作为 Go 库嵌入） |
| 代理引擎 | kiro-rs（Rust 二进制，`go:embed` 嵌入并运行时释放） |

## 架构总览

```
┌──────────────────────────────────────────────────┐
│  Wails Desktop App (单一二进制)                    │
│                                                    │
│  ┌─────────────┐    Wails Binding    ┌──────────┐ │
│  │  React 前端  │ ◄═══════════════► │  Go 后端  │ │
│  │  (WebView)   │  window.go.main.App │          │ │
│  └─────────────┘                     └────┬─────┘ │
│                                           │       │
│       ┌───────────────────────────────────┤       │
│       │               │                  │       │
│       ▼               ▼                  ▼       │
│  ┌─────────┐   ┌───────────┐   ┌──────────────┐ │
│  │ kiro-rs │   │ FRP Client│   │ 凭据/配置管理 │ │
│  │ (子进程) │   │ (内嵌库)  │   │              │ │
│  └────┬────┘   └─────┬─────┘   └──────────────┘ │
└───────┼──────────────┼───────────────────────────┘
        │              │
        ▼              ▼
   本地 API 端点    FRP Server / 公网
  localhost:3456    (内网穿透)
```

## Go 后端模块

### app.go — 核心控制器（~1300 行）

`App` 结构体持有代理子进程、隧道状态、日志缓冲区。所有导出方法通过 Wails 绑定暴露给前端 JS。

**代理生命周期**：
- `StartProxy` — 释放嵌入的 kiro-rs 二进制到 `<dataDir>/bin/`（SHA256 内容哈希缓存，仅变更时重写），以 `kiro-rs -c config.json --credentials credentials.json` 启动子进程，管道捕获 stdout/stderr 到 500 行循环日志缓冲区
- `StopProxy` — 优雅终止子进程
- `OneClickStart` — 一键流程：读取 Keychain → 刷新 Token → 写入凭据 → 启动代理
- `GetStatus` / `GetProxyLogs` — 状态查询与日志拉取

**凭据管理**：
- `ImportCredentials` / `SaveCredentialsRaw` — 导入/保存 `credentials.json`
- `ListKeychainSources` — 从 macOS Keychain（`security find-generic-password`）和 `~/.aws/sso/cache/` 发现 Kiro IDE 凭据
- `refreshCredentials` — 双通道刷新：
  - Social（Google）→ `https://prod.{region}.auth.desktop.kiro.dev/refreshToken`
  - IdC（BuilderId/Enterprise）→ `https://oidc.{region}.amazonaws.com/token`

**配置同步**：
- `syncExternalConfigs` — 代理配置变更时自动更新：
  - `~/.factory/config.json` + `~/.factory/settings.json`（Droid/Factory）
  - `~/.config/opencode/opencode.json`（OpenCode）
  - `~/.claude/settings.json`（Claude Code）

**激活系统**：
- `CheckActivation` / `Activate` / `Deactivate` — 基于机器 UUID 的许可证验证，对接 `http://117.72.183.248:7777`

### account.go — 多账号管理（~740 行）

`AccountStore` 是线程安全的 JSON 文件存储（`accounts.json`），单例模式。

- `Account` 结构体：id、email、label、provider（social/idc）、tokens、clientId/secret、usageData
- CRUD：`GetAll`、`Add`（按 email+provider 去重）、`Update`、`Delete`
- `SyncAccount` — 刷新 token + 调用 `getUsageLimits` API 获取配额用量
- `SwitchAccount` — 切换活跃账号，写入 `credentials.json` + `~/.aws/sso/cache/kiro-auth-token.json`
- 批量操作：`ExportAccounts`、`BatchDeleteAccounts`

### tunnel.go — FRP 隧道管理（~400 行）

两种模式：

| 模式 | 说明 |
|---|---|
| FRP 内置 | 创建 FRP `client.Service`，支持 HTTP/TCP 代理类型，连接远程 FRP Server 注册隧道 |
| 外部穿透 | 用户自行运行 ngrok/花生壳等工具，仅存储公网 URL |

- `StartTunnel` — 官方服务器需验证激活码 + 机器 ID（`/api/tunnel/check`）
- 公网 URL 计算：HTTP 模式 `http://{customDomain}:{vhostPort}`，TCP 模式 `http://{serverAddr}:{remotePort}`

### keychain.go — 凭据发现（~330 行）

跨平台凭据源发现：
- macOS：Keychain（`kirocli:oidc:token`、`kirocli:social:token`）
- 通用：`~/.aws/sso/cache/kiro-auth-token*.json`
- Windows：`%LOCALAPPDATA%/kiro/` 或 `~/.kiro/`
- 优先级：IDE 源 > Keychain 源；IdC > Social；更新的 expiresAt 优先

### sidecar.go — 嵌入二进制释放（~70 行）

通过 `go:embed sidecar/*` 嵌入 kiro-rs 编译产物，运行时释放到 `<dataDir>/bin/kiro-rs`，SHA256 哈希校验避免重复写入。

## React 前端

### 布局

左侧 220px 侧边栏 + 右侧内容区，8 个 Tab 页，支持深色/浅色主题切换（localStorage 持久化）。

### 核心页面

| 页面 | 功能 |
|---|---|
| 代理 (ProxyPanel) | 一键启动、运行状态、API 端点展示、模型列表（从代理 `/v1/models` 拉取）、高级配置（host/port/apiKey/region） |
| 穿透 (TunnelPanel) | FRP 内置配置（服务器地址/端口/token/代理类型/自定义域名）或外部穿透 URL，状态指示 + 公网 URL 复制 |
| 账号 (AccountManager) | 搜索/分页/批量选择/批量删除/批量刷新（500ms 间隔顺序执行）、导入（本地 IDE 账号或 JSON 文件）、导出、配额用量展示 |
| 日志 (LogsPanel) | 实时日志（1s 轮询）、ANSI 过滤、自动滚动 |
| Droid (SettingsPanel) | Droid CLI 设置编辑器：模型选择、推理强度、自主级别、diff 模式等 |
| OpenCode | 同步代理模型到 `opencode.json`（`@ai-sdk/anthropic` provider） |
| Claude Code | 同步代理配置到 `~/.claude/settings.json`（环境变量注入） |
| 关于 (AboutPanel) | 激活信息、机器 ID、功能说明 |

### 状态管理

纯 React Hooks（`useState` / `useEffect` / `useCallback`），无外部状态库。各页面独立管理状态，通过 1-5s 轮询间隔与后端保持同步。

## 与 kiro-rs 的交互

```
kiro-launcher (Go)
    │
    ├─ go:embed 嵌入 kiro-rs 二进制
    ├─ 运行时释放到 <dataDir>/bin/kiro-rs
    ├─ 写入 config.json + credentials.json
    ├─ exec.Command 启动子进程
    ├─ 管道捕获 stdout/stderr → 日志缓冲区
    ├─ HTTP 请求 localhost:port/v1/models → 展示模型列表
    └─ 配置变更 → 同步到 Droid/OpenCode/Claude Code
```

kiro-launcher 是管理面板，kiro-rs 是实际的 API 代理引擎。两者通过文件系统（配置/凭据 JSON）和进程管理（spawn/kill）交互，前端通过 HTTP 直接访问代理端点获取运行时信息。
