import React, { useState, useEffect, useCallback } from "react";
import { Button, Card, Input, Typography, Tag, Toast, Space, Divider } from "@douyinfe/semi-ui";
import { IconCheckCircleStroked, IconAlertCircle, IconRefresh, IconSave } from "@douyinfe/semi-icons";

const { Text } = Typography;

const wails = () => {
  if (!window.go?.main?.App) throw new Error("Wails runtime 尚未就绪");
  return window.go.main.App;
};

export default function ClaudeCodePanel() {
  const [apiKey, setApiKey] = useState("");
  const [baseUrl, setBaseUrl] = useState("https://code.newcli.com/claude");
  const [isConfigured, setIsConfigured] = useState(false);
  const [testing, setTesting] = useState(false);
  const [saving, setSaving] = useState(false);
  const [proxyHost, setProxyHost] = useState("127.0.0.1");
  const [proxyPort, setProxyPort] = useState("13000");
  const [proxyApiKey, setProxyApiKey] = useState("kiro-proxy-123");

  const loadProxyConfig = useCallback(async () => {
    try {
      const s = await wails().GetConfig();
      if (s) {
        setProxyHost(s.host || "127.0.0.1");
        setProxyPort(String(s.port || 13000));
        setProxyApiKey(s.apiKey || "kiro-proxy-123");
      }
    } catch (_) {}
  }, []);

  const baseUrlFull = useCallback(() => `http://${proxyHost}:${proxyPort}`, [proxyHost, proxyPort]);

  const checkConfiguration = useCallback(async () => {
    try {
      const resp = await fetch(`${baseUrlFull()}/claudecode/v1/models`, {
        headers: {
          "x-api-key": proxyApiKey,
          "Authorization": `Bearer ${proxyApiKey}`,
        },
      });
      setIsConfigured(resp.ok);
    } catch (_) {
      setIsConfigured(false);
    }
  }, [baseUrlFull, proxyApiKey]);

  useEffect(() => {
    loadProxyConfig();
  }, [loadProxyConfig]);

  useEffect(() => {
    if (proxyHost && proxyPort) {
      checkConfiguration();
    }
  }, [proxyHost, proxyPort, checkConfiguration]);

  const handleTest = async () => {
    if (!apiKey.trim()) {
      Toast.warning({ content: "请输入 API Key" });
      return;
    }
    if (!baseUrl.trim()) {
      Toast.warning({ content: "请输入 Base URL" });
      return;
    }

    setTesting(true);
    try {
      // 测试连接到 Claude Code API
      const resp = await fetch(`${baseUrl}/v1/models`, {
        headers: {
          "Authorization": `Bearer ${apiKey}`,
          "anthropic-version": "2023-06-01",
        },
      });

      if (resp.ok) {
        Toast.success({ content: "连接测试成功！" });
      } else {
        const text = await resp.text();
        Toast.error({ content: `连接失败: ${resp.status} ${text}` });
      }
    } catch (e) {
      Toast.error({ content: `连接失败: ${e}` });
    } finally {
      setTesting(false);
    }
  };

  const handleSave = async () => {
    if (!apiKey.trim()) {
      Toast.warning({ content: "请输入 API Key" });
      return;
    }
    if (!baseUrl.trim()) {
      Toast.warning({ content: "请输入 Base URL" });
      return;
    }

    setSaving(true);
    try {
      // 保存配置到 config.json
      // @ts-ignore - Wails 绑定已生成，运行时可用
      await wails().SaveClaudeCodeConfig(apiKey.trim(), baseUrl.trim());
      Toast.success({ content: "配置已保存，请重启服务以生效" });
      setTimeout(() => {
        checkConfiguration();
      }, 1000);
    } catch (e) {
      Toast.error({ content: `保存失败: ${e}` });
    } finally {
      setSaving(false);
    }
  };

  return (
    <div style={{ padding: "20px 24px 32px" }}>
      <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: 20 }}>
        <div>
          <Text strong style={{ fontSize: 18, display: "block" }}>Claude Code 配置</Text>
          <Text type="tertiary" size="small">反向代理 Claude Code API，支持 Anthropic 和 OpenAI 格式</Text>
        </div>
        <Space>
          {isConfigured ? (
            <Tag color="green" size="large" type="light">✓ 已配置</Tag>
          ) : (
            <Tag color="grey" size="large" type="light">未配置</Tag>
          )}
          <Button size="small" theme="borderless" icon={<IconRefresh />} onClick={checkConfiguration}>刷新状态</Button>
        </Space>
      </div>

      {/* API 端点 */}
      <Card bodyStyle={{ padding: "14px 20px" }} style={{ marginBottom: 12, borderRadius: 10 }}>
        <Text strong size="small" style={{ display: "block", marginBottom: 8 }}>API 端点</Text>
        <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
          {[
            { method: "POST", path: "/claudecode/v1/messages", desc: "Anthropic 格式" },
            { method: "POST", path: "/claudecode/v1/chat/completions", desc: "OpenAI 格式" },
            { method: "GET", path: "/claudecode/v1/models", desc: "模型列表" },
          ].map((ep) => (
            <div key={ep.path} style={{ display: "flex", alignItems: "center", gap: 8, padding: "6px 10px", borderRadius: 6, background: "var(--semi-color-fill-0)", border: "1px solid var(--semi-color-border)" }}>
              <Tag size="small" color={ep.method === "GET" ? "green" : "blue"} type="light" style={{ fontFamily: "monospace", fontSize: 11, minWidth: 42, textAlign: "center" }}>{ep.method}</Tag>
              <Text size="small" copyable style={{ fontFamily: "monospace", fontSize: 11, flex: 1 }}>{`http://${proxyHost}:${proxyPort}${ep.path}`}</Text>
              <Text type="tertiary" size="small">{ep.desc}</Text>
            </div>
          ))}
        </div>
      </Card>

      {/* 配置表单 */}
      <Card bodyStyle={{ padding: "14px 20px" }} style={{ marginBottom: 12, borderRadius: 10 }}>
        <Text strong size="small" style={{ display: "block", marginBottom: 12 }}>配置 Claude Code</Text>
        
        <div style={{ marginBottom: 12 }}>
          <Text size="small" style={{ display: "block", marginBottom: 6 }}>Base URL</Text>
          <Input
            placeholder="https://code.newcli.com/claude"
            value={baseUrl}
            onChange={setBaseUrl}
            style={{ fontFamily: "monospace", fontSize: 12 }}
          />
          <Text type="tertiary" size="small" style={{ display: "block", marginTop: 4 }}>
            Claude Code API 的基础 URL
          </Text>
        </div>

        <div style={{ marginBottom: 12 }}>
          <Text size="small" style={{ display: "block", marginBottom: 6 }}>API Key</Text>
          <Input
            type="password"
            placeholder="sk-ant-oat01-..."
            value={apiKey}
            onChange={setApiKey}
            style={{ fontFamily: "monospace", fontSize: 12 }}
          />
          <Text type="tertiary" size="small" style={{ display: "block", marginTop: 4 }}>
            Claude Code 提供的 API Key（通常以 sk-ant-oat01- 开头）
          </Text>
        </div>

        <Divider style={{ margin: "16px 0" }} />

        <div style={{ display: "flex", gap: 8 }}>
          <Button
            theme="solid"
            type="primary"
            icon={<IconSave />}
            loading={saving}
            onClick={handleSave}
          >
            保存配置
          </Button>
          <Button
            theme="light"
            type="tertiary"
            loading={testing}
            onClick={handleTest}
          >
            测试连接
          </Button>
        </div>

        <div style={{ marginTop: 12, padding: "10px 12px", borderRadius: 6, background: "var(--semi-color-warning-light-default)", border: "1px solid var(--semi-color-warning-light-active)" }}>
          <Text size="small" style={{ color: "var(--semi-color-warning-dark)" }}>
            ⚠️ 配置保存后需要重启 kiro-go 服务才能生效
          </Text>
        </div>
      </Card>

      {/* 支持的模型 */}
      <Card bodyStyle={{ padding: "14px 20px" }} style={{ borderRadius: 10 }}>
        <Text strong size="small" style={{ display: "block", marginBottom: 8 }}>支持的模型</Text>
        <div style={{ display: "flex", flexWrap: "wrap", gap: 6 }}>
          {[
            "claude-3-5-sonnet-20241022",
            "claude-3-5-haiku-20241022",
            "claude-3-opus-20240229",
          ].map((model) => (
            <Tag key={model} size="small" type="light" style={{ fontFamily: "monospace", fontSize: 11 }}>
              {model}
            </Tag>
          ))}
        </div>
      </Card>
    </div>
  );
}
