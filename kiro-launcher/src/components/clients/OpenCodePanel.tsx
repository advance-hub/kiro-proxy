import React, { useState, useEffect, useCallback } from "react";
import { Button, Card, Input, Typography, Tag, Toast, Space } from "@douyinfe/semi-ui";
import { IconRefresh } from "@douyinfe/semi-icons";

const { Text } = Typography;

const wails = () => {
  if (!window.go?.main?.App) throw new Error("Wails runtime 尚未就绪");
  return window.go.main.App;
};

export default function OpenCodePanel() {
  const [config, setConfig] = useState<any>({});
  const [loading, setLoading] = useState(false);
  const [syncing, setSyncing] = useState(false);
  const [proxyHost, setProxyHost] = useState("127.0.0.1");
  const [proxyPort, setProxyPort] = useState("13000");
  const [proxyApiKey, setProxyApiKey] = useState("kiro-proxy-123");

  const loadConfig = useCallback(async () => {
    try { const c = await wails().ReadOpenCodeConfig(); setConfig(c || {}); } catch (e) { Toast.error({ content: String(e) }); }
  }, []);

  const loadProxyConfig = useCallback(async () => {
    try { const s = await wails().GetConfig(); if (s) { setProxyHost(s.host || "127.0.0.1"); setProxyPort(String(s.port || 13000)); setProxyApiKey(s.apiKey || "kiro-proxy-123"); } } catch (e) { Toast.error({ content: `加载代理配置失败: ${e}` }); }
  }, []);

  useEffect(() => { loadConfig(); loadProxyConfig(); }, [loadConfig, loadProxyConfig]);

  const handleSave = async () => {
    setLoading(true);
    try { const msg = await wails().WriteOpenCodeConfig(config); Toast.success({ content: msg }); } catch (e) { Toast.error({ content: String(e) }); } finally { setLoading(false); }
  };

  const handleSyncModels = async () => {
    setSyncing(true);
    try {
      let currentHost = proxyHost, currentPort = proxyPort, currentApiKey = proxyApiKey;
      try { const s = await wails().GetConfig(); if (s) { currentHost = s.host || "127.0.0.1"; currentPort = String(s.port || 13000); currentApiKey = s.apiKey || "kiro-proxy-123"; setProxyHost(currentHost); setProxyPort(currentPort); setProxyApiKey(currentApiKey); } } catch (_) {}
      const baseUrl = `http://${currentHost}:${currentPort}`;
      const resp = await fetch(`${baseUrl}/v1/models`, { headers: { "x-api-key": currentApiKey } });
      if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
      const data = await resp.json();
      const modelList: any[] = data.data || [];
      if (modelList.length === 0) { Toast.warning({ content: "代理未返回任何模型，请先启动代理" }); return; }

      const providerKey = "kiro-proxy";
      const models: any = {};
      for (const m of modelList) { models[m.id] = { limit: { context: 200000, output: m.max_tokens || 8192 }, modalities: { input: ["text", "image"], output: ["text"] } }; }
      const provider: any = { [providerKey]: { npm: "@ai-sdk/anthropic", name: "Kiro Proxy", options: { apiKey: currentApiKey, baseURL: `${baseUrl}/v1` }, models } };
      const newConfig = { ...config, $schema: "https://opencode.ai/config.json", provider: { ...(config.provider || {}), ...provider } };
      delete newConfig.extensions; delete newConfig.models;
      setConfig(newConfig);
      await wails().WriteOpenCodeConfig(newConfig);
      Toast.success({ content: `已同步 ${modelList.length} 个模型到 opencode.json` });
    } catch (e) { Toast.error({ content: `同步失败: ${e}` }); } finally { setSyncing(false); }
  };

  const providerEntries = Object.entries(config.provider || {});

  return (
    <div style={{ padding: "20px 24px 32px" }}>
      <Card bodyStyle={{ padding: "14px 20px" }} style={{ marginBottom: 12, borderRadius: 10 }}>
        <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: 12 }}>
          <Text strong>模型同步</Text>
          <Space>
            <Button size="small" theme="borderless" icon={<IconRefresh />} onClick={loadConfig}>刷新</Button>
            <Button size="small" theme="solid" type="primary" loading={syncing} onClick={handleSyncModels}>从代理同步模型</Button>
          </Space>
        </div>
        <Text type="tertiary" size="small" style={{ display: "block", marginBottom: 12 }}>自动从本地代理获取模型列表，映射写入 ~/.config/opencode/opencode.json</Text>
      </Card>

      {providerEntries.length > 0 && (
        <Card bodyStyle={{ padding: "14px 20px" }} style={{ marginBottom: 12, borderRadius: 10 }}>
          <Text strong size="small" style={{ display: "block", marginBottom: 10, color: "var(--semi-color-primary)" }}>Providers</Text>
          {providerEntries.map(([key, prov]: [string, any]) => {
            const modelIds = Object.keys(prov?.models || {});
            return (
              <div key={key} style={{ padding: "10px 12px", borderRadius: 6, marginBottom: 6, background: "var(--semi-color-fill-0)", border: "1px solid var(--semi-color-border)" }}>
                <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: 4 }}>
                  <Text strong size="small">{prov?.name || key}</Text>
                  <Space><Tag size="small" color="blue" type="light">{prov?.npm || "unknown"}</Tag><Tag size="small" color="green" type="light">{modelIds.length} 模型</Tag></Space>
                </div>
                {prov?.options?.baseURL && <Text type="tertiary" size="small" style={{ fontFamily: "monospace", display: "block", marginBottom: 6 }}>{prov.options.baseURL}</Text>}
                {modelIds.map((mid: string) => <div key={mid} style={{ padding: "4px 8px", borderRadius: 4, marginTop: 4, background: "var(--semi-color-bg-1)" }}><Text size="small" style={{ fontFamily: "monospace" }}>{mid}</Text></div>)}
              </div>
            );
          })}
        </Card>
      )}

      <Card bodyStyle={{ padding: "14px 20px" }} style={{ borderRadius: 10 }}>
        <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: 10 }}>
          <Text strong size="small" style={{ color: "var(--semi-color-primary)" }}>原始配置</Text>
          <Button size="small" theme="light" type="primary" loading={loading} onClick={handleSave}>保存</Button>
        </div>
        <Input value={JSON.stringify(config, null, 2)} onChange={(v) => { try { setConfig(JSON.parse(v)); } catch (_) {} }} style={{ fontFamily: "monospace", fontSize: 11 }} />
      </Card>
    </div>
  );
}
