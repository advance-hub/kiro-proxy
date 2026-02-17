import React, { useState, useEffect, useCallback } from "react";
import { Button, Card, Input, Typography, Toast, Space, Tag } from "@douyinfe/semi-ui";
import { IconRefresh, IconSave, IconPlay, IconStop, IconLink } from "@douyinfe/semi-icons";

const { Text } = Typography;

interface TunnelConfig {
  enabled: boolean;
  tunnelMode: string;
  serverAddr: string;
  serverPort: number;
  token: string;
  proxyName: string;
  customDomain: string;
  remotePort?: number;
  proxyType: string;
  vhostHTTPPort?: number;
  externalUrl?: string;
}

interface TunnelStatus {
  running: boolean;
  publicUrl: string;
  error?: string;
}

const wails = () => {
  if (!window.go?.main?.App) throw new Error("Wails runtime 尚未就绪");
  return window.go.main.App;
};

export default function TunnelPanel() {
  const [config, setConfig] = useState<TunnelConfig>({
    enabled: false,
    tunnelMode: "frp",
    serverAddr: "",
    serverPort: 7000,
    token: "",
    proxyName: "kiro-proxy",
    customDomain: "",
    proxyType: "http",
    vhostHTTPPort: 8080,
    externalUrl: "",
  });
  const [externalInput, setExternalInput] = useState("");
  const [status, setStatus] = useState<TunnelStatus>({ running: false, publicUrl: "" });
  const [loading, setLoading] = useState(false);
  const [dirty, setDirty] = useState(false);

  const loadConfig = useCallback(async () => {
    try {
      const cfg = await wails().LoadTunnelConfig() as TunnelConfig;
      setConfig(cfg);
      setExternalInput(cfg.externalUrl || "");
      setDirty(false);
    } catch (e) {
      Toast.error({ content: String(e) });
    }
  }, []);

  const loadStatus = useCallback(async () => {
    try {
      const s = await wails().GetTunnelStatus() as TunnelStatus;
      setStatus(s);
    } catch (e) {
      // ignore
    }
  }, []);

  useEffect(() => {
    loadConfig();
    loadStatus();
    const timer = setInterval(loadStatus, 3000);
    return () => clearInterval(timer);
  }, [loadConfig, loadStatus]);

  const update = (key: keyof TunnelConfig, value: any) => {
    setConfig(prev => ({ ...prev, [key]: value }));
    setDirty(true);
  };

  const handleModeSwitch = async (mode: string) => {
    const newConfig = { ...config, tunnelMode: mode };
    setConfig(newConfig);
    try {
      await wails().SaveTunnelConfig(newConfig);
      setDirty(false);
    } catch (e) {
      Toast.error({ content: String(e) });
    }
  };

  const handleSave = async () => {
    setLoading(true);
    try {
      const msg = await wails().SaveTunnelConfig(config);
      Toast.success({ content: msg });
      setDirty(false);
    } catch (e) {
      Toast.error({ content: String(e) });
    } finally {
      setLoading(false);
    }
  };

  const handleStart = async () => {
    setLoading(true);
    try {
      const msg = await wails().StartTunnel();
      Toast.success({ content: msg });
      loadStatus();
    } catch (e) {
      Toast.error({ content: String(e) });
    } finally {
      setLoading(false);
    }
  };

  const handleStop = async () => {
    setLoading(true);
    try {
      const msg = await wails().StopTunnel();
      Toast.success({ content: msg });
      loadStatus();
    } catch (e) {
      Toast.error({ content: String(e) });
    } finally {
      setLoading(false);
    }
  };

  const handleSetExternal = async () => {
    if (!externalInput.trim()) {
      Toast.warning({ content: "请输入穿透地址" });
      return;
    }
    setLoading(true);
    try {
      const msg = await wails().SetExternalTunnel(externalInput.trim());
      Toast.success({ content: msg });
      loadStatus();
      loadConfig();
    } catch (e) {
      Toast.error({ content: String(e) });
    } finally {
      setLoading(false);
    }
  };

  const handleClearExternal = async () => {
    setLoading(true);
    try {
      const msg = await wails().ClearExternalTunnel();
      Toast.success({ content: msg });
      setExternalInput("");
      loadStatus();
      loadConfig();
    } catch (e) {
      Toast.error({ content: String(e) });
    } finally {
      setLoading(false);
    }
  };

  const isExternal = config.tunnelMode === "external";

  const labelStyle: React.CSSProperties = { fontSize: 13, color: "var(--semi-color-text-1)", fontWeight: 500 };
  const descStyle: React.CSSProperties = { fontSize: 11, color: "var(--semi-color-text-2)", marginTop: 2 };
  const rowStyle: React.CSSProperties = { display: "flex", alignItems: "center", justifyContent: "space-between", padding: "10px 0", borderBottom: "1px solid var(--semi-color-border)" };

  return (
    <div style={{ padding: "20px 24px 32px" }}>
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: 20 }}>
        <div>
          <Text strong style={{ fontSize: 18, display: "block" }}>内网穿透</Text>
          <Text type="tertiary" size="small">将本地代理暴露到公网，供 Cursor 等工具使用</Text>
        </div>
        <Space>
          <Button size="small" theme="borderless" icon={<IconRefresh />} onClick={() => { loadConfig(); loadStatus(); }}>刷新</Button>
          {!isExternal && <Button size="small" theme="solid" type="primary" icon={<IconSave />} loading={loading} disabled={!dirty} onClick={handleSave}>保存</Button>}
        </Space>
      </div>

      {/* 模式切换 */}
      <Card bodyStyle={{ padding: "12px 16px" }} style={{ borderRadius: 10, marginBottom: 16 }}>
        <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
          <div style={{ flex: 1 }}>
            <Text strong style={{ fontSize: 13 }}>穿透方式</Text>
            <div style={{ fontSize: 11, color: "var(--semi-color-text-2)", marginTop: 2 }}>选择内置 FRP 或填写外部穿透地址</div>
          </div>
          <div style={{ display: "flex", gap: 6 }}>
            {(["external", "frp"] as const).map(m => (
              <Tag key={m}
                color={config.tunnelMode === m ? "blue" : "grey"}
                type={config.tunnelMode === m ? "solid" : "ghost"}
                style={{ cursor: "pointer", padding: "4px 14px", fontSize: 13 }}
                onClick={() => { handleModeSwitch(m); }}>
                {m === "external" ? "外部穿透" : "内置 FRP"}
              </Tag>
            ))}
          </div>
        </div>
      </Card>

      {/* 状态卡片 */}
      <Card bodyStyle={{ padding: "16px" }} style={{ borderRadius: 10, marginBottom: 16 }}>
        <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
          <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
            <div style={{
              width: 10, height: 10, borderRadius: "50%",
              background: status.running ? "#52c41a" : "#999",
              boxShadow: status.running ? "0 0 8px #52c41a" : "none"
            }} />
            <div>
              <Text strong>{status.running ? "穿透运行中" : "穿透未启动"}</Text>
              {status.publicUrl && (
                <div style={{ marginTop: 4 }}>
                  <Tag color="blue" style={{ cursor: "pointer" }} onClick={() => {
                    navigator.clipboard.writeText(status.publicUrl + "/v1");
                    Toast.success({ content: "已复制公网地址" });
                  }}>
                    <IconLink style={{ marginRight: 4 }} />
                    {status.publicUrl}/v1
                  </Tag>
                </div>
              )}
              {status.error && <Text type="danger" size="small" style={{ display: "block", marginTop: 4 }}>{status.error}</Text>}
            </div>
          </div>
          <Space>
            {!isExternal && (
              status.running ? (
                <Button type="danger" icon={<IconStop />} loading={loading} onClick={handleStop}>停止穿透</Button>
              ) : (
                <Button type="primary" theme="solid" icon={<IconPlay />} loading={loading} onClick={handleStart}>启动穿透</Button>
              )
            )}
          </Space>
        </div>
      </Card>

      {/* 外部穿透模式 */}
      {isExternal ? (
        <>
          <Card bodyStyle={{ padding: "4px 16px" }} style={{ borderRadius: 10, marginBottom: 12 }}>
            <Text strong size="small" style={{ display: "block", padding: "10px 0 4px", color: "var(--semi-color-primary)" }}>外部穿透地址</Text>
            <div style={{ ...rowStyle, borderBottom: "none" }}>
              <div style={{ flex: 1 }}>
                <div style={labelStyle}>穿透域名 / URL</div>
                <div style={descStyle}>填写第三方穿透服务提供的公网地址，如小蜜球、花生壳等</div>
              </div>
            </div>
            <div style={{ display: "flex", gap: 8, paddingBottom: 12 }}>
              <Input
                style={{ flex: 1 }}
                placeholder="如 d4mpfjxfo0wb.vip3.xiaomiqiu123.top"
                value={externalInput}
                onChange={v => setExternalInput(v)}
                onEnterPress={handleSetExternal}
              />
              <Button type="primary" theme="solid" loading={loading} onClick={handleSetExternal}>
                {status.running && config.externalUrl ? "更新" : "启用"}
              </Button>
              {status.running && config.externalUrl && (
                <Button type="danger" loading={loading} onClick={handleClearExternal}>清除</Button>
              )}
            </div>
          </Card>

          <Card bodyStyle={{ padding: "16px" }} style={{ borderRadius: 10, background: "var(--semi-color-fill-0)" }}>
            <Text strong size="small" style={{ display: "block", marginBottom: 8 }}>使用说明</Text>
            <div style={{ fontSize: 12, color: "var(--semi-color-text-2)", lineHeight: 1.8 }}>
              <div>1. 使用第三方穿透工具（如小蜜球、花生壳、Ngrok 等）将本地代理端口映射到公网</div>
              <div>2. 将获得的公网域名粘贴到上方输入框，点击「启用」</div>
              <div>3. 公网地址会自动显示在上方状态栏，点击即可复制</div>
              <div style={{ marginTop: 8, color: "var(--semi-color-warning)" }}>注意：请确保代理服务配置中 Host 设为 0.0.0.0，否则外部无法访问</div>
            </div>
          </Card>
        </>
      ) : (
        <>
          {/* FRP 服务器配置 */}
          <Card bodyStyle={{ padding: "4px 16px" }} style={{ borderRadius: 10, marginBottom: 12 }}>
            <Text strong size="small" style={{ display: "block", padding: "10px 0 4px", color: "var(--semi-color-primary)" }}>FRP 服务器</Text>

            <div style={rowStyle}>
              <div style={{ flex: 1 }}><div style={labelStyle}>服务器地址</div><div style={descStyle}>FRP 服务端 IP 或域名</div></div>
              <Input size="small" style={{ width: 200 }} placeholder="如 1.2.3.4" value={config.serverAddr} onChange={v => update("serverAddr", v)} />
            </div>

            <div style={rowStyle}>
              <div style={{ flex: 1 }}><div style={labelStyle}>服务器端口</div><div style={descStyle}>FRP 服务端 bindPort，默认 7000</div></div>
              <Input size="small" style={{ width: 100 }} type="number" value={String(config.serverPort)} onChange={v => update("serverPort", Number(v))} />
            </div>

            <div style={{ ...rowStyle, borderBottom: "none" }}>
              <div style={{ flex: 1 }}><div style={labelStyle}>认证 Token</div><div style={descStyle}>与服务端 auth.token 一致</div></div>
              <Input size="small" style={{ width: 200 }} mode="password" placeholder="frps.toml 中的 token" value={config.token} onChange={v => update("token", v)} />
            </div>
          </Card>

          {/* 代理配置 */}
          <Card bodyStyle={{ padding: "4px 16px" }} style={{ borderRadius: 10, marginBottom: 12 }}>
            <Text strong size="small" style={{ display: "block", padding: "10px 0 4px", color: "var(--semi-color-primary)" }}>代理配置</Text>

            <div style={rowStyle}>
              <div style={{ flex: 1 }}><div style={labelStyle}>代理名称</div><div style={descStyle}>FRP 代理标识名（需唯一）</div></div>
              <Input size="small" style={{ width: 180 }} value={config.proxyName} onChange={v => update("proxyName", v)} />
            </div>

            <div style={rowStyle}>
              <div style={{ flex: 1 }}><div style={labelStyle}>穿透模式</div><div style={descStyle}>HTTP 需域名，TCP 需端口</div></div>
              <div style={{ display: "flex", gap: 6 }}>
                {["http", "tcp"].map(t => (
                  <Tag key={t} color={config.proxyType === t ? "blue" : "grey"} type={config.proxyType === t ? "solid" : "ghost"}
                    style={{ cursor: "pointer", padding: "2px 12px" }}
                    onClick={() => update("proxyType", t)}>{t.toUpperCase()}</Tag>
                ))}
              </div>
            </div>

            {config.proxyType === "http" ? (
              <>
                <div style={rowStyle}>
                  <div style={{ flex: 1 }}><div style={labelStyle}>自定义域名</div><div style={descStyle}>需解析到服务器 IP</div></div>
                  <Input size="small" style={{ width: 200 }} placeholder="如 kiro.example.com" value={config.customDomain} onChange={v => update("customDomain", v)} />
                </div>
                <div style={{ ...rowStyle, borderBottom: "none" }}>
                  <div style={{ flex: 1 }}><div style={labelStyle}>HTTP 端口</div><div style={descStyle}>服务端 vhostHTTPPort，默认 8080</div></div>
                  <Input size="small" style={{ width: 100 }} type="number" value={String(config.vhostHTTPPort || 8080)} onChange={v => update("vhostHTTPPort", Number(v))} />
                </div>
              </>
            ) : (
              <div style={{ ...rowStyle, borderBottom: "none" }}>
                <div style={{ flex: 1 }}><div style={labelStyle}>远程端口</div><div style={descStyle}>服务器上暴露的端口号</div></div>
                <Input size="small" style={{ width: 100 }} type="number" placeholder="如 13000" value={String(config.remotePort || "")} onChange={v => update("remotePort", Number(v))} />
              </div>
            )}
          </Card>

          {/* 部署说明 */}
          <Card bodyStyle={{ padding: "16px" }} style={{ borderRadius: 10, background: "var(--semi-color-fill-0)" }}>
            <Text strong size="small" style={{ display: "block", marginBottom: 8 }}>自建 FRP 服务器</Text>
            <pre style={{ margin: 0, padding: "10px 12px", borderRadius: 6, background: "var(--semi-color-bg-1)", border: "1px solid var(--semi-color-border)", fontSize: 11, fontFamily: "'SF Mono', 'Fira Code', monospace", lineHeight: 1.6, whiteSpace: "pre-wrap", wordBreak: "break-all", color: "var(--semi-color-text-1)" }}>{`# SSH 到 Linux 服务器执行：
cd /opt && wget -qO- https://github.com/fatedier/frp/releases/download/v0.67.0/frp_0.67.0_linux_amd64.tar.gz | tar xz
cd frp_0.67.0_linux_amd64

# 写配置（改密码）
cat > frps.toml << 'EOF'
bindPort = 7000
auth.token = "你的密码"
vhostHTTPPort = 8080
EOF

# 启动
./frps -c frps.toml &

# 云控制台开放 7000 + 8080 端口
# 将 IP、端口、密码填入上方配置保存即可`}</pre>
          </Card>
        </>
      )}
    </div>
  );
}
