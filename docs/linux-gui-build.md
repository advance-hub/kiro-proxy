# Linux GUI 构建指南

> 由于 macOS 无法直接交叉编译 Linux GUI（Wails 依赖 WebKit2GTK + CGO），需要借助 Linux 服务器完成编译。
> 本文档记录了完整的环境搭建和构建流程。

## 架构概览

```
本地 macOS                          远程 Linux 服务器
┌──────────────┐                    ┌──────────────────────┐
│ 1. 编译前端   │  ──── scp ────>   │ 3. wails build -s    │
│    pnpm build │                   │    (跳过前端,仅编译Go)│
│              │                    │                      │
│ 2. 打包源码   │  ──── scp ────>   │ 4. 输出 kiro-launcher │
│    tar czf    │                   │    Linux GUI 二进制    │
│              │  <─── scp ─────   │                      │
│ 5. 取回产物   │                   └──────────────────────┘
└──────────────┘
```

## 前置条件

### 本地 (macOS)
- Go 1.21+
- Node.js 18+
- pnpm
- SSH 到服务器的免密登录

### 服务器 (Linux)
- **OS**: Ubuntu 22.04+ / Debian 12+
- **IP**: `117.72.183.248` (默认，可通过 `DEPLOY_SERVER` 环境变量覆盖)

---

## 一、服务器环境安装（仅首次）

### 1.1 安装系统依赖

```bash
ssh root@117.72.183.248

# GTK3 + WebKit2GTK + 编译工具链
apt-get update
apt-get install -y libgtk-3-dev libwebkit2gtk-4.0-dev build-essential pkg-config
```

### 1.2 安装 Go

```bash
# 使用阿里云镜像（国内服务器访问 go.dev 可能超时）
curl -sL https://mirrors.aliyun.com/golang/go1.23.6.linux-amd64.tar.gz -o /tmp/go.tar.gz
rm -rf /usr/local/go
tar -C /usr/local -xzf /tmp/go.tar.gz
rm /tmp/go.tar.gz

# 添加到 PATH
echo 'export PATH=$PATH:/usr/local/go/bin:/root/go/bin' >> /root/.bashrc
source /root/.bashrc

# 设置 Go 代理（国内加速）
go env -w GOPROXY=https://goproxy.cn,direct

# 验证
go version  # 应输出 go1.23.6 linux/amd64
```

### 1.3 升级 Node.js（如果版本 < 18）

```bash
# 使用 NodeSource 安装 Node 20
curl -fsSL https://deb.nodesource.com/setup_20.x | bash -

# 如果遇到 libnode-dev 冲突，先卸载
apt-get remove -y libnode-dev nodejs-doc 2>/dev/null
dpkg --configure -a

apt-get install -y nodejs
node --version  # 应输出 v20.x
npm --version
```

### 1.4 安装 pnpm

```bash
npm install -g pnpm
pnpm --version

# 注意：如果使用宝塔面板的 Node，pnpm 可能安装到非标准路径
# 例如 /www/server/nodejs/v20.19.0/bin/pnpm
# 需要确保该路径在 PATH 中
```

### 1.5 安装 Wails CLI

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
wails version  # 应输出 v2.11.0
```

### 1.6 验证环境

```bash
# 一键检查所有依赖
echo "=== Go ===" && go version && \
echo "=== Node ===" && node --version && \
echo "=== pnpm ===" && pnpm --version && \
echo "=== Wails ===" && wails version && \
echo "=== GTK ===" && pkg-config --modversion gtk+-3.0 && \
echo "=== WebKit ===" && pkg-config --modversion webkit2gtk-4.0 && \
echo "ALL OK"
```

---

## 二、构建流程

### 方式一：一键脚本（推荐）

在本地执行：

```bash
# 构建 Linux GUI 并取回产物
bash build.sh linux-gui

# 或手动指定服务器
DEPLOY_SERVER=root@117.72.183.248 bash build.sh linux-gui
```

产物输出到：`release/linux-gui/kiro-launcher`

### 方式二：手动步骤

#### 2.1 本地编译前端

```bash
cd kiro-launcher/frontend
pnpm install
pnpm build
# 产物在 kiro-launcher/dist/
```

#### 2.2 打包源码 + 前端产物

```bash
cd /path/to/kiro-proxy
tar czf /tmp/kiro-proxy-src.tar.gz \
  --exclude='.git' \
  --exclude='node_modules' \
  --exclude='release' \
  --exclude='build' \
  --exclude='杂' \
  kiro-go kiro-launcher build.sh
```

#### 2.3 上传到服务器

```bash
scp /tmp/kiro-proxy-src.tar.gz root@117.72.183.248:/tmp/
```

#### 2.4 服务器上解压 + 编译

```bash
ssh root@117.72.183.248

export PATH=$PATH:/usr/local/go/bin:/root/go/bin
export GOPROXY=https://goproxy.cn,direct

# 解压源码
rm -rf /opt/kiro-proxy-src
mkdir -p /opt/kiro-proxy-src
tar xzf /tmp/kiro-proxy-src.tar.gz -C /opt/kiro-proxy-src

# 编译 kiro-go
cd /opt/kiro-proxy-src/kiro-go
go build -o /tmp/kiro-go-linux .

# 准备 sidecar
mkdir -p /opt/kiro-proxy-src/kiro-launcher/sidecar
cp /tmp/kiro-go-linux /opt/kiro-proxy-src/kiro-launcher/sidecar/kiro-go

# 确保前端 dist 在正确位置
mkdir -p /opt/kiro-proxy-src/kiro-launcher/frontend/dist
cp -r /opt/kiro-proxy-src/kiro-launcher/dist/* /opt/kiro-proxy-src/kiro-launcher/frontend/dist/

# 编译 Wails GUI（跳过前端，因为已经有 dist）
cd /opt/kiro-proxy-src/kiro-launcher
wails build -clean -o kiro-launcher -s
# -s: 跳过前端编译
# 产物在 build/bin/kiro-launcher
```

#### 2.5 取回产物

```bash
# 本地执行
mkdir -p release/linux-gui
scp root@117.72.183.248:/opt/kiro-proxy-src/kiro-launcher/build/bin/kiro-launcher release/linux-gui/
```

---

## 三、使用说明

### 在 Linux 桌面上运行

```bash
# 安装运行时依赖（仅首次）
sudo apt install libgtk-3-0 libwebkit2gtk-4.0-37

# 赋予执行权限
chmod +x kiro-launcher

# 运行
./kiro-launcher
```

---

## 四、常见问题

### Q: pnpm install 超时，提示 xiaohongshu artifactory
**A**: pnpm-lock.yaml 里锁定了内部 npm 源。删除 lockfile 重新生成：
```bash
rm pnpm-lock.yaml
pnpm install --registry https://registry.npmmirror.com
```

### Q: go.dev 下载超时
**A**: 使用阿里云镜像替代：
```bash
curl -sL https://mirrors.aliyun.com/golang/go1.23.6.linux-amd64.tar.gz -o /tmp/go.tar.gz
```

### Q: wails build 报 "pnpm not found"
**A**: 确保 pnpm 在 PATH 中。宝塔面板的 Node 路径可能不在默认 PATH：
```bash
export PATH=/www/server/nodejs/v20.19.0/bin:$PATH
```

### Q: 编译成功但运行时报 GTK 错误
**A**: 确保安装了运行时库（非 -dev 版本）：
```bash
sudo apt install libgtk-3-0 libwebkit2gtk-4.0-37
```

---

## 五、服务器信息

| 项目 | 值 |
|---|---|
| 服务器 | `root@117.72.183.248` |
| 源码目录 | `/opt/kiro-proxy-src` |
| Go 路径 | `/usr/local/go/bin/go` |
| Wails 路径 | `/root/go/bin/wails` |
| Node 路径 | `/www/server/nodejs/v20.19.0/bin/node`（宝塔） |
| pnpm 路径 | `/www/server/nodejs/v20.19.0/bin/pnpm`（宝塔） |
