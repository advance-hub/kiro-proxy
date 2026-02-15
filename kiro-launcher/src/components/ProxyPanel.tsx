import React, { useState, useEffect, useCallback } from "react";
import { Button, Card, Input, Typography, Tag, Toast, Collapsible, Space, Divider, Tooltip } from "@douyinfe/semi-ui";
import { IconPlay, IconStop, IconRefresh, IconSetting, IconCopy, IconTick, IconChevronDown, IconChevronRight, IconLink, IconCheckCircleStroked, IconAlertCircle, IconInfoCircle } from "@douyinfe/semi-icons";

const { Text } = Typography;

const wails = () => {
  if (!window.go?.main?.App) throw new Error("Wails runtime 尚未就绪");
  return window.go.main.App;
};

interface ProxyConfig { host: string; port: number; apiKey: string; region: string; tlsBackend: string; }
interface StatusInfo { running: boolean; has_credentials: boolean; config: ProxyConfig; }
interface ProxyModel { id: string; display_name: string; owned_by: string; max_tokens: number; type: string; }

export default function ProxyPanel() {
  const [status, setStatus] = useState<StatusInfo | null>(null);
  const [host, setHost] = useState("127.0.0.1");
  const [port, setPort] = useState("13000");
  const [apiKey, setApiKey] = useState("kiro-proxy-123");
  const [region, setRegion] = useState("us-east-1");
  const [loading, setLoading] = useState("");
  const [showConfig, setShowConfig] = useState(false);
  const [copied, setCopied] = useState(false);
  const [models, setModels] = useState<ProxyModel[]>([]);
  const [modelsLoading, setModelsLoading] = useState(false);

  const refreshStatus = useCallback(async () => {
    try {
      const s = await wails().GetStatus() as StatusInfo;
      setStatus(s);
      if (s.config) { setHost(s.config.host); setPort(String(s.config.port)); setApiKey(s.config.apiKey); setRegion(s.config.region); }
    } catch (_) {}
  }, []);

  useEffect(() => { refreshStatus(); const t = setInterval(refreshStatus, 3000); return () => clearInterval(t); }, [refreshStatus]);

  const wrap = async (key: string, fn: () => Promise<void>) => {
    setLoading(key);
    try { await fn(); } catch (e) { Toast.error({ content: String(e) }); } finally { setLoading(""); }
  };

  const fetchModels = async (h: string, p: string, k: string) => {
    setModelsLoading(true);
    try {
      const resp = await fetch(`http://${h}:${p}/v1/models`, { headers: { "x-api-key": k } });
      if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
      const data = await resp.json();
      const list: ProxyModel[] = (data.data || []).map((m: any) => ({ id: m.id, display_name: m.display_name || m.id, owned_by: m.owned_by || "unknown", max_tokens: m.max_tokens || 8192, type: m.type || "chat" }));
      setModels(list);
      return list;
    } catch (e) { Toast.warning({ content: `获取模型列表失败: ${e}` }); return []; }
    finally { setModelsLoading(false); }
  };

  const syncModelsToFactory = async (h: string, p: string, k: string) => {
    const baseUrl = `http://${h}:${p}`;
    const list = await fetchModels(h, p, k);
    if (list.length === 0) return;
    try {
      const configModels = list.map((m) => ({ model_display_name: `${m.display_name} [Kiro]`, model: m.id, base_url: baseUrl, api_key: k, provider: "anthropic", supports_vision: true, max_tokens: m.max_tokens }));
      await wails().WriteFactoryConfig({ custom_models: configModels });
      const customModels = list.map((m, i) => ({ displayName: `${m.display_name} [Kiro]`, id: `custom:${m.display_name.replace(/[\s()]/g, "-")}-[Kiro]-${i}`, index: i, model: m.id, baseUrl, apiKey: k, provider: "anthropic", noImageSupport: false, maxOutputTokens: m.max_tokens }));
      const settings = await wails().ReadDroidSettings() as any;
      settings.customModels = customModels;
      await wails().WriteDroidSettings(settings);
      Toast.success({ content: `已同步 ${list.length} 个模型` });
    } catch (e) { Toast.warning({ content: `模型同步失败: ${e}` }); }
  };

  const handleOneClick = () => wrap("start", async () => {
    await wails().EnsureFactoryApiKey();
    await wails().SaveConfig(host, Number(port), apiKey, region);
    const r = await wails().OneClickStart();
    Toast.success({ content: r });
    await refreshStatus();
    setTimeout(() => syncModelsToFactory(host, port, apiKey), 1500);
  });

  const handleStop = async () => { await wails().StopProxy(); Toast.success({ content: "代理已停止" }); await refreshStatus(); };

  const handleSaveConfig = async () => {
    try {
      await wails().SaveConfig(host, Number(port), apiKey, region);
      if (status?.running) { await wails().StopProxy(); const r = await wails().StartProxy(); Toast.success({ content: `配置已保存，代理已重启: ${r}` }); }
      else { Toast.success({ content: "配置已保存" }); }
      await refreshStatus();
    } catch (e) { Toast.error({ content: String(e) }); }
  };

  const handleCopy = () => { navigator.clipboard.writeText(`ANTHROPIC_BASE_URL=http://${host}:${port}\nANTHROPIC_API_KEY=${apiKey}`); setCopied(true); setTimeout(() => setCopied(false), 2000); };

  const running = status?.running ?? false;

  return (
    <div style={{ padding: "20px 24px 32px" }}>
      <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: 20 }}>
        <div><Text strong style={{ fontSize: 18, display: "block" }}>代理服务</Text><Text type="tertiary" size="small">管理本地代理，转发 API 请求</Text></div>
      </div>

      <Card bodyStyle={{ padding: 0 }} style={{ marginBottom: 12, overflow: "hidden", borderRadius: 12 }}>
        <div style={{ padding: "16px 20px", display: "flex", alignItems: "center", gap: 12 }}>
          <div style={{ width: 40, height: 40, borderRadius: "50%", background: running ? "#e8f5e9" : "var(--semi-color-fill-0)", display: "flex", alignItems: "center", justifyContent: "center" }}>
            {running ? <IconCheckCircleStroked style={{ color: "#00b365", fontSize: 20 }} /> : <IconAlertCircle style={{ color: "#bbb", fontSize: 20 }} />}
          </div>
          <div style={{ flex: 1 }}>
            <Text strong>{running ? "代理服务运行中" : "代理服务未启动"}</Text>
            <div style={{ marginTop: 2 }}>{running ? <Text type="tertiary" size="small" copyable style={{ fontFamily: "monospace" }}>{`http://${host}:${port}`}</Text> : <Text type="tertiary" size="small">点击下方按钮启动服务</Text>}</div>
          </div>
        </div>
        <Divider style={{ margin: 0 }} />
        <div style={{ padding: "12px 20px" }}>
          {running ? <Button type="danger" theme="solid" block icon={<IconStop />} onClick={handleStop}>停止代理</Button> : <Button type="primary" theme="solid" block icon={<IconPlay />} loading={loading === "start"} onClick={handleOneClick}>一键启动</Button>}
        </div>
      </Card>

      <Card bodyStyle={{ padding: "14px 20px" }} style={{ marginBottom: 12, borderRadius: 10 }}>
        <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: 10 }}><Space><IconLink style={{ color: "var(--semi-color-text-2)" }} /><Text strong>API 端点</Text></Space></div>
        <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
          {[{ label: "Anthropic 原生格式", path: "/v1/messages" }, { label: "OpenAI 兼容格式 ⭐", path: "/v1/chat/completions" }, { label: "模型列表", path: "/v1/models" }].map(ep => (
            <div key={ep.path} style={{ padding: "8px 10px", borderRadius: 4, background: "var(--semi-color-fill-0)", border: "1px solid var(--semi-color-border)" }}>
              <Text size="small" type="secondary" style={{ display: "block", marginBottom: 4 }}>{ep.label}</Text>
              <Text size="small" copyable style={{ fontFamily: "monospace", fontSize: 11 }}>{`http://${host}:${port}${ep.path}`}</Text>
            </div>
          ))}
        </div>
      </Card>

      {models.length > 0 && (
        <Card bodyStyle={{ padding: "14px 20px" }} style={{ marginBottom: 12, borderRadius: 10 }}>
          <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: 10 }}>
            <Space><IconInfoCircle style={{ color: "var(--semi-color-text-2)" }} /><Text strong>可用模型</Text><Tag size="small" color="blue" type="light">{models.length}</Tag></Space>
            <Button size="small" theme="borderless" type="tertiary" icon={<IconRefresh />} loading={modelsLoading} onClick={() => fetchModels(host, port, apiKey)}>刷新</Button>
          </div>
          <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
            {models.map((m) => (
              <div key={m.id} style={{ padding: "8px 12px", borderRadius: 6, background: "var(--semi-color-fill-0)", border: "1px solid var(--semi-color-border)" }}>
                <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: 4 }}><Text strong size="small">{m.display_name}</Text><Tag size="small" color="green" type="light">{m.type}</Tag></div>
                <div style={{ display: "flex", alignItems: "center", gap: 8 }}><Text type="tertiary" size="small" style={{ fontFamily: "monospace" }}>{m.id}</Text><Text type="tertiary" size="small">|</Text><Text type="tertiary" size="small">{m.owned_by}</Text><Text type="tertiary" size="small">|</Text><Text type="tertiary" size="small">max: {m.max_tokens.toLocaleString()}</Text></div>
              </div>
            ))}
          </div>
        </Card>
      )}

      <Card bodyStyle={{ padding: "14px 20px" }} style={{ marginBottom: 12, borderRadius: 10 }}>
        <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: 10 }}><Space><IconLink style={{ color: "var(--semi-color-text-2)" }} /><Text strong>客户端配置</Text></Space><Tooltip content={copied ? "已复制" : "复制到剪贴板"}><Button size="small" theme="borderless" type="tertiary" icon={copied ? <IconTick style={{ color: "#00b365" }} /> : <IconCopy />} onClick={handleCopy} /></Tooltip></div>
        <pre style={{ margin: 0, padding: "10px 12px", borderRadius: 6, background: "var(--semi-color-fill-0)", border: "1px solid var(--semi-color-border)", fontSize: 12, fontFamily: "'SF Mono', 'Fira Code', monospace", color: "var(--semi-color-text-0)", lineHeight: 1.8, whiteSpace: "pre-wrap", wordBreak: "break-all" }}>{`ANTHROPIC_BASE_URL=http://${host}:${port}\nANTHROPIC_API_KEY=${apiKey}`}</pre>
      </Card>

      <Card bodyStyle={{ padding: "14px 20px" }} style={{ marginBottom: 12, borderRadius: 10 }}>
        <Space style={{ marginBottom: 10 }}><IconInfoCircle style={{ color: "var(--semi-color-text-2)" }} /><Text strong>API 端点</Text></Space>
        <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
          {[{ method: "GET", path: "/v1/models", desc: "模型列表" }, { method: "POST", path: "/v1/messages", desc: "对话" }, { method: "POST", path: "/v1/messages/count_tokens", desc: "Token 估算" }, { method: "POST", path: "/cc/v1/messages", desc: "对话 (Claude Code)" }, { method: "POST", path: "/cc/v1/messages/count_tokens", desc: "Token 估算 (CC)" }].map((ep) => (
            <div key={ep.path} style={{ display: "flex", alignItems: "center", gap: 8, padding: "6px 10px", borderRadius: 4, background: "var(--semi-color-fill-0)" }}>
              <Tag size="small" color={ep.method === "GET" ? "green" : "blue"} type="light" style={{ fontFamily: "monospace", fontSize: 11, minWidth: 42, textAlign: "center" }}>{ep.method}</Tag>
              <Text style={{ fontFamily: "monospace", fontSize: 12, flex: 1 }}>{ep.path}</Text>
              <Text type="tertiary" size="small">{ep.desc}</Text>
            </div>
          ))}
        </div>
      </Card>

      <Card bodyStyle={{ padding: 0 }} style={{ borderRadius: 10 }}>
        <div style={{ padding: "12px 20px", cursor: "pointer", userSelect: "none", display: "flex", alignItems: "center", justifyContent: "space-between" }} onClick={() => setShowConfig(!showConfig)}>
          <Space><IconSetting style={{ color: "var(--semi-color-text-2)", fontSize: 14 }} /><Text strong size="small">高级配置</Text></Space>
          {showConfig ? <IconChevronDown size="small" /> : <IconChevronRight size="small" />}
        </div>
        <Collapsible isOpen={showConfig}>
          <Divider style={{ margin: 0 }} />
          <div style={{ padding: "14px 20px" }}>
            <div style={{ display: "grid", gridTemplateColumns: "1fr 100px", gap: 8, marginBottom: 10 }}>
              <div><Text size="small" type="secondary" style={{ marginBottom: 2, display: "block" }}>监听地址</Text><Input size="small" value={host} onChange={setHost} /></div>
              <div><Text size="small" type="secondary" style={{ marginBottom: 2, display: "block" }}>端口</Text><Input size="small" value={port} onChange={setPort} /></div>
            </div>
            <div style={{ marginBottom: 10 }}><Text size="small" type="secondary" style={{ marginBottom: 2, display: "block" }}>API Key</Text><Input size="small" value={apiKey} onChange={setApiKey} /></div>
            <div style={{ marginBottom: 14 }}><Text size="small" type="secondary" style={{ marginBottom: 2, display: "block" }}>Region</Text><Input size="small" value={region} onChange={setRegion} /></div>
            <Button size="small" theme="light" block onClick={handleSaveConfig}>保存配置</Button>
          </div>
        </Collapsible>
      </Card>
    </div>
  );
}
