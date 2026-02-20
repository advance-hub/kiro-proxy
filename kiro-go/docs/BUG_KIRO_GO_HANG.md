# kiro-go 进程卡死 Bug 分析

## 现象

- kiro-launcher 前端"可用模型"一直显示"正在加载模型列表..."
- Claude Code 请求 `/v1/models` 无响应
- `curl http://127.0.0.1:13000/v1/models` 连接成功但 **0 bytes received**，超时
- **所有 HTTP 端点都卡死**，包括 `/api/admin/user-credentials` 等不经过 auth 中间件的端点
- kiro-go 进程仍在运行（ps 可见），TCP 端口仍在监听

## 根因

**Go `log` 包的 `sync.Mutex` + stdout pipe buffer 满 = 全局死锁**

### 调用链

```
kiro-go 进程
  └─ stdout/stderr → pipe → kiro-launcher 的 spawnReader (goroutine)
```

### 触发条件

1. `/v1/messages` 请求处理时，打印大量日志（`[KIRO_PRE]`、`[KIRO_POST]`、`[RAW_KIRO_REQUEST]` 等，每条包含完整消息内容，可达数十 KB）
2. macOS pipe buffer 默认 **64 KB**，日志量大时 buffer 被填满
3. 当 pipe buffer 满时，`write()` 系统调用**阻塞**
4. Go 的 `log` 包内部有一个 `sync.Mutex`，阻塞在 `write()` 的 goroutine **持有这个锁**
5. 所有后续调用 `log.Printf` 的 goroutine 都会**等待这个锁**
6. auth 中间件 (`auth.go:194`) 每个请求都调用 `log.Printf`
7. 因此**所有 HTTP 请求**（包括纯静态的 `/v1/models`）都被阻塞

### 示意图

```
goroutine A: /v1/messages handler
  → log.Printf("[KIRO_PRE] ...大量内容...")
  → log.mutex.Lock()  ✅ 获取锁
  → write(stderr_fd, data)  ❌ pipe buffer 满，阻塞
  → 锁一直不释放

goroutine B: /v1/models handler
  → authMw.Wrap → log.Printf("[AUTH] ...")
  → log.mutex.Lock()  ❌ 等待锁，永远阻塞

goroutine C: curl /api/admin/user-credentials
  → corsMiddleware → handler → log.Printf(...)
  → log.mutex.Lock()  ❌ 等待锁，永远阻塞
```

## 修复

在 `kiro-go/main.go` 中添加 `asyncWriter`，替代直接写 `os.Stderr`：

```go
// main.go
func main() {
    log.SetOutput(newAsyncWriter(os.Stderr, 1024))
    // ...
}
```

### asyncWriter 实现

- 内部使用 buffered channel（容量 1024 条）
- `Write()` 方法非阻塞：channel 满时丢弃日志，不阻塞调用方
- 后台 goroutine 异步 drain channel 内容到 stderr
- 即使 pipe buffer 满，`log.Printf` 也不会阻塞任何 goroutine

## 其他相关修复

| 文件 | 修改 | 目的 |
|------|------|------|
| `token_manager.go` | `AcquireContext` 移除 token 刷新 | 不在请求路径阻塞 |
| `provider.go` | `CallWithCredentials` 移除 token 刷新 | 不在请求路径阻塞 |
| `proxy.go` (launcher) | `OneClickStart` 同步刷新 token (8s 超时) | 启动前确保 token 新鲜 |
| `ProxyPanel.tsx` | `fetchModels` 加 3s AbortController | 前端不会无限等待 |

## 复现条件

1. kiro-go 通过 kiro-launcher 以子进程方式启动（stdout/stderr 走 pipe）
2. 发送一个 `/v1/messages` 请求（触发大量日志输出）
3. pipe buffer 被填满（日志量 > 64KB）
4. 此后所有 HTTP 请求都会卡死
