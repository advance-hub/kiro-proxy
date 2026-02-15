import React, { useState, useEffect, useCallback } from "react";
import { Button, Card, Input, Typography, Toast, Space, Select } from "@douyinfe/semi-ui";
import { IconRefresh } from "@douyinfe/semi-icons";

const { Text } = Typography;

const wails = () => {
  if (!window.go?.main?.App) throw new Error("Wails runtime 尚未就绪");
  return window.go.main.App;
};

export default function ClaudeCodePanel() {
  const [config, setConfig] = useState<any>({});
  const [loading, setLoading] = useState(false);
  const [syncing, setSyncing] = useState(false);
  const [proxyHost, setProxyHost] = useState("127.0.0.1");
  const [proxyPort, setProxyPort] = useState("13000");
  const [proxyApiKey, setProxyApiKey] = useState("kiro-proxy-123");
  const [defaultModel, setDefaultModel] = useState("claude-opus-4-5-20251101");
  const [modelOptions, setModelOptions] = useState<{label: string; value: string}[]>([
    { label: "[Kiro] Claude Sonnet 4.5", value: "claude-sonnet-4-5-20250929" },
    { label: "[Kiro] Claude Sonnet 4.5 (Thinking)", value: "claude-sonnet-4-5-20250929-thinking" },
    { label: "[Kiro] Claude Opus 4.5", value: "claude-opus-4-5-20251101" },
    { label: "[Kiro] Claude Opus 4.5 (Thinking)", value: "claude-opus-4-5-20251101-thinking" },
  ]);

  const loadConfig = useCallback(async () => {
    try {
      const c = await wails().ReadClaudeCodeSettings();
      setConfig(c || {});
      const env = c?.env || {};
      if (env.ANTHROPIC_MODEL) { setDefaultModel(String(env.ANTHROPIC_MODEL).replace(/^kiroproxy\//, "")); }
    } catch (e) { Toast.error({ content: String(e) }); }
  }, []);

  const loadProxyConfig = useCallback(async () => {
    try { const s = await wails().GetConfig(); if (s) { setProxyHost(s.host || "127.0.0.1"); setProxyPort(String(s.port || 13000)); setProxyApiKey(s.apiKey || "kiro-proxy-123"); } } catch (e) { Toast.error({ content: `加载代理配置失败: ${e}` }); }
  }, []);

  const loadModels = useCallback(async () => {
    try {
      let h = proxyHost, p = proxyPort, k = proxyApiKey;
      try { const s = await wails().GetConfig(); if (s) { h = s.host || h; p = String(s.port || 13000); k = s.apiKey || k; } } catch (_) {}
      const resp = await fetch(`http://${h}:${p}/v1/models`, { headers: { "x-api-key": k } });
      if (!resp.ok) return;
      const data = await resp.json();
      const models: any[] = data.data || [];
      const opts = models.map((m: any) => ({ label: `[Kiro] ${m.display_name || m.id}`, value: m.id }));
      if (opts.length > 0) setModelOptions(opts);
    } catch (_) {}
  }, [proxyHost, proxyPort, proxyApiKey]);

  useEffect(() => { loadConfig(); loadProxyConfig(); loadModels(); }, [loadConfig, loadProxyConfig, loadModels]);

  const handleSave = async () => {
    setLoading(true);
    try { const msg = await wails().WriteClaudeCodeSettings(config); Toast.success({ content: msg }); } catch (e) { Toast.error({ content: String(e) }); } finally { setLoading(false); }
  };

  const handleSyncConfig = async () => {
    setSyncing(true);
    try {
      let currentHost = proxyHost, currentPort = proxyPort, currentApiKey = proxyApiKey;
      try { const s = await wails().GetConfig(); if (s) { currentHost = s.host || "127.0.0.1"; currentPort = String(s.port || 13000); currentApiKey = s.apiKey || "kiro-proxy-123"; setProxyHost(currentHost); setProxyPort(currentPort); setProxyApiKey(currentApiKey); } } catch (_) {}
      const baseUrl = `http://${currentHost}:${currentPort}`;
      const PREFIX = "kiroproxy/";
      let resp: Response;
      try { resp = await fetch(`${baseUrl}/v1/models`, { headers: { "x-api-key": currentApiKey } }); } catch (_) { Toast.warning({ content: `无法连接代理 ${baseUrl}，请先启动代理` }); return; }
      if (!resp.ok) throw new Error(`获取模型列表失败: HTTP ${resp.status}`);
      const data = await resp.json();
      const models: any[] = data.data || [];
      if (models.length === 0) { Toast.warning({ content: "代理未返回任何模型，请先启动代理" }); return; }

      const opts = models.map((m: any) => ({ label: `[Kiro] ${m.display_name || m.id}`, value: m.id }));
      setModelOptions(opts);

      const modelWithPrefix = defaultModel.startsWith(PREFIX) ? defaultModel : `${PREFIX}${defaultModel}`;
      const envConfig: Record<string, string> = { ANTHROPIC_AUTH_TOKEN: currentApiKey, ANTHROPIC_BASE_URL: baseUrl, ANTHROPIC_MODEL: modelWithPrefix, CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC: "1", CLAUDE_CODE_MAX_OUTPUT_TOKENS: "102000" };

      const selectedType = defaultModel.includes("haiku") ? "haiku" : defaultModel.includes("sonnet") ? "sonnet" : defaultModel.includes("opus") ? "opus" : "";
      for (const m of models) {
        const id: string = m.id;
        if (id.includes("thinking")) continue;
        if (id.includes("haiku") && (!envConfig.ANTHROPIC_DEFAULT_HAIKU_MODEL || selectedType === "haiku")) envConfig.ANTHROPIC_DEFAULT_HAIKU_MODEL = selectedType === "haiku" ? modelWithPrefix : `${PREFIX}${id}`;
        if (id.includes("sonnet") && (!envConfig.ANTHROPIC_DEFAULT_SONNET_MODEL || selectedType === "sonnet")) envConfig.ANTHROPIC_DEFAULT_SONNET_MODEL = selectedType === "sonnet" ? modelWithPrefix : `${PREFIX}${id}`;
        if (id.includes("opus") && (!envConfig.ANTHROPIC_DEFAULT_OPUS_MODEL || selectedType === "opus")) envConfig.ANTHROPIC_DEFAULT_OPUS_MODEL = selectedType === "opus" ? modelWithPrefix : `${PREFIX}${id}`;
      }

      const newConfig: any = { ...config, env: { ...(config.env || {}), ...envConfig }, includeCoAuthoredBy: false };
      delete newConfig.allowedModels;
      setConfig(newConfig);
      await wails().WriteClaudeCodeSettings(newConfig);
      Toast.success({ content: "已同步配置到 ~/.claude/settings.json，请重启 Claude Code" });
    } catch (e) { Toast.error({ content: `同步失败: ${e}` }); } finally { setSyncing(false); }
  };

  const env = config?.env || {};
  const envEntries = Object.entries(env);

  return (
    <div style={{ padding: "20px 24px 32px" }}>
      <Card bodyStyle={{ padding: "14px 20px" }} style={{ marginBottom: 12, borderRadius: 10 }}>
        <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: 12 }}>
          <Text strong>一键配置</Text>
          <Space>
            <Button size="small" theme="borderless" icon={<IconRefresh />} onClick={loadConfig}>刷新</Button>
            <Button size="small" theme="solid" type="primary" loading={syncing} onClick={handleSyncConfig}>同步到 Claude Code</Button>
          </Space>
        </div>
        <Text type="tertiary" size="small" style={{ display: "block", marginBottom: 12 }}>自动生成 Claude Code 配置并写入 ~/.claude/settings.json，同步后需重启 Claude Code 客户端</Text>
        <div style={{ marginBottom: 8 }}>
          <Text size="small" style={{ display: "block", marginBottom: 4, fontWeight: 500 }}>默认模型 (ANTHROPIC_MODEL)</Text>
          <Select size="small" value={defaultModel} onChange={(v) => setDefaultModel(v as string)} style={{ width: "100%" }} optionList={modelOptions} />
        </div>
      </Card>

      {envEntries.length > 0 && (
        <Card bodyStyle={{ padding: "14px 20px" }} style={{ marginBottom: 12, borderRadius: 10 }}>
          <Text strong size="small" style={{ display: "block", marginBottom: 10, color: "var(--semi-color-primary)" }}>当前环境变量</Text>
          {envEntries.map(([key, val]) => (
            <div key={key} style={{ display: "flex", alignItems: "center", justifyContent: "space-between", padding: "6px 10px", borderRadius: 4, marginBottom: 4, background: "var(--semi-color-fill-0)" }}>
              <Text size="small" style={{ fontFamily: "monospace", fontWeight: 500 }}>{key}</Text>
              <Text size="small" type="tertiary" style={{ fontFamily: "monospace", wordBreak: "break-all", textAlign: "right", flexShrink: 0 }}>{String(val)}</Text>
            </div>
          ))}
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
