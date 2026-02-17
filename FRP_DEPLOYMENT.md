# FRP 服务端部署指南

> 客户端（FRP Client）已内置在 Kiro Launcher 中，无需单独安装。  
> 本文档只讲 **服务端（frps）** 的部署。

---

## 目录

- [准备工作](#准备工作)
- [Linux 部署](#linux-部署)
  - [下载](#linux-下载)
  - [配置](#linux-配置)
  - [注册 systemd 服务（推荐）](#注册-systemd-服务推荐)
  - [手动启动（临时测试）](#手动启动临时测试)
  - [防火墙 / 安全组](#linux-防火墙--安全组)
  - [卸载](#linux-卸载)
- [Windows 部署](#windows-部署)
  - [下载](#windows-下载)
  - [配置](#windows-配置)
  - [手动启动](#windows-手动启动)
  - [注册为 Windows 服务（推荐）](#注册为-windows-服务推荐)
  - [防火墙](#windows-防火墙)
  - [卸载](#windows-卸载)
- [填入 Kiro Launcher](#填入-kiro-launcher)
- [域名配置（可选）](#域名配置可选)
- [常见问题](#常见问题)

---

## 准备工作

1. 一台有公网 IP 的服务器（云服务器、VPS 均可）
2. 确认服务器架构：
   - Linux: 运行 `uname -m`，`x86_64` 对应 amd64，`aarch64` 对应 arm64
   - Windows: 一般为 amd64（64 位系统）
3. 到 [FRP Releases](https://github.com/fatedier/frp/releases) 页面确认最新版本号（本文以 `v0.67.0` 为例）

---

## Linux 部署

### Linux 下载

根据架构选择对应包：

```bash
# amd64（大多数云服务器）
cd /opt
wget https://github.com/fatedier/frp/releases/download/v0.67.0/frp_0.67.0_linux_amd64.tar.gz
tar -xzf frp_0.67.0_linux_amd64.tar.gz
mv frp_0.67.0_linux_amd64 frp
rm frp_0.67.0_linux_amd64.tar.gz

# arm64（ARM 服务器，如华为鲲鹏、树莓派等）
cd /opt
wget https://github.com/fatedier/frp/releases/download/v0.67.0/frp_0.67.0_linux_arm64.tar.gz
tar -xzf frp_0.67.0_linux_arm64.tar.gz
mv frp_0.67.0_linux_arm64 frp
rm frp_0.67.0_linux_arm64.tar.gz
```

### Linux 配置

```bash
cat > /opt/frp/frps.toml << 'EOF'
# FRP 服务端配置
bindPort = 7000
auth.token = "改成你自己的密码"

# HTTP 虚拟主机端口（HTTP 模式穿透用）
vhostHTTPPort = 8080
EOF
```

> 把 `auth.token` 改成一个强密码，客户端连接时需要用到。

### 注册 systemd 服务（推荐）

注册为系统服务后可以开机自启、崩溃自动重启：

```bash
cat > /etc/systemd/system/frps.service << 'EOF'
[Unit]
Description=FRP Server
After=network.target

[Service]
Type=simple
ExecStart=/opt/frp/frps -c /opt/frp/frps.toml
Restart=on-failure
RestartSec=5s
LimitNOFILE=1048576

[Install]
WantedBy=multi-user.target
EOF

# 启用并启动
systemctl daemon-reload
systemctl enable frps    # 开机自启
systemctl start frps     # 立即启动
```

常用管理命令：

```bash
systemctl status frps      # 查看状态
systemctl restart frps     # 重启
systemctl stop frps        # 停止
journalctl -u frps -f      # 实时查看日志
journalctl -u frps --since "1 hour ago"  # 查看最近 1 小时日志
```

### 手动启动（临时测试）

不想注册服务的话，可以直接前台或后台运行：

```bash
# 前台运行（Ctrl+C 停止）
/opt/frp/frps -c /opt/frp/frps.toml

# 后台运行
nohup /opt/frp/frps -c /opt/frp/frps.toml > /opt/frp/frps.log 2>&1 &
```

### Linux 防火墙 / 安全组

需要放行以下端口：

| 端口 | 用途 | 必须 |
|------|------|------|
| 7000 | FRP 通信端口 | ✅ |
| 8080 | HTTP 穿透端口 | HTTP 模式需要 |
| 自定义端口 | TCP 穿透远程端口 | TCP 模式需要 |

**云服务器安全组**（阿里云/腾讯云/华为云等）：在控制台的安全组规则中添加入站规则。

**系统防火墙**（如果开启了 firewalld 或 iptables）：

```bash
# firewalld
firewall-cmd --permanent --add-port=7000/tcp
firewall-cmd --permanent --add-port=8080/tcp
firewall-cmd --reload

# iptables
iptables -I INPUT -p tcp --dport 7000 -j ACCEPT
iptables -I INPUT -p tcp --dport 8080 -j ACCEPT
iptables-save > /etc/iptables.rules
```

### Linux 卸载

```bash
systemctl stop frps
systemctl disable frps
rm -f /etc/systemd/system/frps.service
systemctl daemon-reload
rm -rf /opt/frp
```

---

## Windows 部署

### Windows 下载

1. 到 [FRP Releases](https://github.com/fatedier/frp/releases) 下载 `frp_0.67.0_windows_amd64.zip`
2. 解压到一个固定目录，比如 `C:\frp`

> 解压后目录结构应该是：`C:\frp\frps.exe`、`C:\frp\frpc.exe` 等

### Windows 配置

在 `C:\frp` 目录下创建 `frps.toml` 文件，内容：

```toml
# FRP 服务端配置
bindPort = 7000
auth.token = "改成你自己的密码"

# HTTP 虚拟主机端口（HTTP 模式穿透用）
vhostHTTPPort = 8080
```

### Windows 手动启动

打开 CMD 或 PowerShell：

```powershell
cd C:\frp
.\frps.exe -c .\frps.toml
```

> 窗口关闭后服务会停止。适合临时测试。

### 注册为 Windows 服务（推荐）

使用 [WinSW](https://github.com/winsw/winsw/releases) 或 [NSSM](https://nssm.cc/download) 将 frps 注册为 Windows 服务，实现开机自启。

**方法一：使用 WinSW（推荐）**

1. 下载 [WinSW-x64.exe](https://github.com/winsw/winsw/releases)，重命名为 `frps-service.exe`，放到 `C:\frp\` 目录
2. 在 `C:\frp\` 创建 `frps-service.xml`：

```xml
<service>
  <id>frps</id>
  <name>FRP Server</name>
  <description>FRP Server Service</description>
  <executable>C:\frp\frps.exe</executable>
  <arguments>-c C:\frp\frps.toml</arguments>
  <log mode="roll-by-size">
    <sizeThreshold>10240</sizeThreshold>
    <keepFiles>3</keepFiles>
  </log>
  <startmode>Automatic</startmode>
</service>
```

3. 以管理员身份打开 CMD，执行：

```cmd
cd C:\frp
frps-service.exe install
frps-service.exe start
```

管理命令：

```cmd
frps-service.exe status    :: 查看状态
frps-service.exe restart   :: 重启
frps-service.exe stop      :: 停止
```

**方法二：使用 NSSM**

1. 下载 [NSSM](https://nssm.cc/download)，解压后把 `nssm.exe` 放到 `C:\frp\`
2. 以管理员身份打开 CMD：

```cmd
cd C:\frp
nssm install frps C:\frp\frps.exe -c C:\frp\frps.toml
nssm start frps
```

管理命令：

```cmd
nssm status frps     :: 查看状态
nssm restart frps    :: 重启
nssm stop frps       :: 停止
nssm edit frps       :: 编辑配置（GUI）
```

### Windows 防火墙

以管理员身份打开 PowerShell：

```powershell
# 放行 FRP 通信端口
New-NetFirewallRule -DisplayName "FRP Server" -Direction Inbound -Protocol TCP -LocalPort 7000 -Action Allow

# 放行 HTTP 穿透端口
New-NetFirewallRule -DisplayName "FRP HTTP Vhost" -Direction Inbound -Protocol TCP -LocalPort 8080 -Action Allow
```

> 如果是云服务器上的 Windows，同样需要在云控制台安全组中放行对应端口。

### Windows 卸载

```cmd
:: WinSW 方式
frps-service.exe stop
frps-service.exe uninstall

:: NSSM 方式
nssm stop frps
nssm remove frps confirm

:: 最后删除目录
rmdir /s /q C:\frp
```

---

## 填入 Kiro Launcher

在 Kiro Launcher 的穿透页面填写：

| 字段 | 值 | 说明 |
|------|------|------|
| 服务器地址 | 你的服务器公网 IP | 如 `1.2.3.4` |
| 服务器端口 | `7000` | 对应 frps.toml 的 bindPort |
| 认证 Token | 你设的密码 | 对应 frps.toml 的 auth.token |
| 代理类型 | `http` 或 `tcp` | 按需选择 |
| HTTP 端口 | `8080` | HTTP 模式，对应 vhostHTTPPort |
| 自定义域名 | 你的域名 | HTTP 模式需要 |
| 远程端口 | 如 `6001` | TCP 模式需要 |

---

## 域名配置

### 没有域名（推荐用 TCP 模式）

没有域名的话，直接用 TCP 模式最简单：

1. 在 Kiro Launcher 穿透页面，代理类型选 `tcp`
2. 填写远程端口（如 `6001`），确保服务器安全组已放行该端口
3. 访问地址：`http://服务器公网IP:6001`

> TCP 模式不需要域名，通过 IP + 端口直接访问，适合个人使用或测试。

### 有域名（可用 HTTP 模式）

如果你有域名，可以配合 HTTP 模式使用：

1. 在域名 DNS 添加 A 记录：`kiro.yourdomain.com → 服务器公网 IP`
2. 在 Kiro Launcher 穿透页面，代理类型选 `http`，填入自定义域名 `kiro.yourdomain.com`
3. 访问地址：`http://kiro.yourdomain.com:8080`

如果想去掉端口号（直接用 80 端口）：
- 把 frps.toml 中 `vhostHTTPPort` 改为 `80`
- Kiro Launcher 中 HTTP 端口也改为 `80`
- 访问地址变为：`http://kiro.yourdomain.com`

---

## 常见问题

**Q: 连接不上怎么办？**
1. 确认 frps 正在运行：`systemctl status frps`（Linux）或查看服务状态（Windows）
2. 确认端口已放行（安全组 + 系统防火墙都要检查）
3. 确认 Token 一致（服务端和客户端必须相同）
4. 用 `telnet 服务器IP 7000` 测试端口是否通

**Q: HTTP 模式访问返回 404？**
- 确认域名已正确解析到服务器 IP（`ping kiro.yourdomain.com`）
- 确认 Kiro Launcher 中填的自定义域名和 DNS 记录一致
- 确认 vhostHTTPPort 端口已放行

**Q: TCP 模式怎么用？**
- 不需要域名，直接通过 `服务器IP:远程端口` 访问
- 需要在安全组中额外放行你配置的远程端口

**Q: 能用 HTTPS 吗？**
- FRP 本身支持 HTTPS，但需要额外配置证书
- 更简单的方案：在服务器上用 Nginx 反向代理 + Let's Encrypt 证书，Nginx 监听 443 转发到 frps 的 vhostHTTPPort
