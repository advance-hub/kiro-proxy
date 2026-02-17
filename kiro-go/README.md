# Kiro-Go — 统一代理网关

Kiro-Go 是一个高性能 Go 代理网关，将 **Anthropic API** 和 **OpenAI API** 请求转换为 Kiro（AWS CodeWhisperer）内部协议，支持本地单用户和线上多用户两种部署模式。

## 架构总览

```
┌─────────────────────────────────────────────────────────────────┐
│                        客户端 (Clients)                          │
├──────────────────┬──────────────────┬───────────────────────────┤
│  Claude Code     │  Cursor / IDE    │  kiro-launcher (本地)      │
│  Anthropic 格式  │  OpenAI 格式     │  Anthropic 格式            │
│  /v1/messages    │  /v1/chat/compl  │  /v1/messages              │
└────────┬─────────┴────────┬─────────┴─────────────┬─────────────┘
         │                  │                       │
         ▼                  ▼                       ▼
┌─────────────────────────────────────────────────────────────────┐
│                      kiro-go 代理网关                            │
│                                                                  │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │                   认证中间件 (AuthMiddleware)              │   │
│  │  1. X-Kiro-Credentials header → 直接使用凭证              │   │
│  │  2. act-xxx 激活码 → app.js验证 → 查凭证/主池回退         │   │
│  │  3. creds-xxx base64 → 解码凭证                           │   │
│  │  4. 普通 API Key → 使用主凭证池                            │   │
│  └──────────────────────────────────────────────────────────┘   │
│                                                                  │
│  ┌─────────────────┐    ┌──────────────────┐                    │
│  │ Anthropic 处理器 │    │  OpenAI 处理器    │                    │
│  │ /v1/messages     │    │ /v1/chat/compl   │                    │
│  │ /v1/models       │    │                  │                    │
│  └────────┬─────────┘    └────────┬─────────┘                    │
│           │                       │                              │
│           ▼                       ▼                              │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │              Anthropic → Kiro 转换器 (converter.go)       │   │
│  │  • 消息规范化流水线 (角色/交替/合并)                        │   │
│  │  • 工具转换 (inputSchema → {"json": schema})              │   │
│  │  • 图片转换 (base64 → Kiro 格式)                          │   │
│  │  • 系统提示词注入                                          │   │
│  │  • Thinking 模式支持                                       │   │
│  └──────────────────────────────────────────────────────────┘   │
│                          │                                       │
│                          ▼                                       │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │                 Kiro Provider (provider.go)                │   │
│  │  • Token 管理 + 自动刷新                                   │   │
│  │  • 多凭证轮转 + 故障转移                                   │   │
│  │  • 402 额度用尽自动换号                                    │   │
│  │  • 403 Token 过期强制刷新                                  │   │
│  └──────────────────────────────────────────────────────────┘   │
│                          │                                       │
└──────────────────────────┼───────────────────────────────────────┘
                           ▼
              ┌─────────────────────────┐
              │   Kiro API (AWS)        │
              │   generateAssistantResp │
              └─────────────────────────┘
```

## 两种部署模式

### 模式 A：本地部署（单用户）

```
Claude Code / IDE  →  kiro-go (localhost:13000)  →  Kiro API
                      使用本地 credentials.json
```

- 用户在本地运行 `kiro-go`，直接使用自己的凭证
- API Key 仅用于本地安全（防止其他程序调用）
- 适合个人开发者

### 模式 B：线上部署（多用户）

```
Cursor (远程)  ──FRP穿透──→  kiro-go (服务器:13000)  →  Kiro API
Claude Code    ──直连──────→  使用激活码查找用户凭证
kiro-launcher  ──FRP穿透──→
```

- 服务器运行 `kiro-go`，支持多用户
- 每个用户有独立激活码（`act-xxx`），对应独立凭证
- `credentials.json` 作为主凭证池（后备）
- `user_credentials.json` 存储用户激活码→凭证映射
- 支持 402 额度用尽自动换号

## API 端点

### 代理 API

| 端点 | 方法 | 格式 | 说明 |
|------|------|------|------|
| `/v1/models` | GET | Anthropic | 获取模型列表 |
| `/v1/messages` | POST | Anthropic | 发送消息（流式/非流式） |
| `/v1/chat/completions` | POST | OpenAI | 发送消息（流式/非流式） |

### 管理 API

| 端点 | 方法 | 说明 |
|------|------|------|
| `/api/admin/user-credentials` | GET | 列出所有用户凭证 |
| `/api/admin/user-credentials` | POST | 添加/更新用户凭证 |
| `/api/admin/user-credentials/:code` | GET | 查询指定激活码 |
| `/api/admin/user-credentials/:code` | DELETE | 删除指定激活码 |
| `/api/admin/user-credentials/stats` | GET | 用户统计 |
| `/api/admin/reload-credentials` | POST | 热加载主凭证 |

### 认证方式

| 方式 | 格式 | 场景 |
|------|------|------|
| API Key | `x-api-key: kiro-server-2026` | 使用主凭证池 |
| 激活码 | `x-api-key: act-xxx` | 多用户模式 |
| Base64 凭证 | `x-api-key: creds-{base64}` | 客户端直传凭证 |
| Header 凭证 | `X-Kiro-Credentials: {json}` | kiro-launcher 本地模式 |
| Bearer Token | `Authorization: Bearer xxx` | OpenAI 兼容 |

## 支持的模型

| 模型 ID | 内部映射 | Thinking |
|---------|---------|----------|
| `claude-sonnet-4-5-20250929` | claude-sonnet-4.5 | ❌ |
| `claude-sonnet-4-5-20250929-thinking` | claude-sonnet-4.5 | ✅ |
| `claude-opus-4-5-20251101` | claude-opus-4.5 | ❌ |
| `claude-opus-4-5-20251101-thinking` | claude-opus-4.5 | ✅ |
| `claude-opus-4-6` | claude-opus-4.6 | ❌ |
| `claude-opus-4-6-thinking` | claude-opus-4.6 | ✅ |
| `claude-haiku-4-5-20251001` | claude-haiku-4.5 | ❌ |
| `claude-haiku-4-5-20251001-thinking` | claude-haiku-4.5 | ✅ |

模型名支持模糊匹配：`claude-sonnet-4-20250514`、`claude-3-5-sonnet` 等均自动映射。

## 配置文件

### config.json

```json
{
  "host": "0.0.0.0",
  "port": 13000,
  "apiKey": "your-api-key",
  "regions": ["us-east-1"],
  "kiroVersion": "1.6.0",
  "systemVersion": "linux",
  "nodeVersion": "v22.12.0",
  "userCredentialsPath": "/opt/kiro-proxy/user_credentials.json",
  "activationServerUrl": "http://127.0.0.1:7777"
}
```

### credentials.json（主凭证池）

```json
{
  "accessToken": "...",
  "refreshToken": "...",
  "expiresAt": "2026-02-17T18:58:56+08:00",
  "authMethod": "idc",
  "clientId": "...",
  "clientSecret": "...",
  "region": "us-east-1"
}
```

支持数组格式（多凭证轮转）：

```json
[
  { "accessToken": "...", "refreshToken": "...", ... },
  { "accessToken": "...", "refreshToken": "...", ... }
]
```

### user_credentials.json（用户激活码映射）

```json
{
  "act-user001": {
    "activation_code": "act-user001",
    "user_name": "用户A",
    "credentials": { "accessToken": "...", "refreshToken": "...", ... },
    "created_at": "2026-02-17T13:00:00Z",
    "updated_at": "2026-02-17T13:00:00Z"
  }
}
```

## 目录结构

```
kiro-go/
├── main.go                          # 入口：路由、中间件、服务启动
├── config.local.json                # 本地开发配置
├── go.mod / go.sum
├── internal/
│   ├── anthropic/
│   │   ├── handlers.go              # Anthropic API 处理器
│   │   ├── converter.go             # Anthropic → Kiro 请求转换
│   │   ├── model_resolver.go        # 模型名标准化 + 映射
│   │   ├── stream_context.go        # 流式响应处理 (thinking 提取)
│   │   └── types.go                 # Anthropic 类型定义
│   ├── openai/
│   │   └── handlers.go              # OpenAI → Anthropic → Kiro 转换
│   ├── kiro/
│   │   ├── provider.go              # Kiro API 调用 + 重试 + 故障转移
│   │   ├── token_manager.go         # Token 管理 + IdC 刷新
│   │   ├── user_credentials.go      # 用户凭证管理器
│   │   ├── event.go                 # Kiro 事件解析
│   │   └── machine_id.go            # 机器 ID 生成
│   ├── common/
│   │   └── auth.go                  # 认证中间件
│   ├── model/
│   │   ├── config.go                # 配置结构
│   │   └── credentials.go           # 凭证结构
│   └── parser/
│       └── decoder.go               # AWS Event Stream 解码器
└── README.md
```

## 请求转换流程

### Anthropic → Kiro

```
Anthropic /v1/messages
  ↓
1. 解析请求 (model, messages, tools, system, thinking)
2. 模型映射 (claude-sonnet-4-20250514 → claude-sonnet-4.5)
3. 消息规范化流水线:
   a. 角色规范化 (system/developer/tool → user)
   b. 合并相邻同角色消息
   c. 确保第一条是 user
   d. 确保 user/assistant 交替
4. 构建 Kiro 请求:
   - conversationState.chatTriggerType = "MANUAL"
   - conversationState.conversationId = UUID
   - currentMessage.userInputMessage.content = 文本
   - currentMessage.userInputMessage.modelId = 内部模型ID
   - userInputMessageContext.tools = [{toolSpecification: {name, description, inputSchema: {json: ...}}}]
   - userInputMessageContext.toolResults = [...]
   - history = [...] (仅非空时包含)
5. 调用 Kiro API (generateAssistantResponse)
6. 解析 AWS Event Stream 响应
7. 转换回 Anthropic 格式返回
```

### OpenAI → Kiro

```
OpenAI /v1/chat/completions
  ↓
1. 解析 OpenAI 请求
2. 转换为 Anthropic MessagesRequest:
   - system/developer → system prompt
   - tool messages → user with tool_result content blocks
   - tools → Anthropic tool 格式
   - max_tokens / max_completion_tokens
   - 模型名含 "thinking" → 自动启用 thinking
3. 复用 Anthropic → Kiro 转换流程
4. 转换响应:
   - content → choices[0].message.content
   - tool_use → choices[0].message.tool_calls
   - thinking → reasoning_content
   - 流式: SSE data: {...} 格式
```

---

## 部署指南

### 本地开发

```bash
cd kiro-go
go build -o kiro-go .
./kiro-go -config config.local.json -credentials ~/Library/Application\ Support/kiro-launcher/credentials.json
```

### 服务器部署

#### 1. 交叉编译

```bash
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o kiro-go-linux .
```

#### 2. 上传

```bash
scp kiro-go-linux root@YOUR_SERVER:/opt/kiro-proxy/kiro-go-latest
ssh root@YOUR_SERVER "chmod +x /opt/kiro-proxy/kiro-go-latest"
```

#### 3. 配置

```bash
# config.json
cat > /opt/kiro-proxy/config.json << 'EOF'
{
  "host": "0.0.0.0",
  "port": 13000,
  "apiKey": "your-strong-api-key",
  "regions": ["us-east-1", "us-west-2"],
  "kiroVersion": "1.6.0",
  "systemVersion": "linux",
  "nodeVersion": "v22.12.0",
  "userCredentialsPath": "/opt/kiro-proxy/user_credentials.json"
}
EOF

# 创建空的用户凭证文件
echo '{}' > /opt/kiro-proxy/user_credentials.json
```

#### 4. Systemd 服务

```bash
cat > /etc/systemd/system/kiro-proxy.service << 'EOF'
[Unit]
Description=Kiro Proxy Service (Go)
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=/opt/kiro-proxy
ExecStart=/opt/kiro-proxy/kiro-go-latest -config /opt/kiro-proxy/config.json -credentials /opt/kiro-proxy/credentials.json
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable kiro-proxy
systemctl start kiro-proxy
```

#### 5. 管理命令

```bash
systemctl status kiro-proxy        # 状态
systemctl restart kiro-proxy       # 重启
journalctl -u kiro-proxy -f        # 实时日志
journalctl -u kiro-proxy --since "1 hour ago"  # 最近日志
```

#### 6. 防火墙

```bash
# 放行 13000 端口（API）
firewall-cmd --permanent --add-port=13000/tcp
firewall-cmd --reload
```

### 添加用户（激活码模式）

```bash
# 添加用户凭证
curl -X POST http://YOUR_SERVER:13000/api/admin/user-credentials \
  -H "Content-Type: application/json" \
  -d '{
    "activation_code": "act-user001",
    "user_name": "用户A",
    "credentials": {
      "accessToken": "...",
      "refreshToken": "...",
      "expiresAt": "...",
      "authMethod": "idc",
      "clientId": "...",
      "clientSecret": "...",
      "region": "us-east-1"
    }
  }'

# 用户使用激活码调用 API
curl -X POST http://YOUR_SERVER:13000/v1/messages \
  -H "x-api-key: act-user001" \
  -H "Content-Type: application/json" \
  -d '{"model":"claude-sonnet-4-5-20250929","max_tokens":1000,"messages":[{"role":"user","content":"hello"}]}'
```

### 一键更新脚本

```bash
#!/bin/bash
# deploy.sh - 从本地编译并部署到服务器
set -e
SERVER="root@117.72.183.248"

echo "编译 Linux 二进制..."
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o /tmp/kiro-go-linux .

echo "上传到服务器..."
scp /tmp/kiro-go-linux $SERVER:/opt/kiro-proxy/kiro-go-new
ssh $SERVER "chmod +x /opt/kiro-proxy/kiro-go-new && mv /opt/kiro-proxy/kiro-go-new /opt/kiro-proxy/kiro-go-latest && systemctl restart kiro-proxy && sleep 2 && systemctl status kiro-proxy --no-pager"

echo "部署完成！"
```

---

## 与 app.js 卡密系统集成

kiro-go 支持与 `app.js`（端口 7777）的卡密管理系统集成。配置 `activationServerUrl` 后，收到 `act-XXXX-XXXX-XXXX-XXXX` 格式的激活码时，会先调 app.js 验证。

### 激活码认证流程

```
用户请求 (x-api-key: act-XXXX-XXXX-XXXX-XXXX)
  │
  ▼
kiro-go AuthMiddleware
  │
  ├─ 1. 提取激活码 (去掉 act- 前缀，转大写)
  │
  ├─ 2. 调 app.js /api/tunnel/check 验证
  │     ├─ 激活码无效 → 403 拒绝
  │     ├─ 设备不匹配 → 403 拒绝
  │     ├─ 穿透过期 → 403 拒绝
  │     ├─ 验证服务不可用 → 降级放行
  │     └─ 验证通过 → 继续
  │
  ├─ 3. 查 user_credentials.json 获取 Kiro 凭证
  │     ├─ 有独立凭证 → 使用该凭证调 Kiro API
  │     └─ 无独立凭证 → 回退到主凭证池 (credentials.json)
  │
  ▼
调用 Kiro API → 返回结果
```

### 配置字段说明

| 字段 | 说明 | 示例 |
|------|------|------|
| `activationServerUrl` | app.js 卡密验证服务地址 | `http://127.0.0.1:7777` |
| `userCredentialsPath` | 用户激活码→凭证映射文件 | `/opt/kiro-proxy/user_credentials.json` |

- **`activationServerUrl` 为空**：跳过 app.js 验证，直接查 `user_credentials.json`
- **`activationServerUrl` 已配置**：先验证再查凭证，验证失败直接拒绝
- **app.js 服务不可用**：降级放行（保证可用性）

### app.js 卡密数据 (codes.json)

```json
[
  {
    "code": "4ZJ6-NK46-QJBG-DTZD",
    "active": true,
    "machineId": "c0a08047...",
    "activatedAt": "2026-02-11T04:23:04.340Z",
    "tunnelDays": 30
  }
]
```

### 客户端使用方式

```bash
# Cursor / Claude Code 配置 API Key 为:
act-4ZJ6-NK46-QJBG-DTZD

# 请求时自动携带机器码 header (kiro-launcher 自动处理):
X-Machine-Id: c0a08047cb478115587f035b8c1245cc...
```

---

## 当前服务器状态

| 项目 | 值 |
|------|------|
| 服务器 | `117.72.183.248:13000` |
| API Key | `kiro-server-2026` |
| 服务 | `kiro-proxy.service` (systemd, auto-restart) |
| 二进制 | `/opt/kiro-proxy/kiro-go-latest` |
| 配置 | `/opt/kiro-proxy/config.json` |
| 主凭证 | `/opt/kiro-proxy/credentials.json` |
| 用户凭证 | `/opt/kiro-proxy/user_credentials.json` |
| 激活码验证 | `http://127.0.0.1:7777` (app.js) |
| 支持格式 | Anthropic (`/v1/messages`) + OpenAI (`/v1/chat/completions`) |
| 模型数 | 8 (含 thinking 变体) |
