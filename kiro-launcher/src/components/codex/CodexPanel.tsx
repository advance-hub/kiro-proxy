import React, { useState, useEffect, useCallback } from "react";
import { Button, Card, Input, Typography, Tag, Toast, Space, Table, TextArea } from "@douyinfe/semi-ui";
import { IconPlus, IconRefresh, IconUpload } from "@douyinfe/semi-icons";

const { Text } = Typography;

const wails = () => {
  if (!window.go?.main?.App) throw new Error("Wails runtime 尚未就绪");
  return window.go.main.App;
};

interface CodexCredential {
  id: number;
  name: string;
  email: string;
  disabled: boolean;
  useCount: number;
  errorCount: number;
  lastError: string;
}

interface CodexStats {
  total: number;
  active: number;
}

export default function CodexPanel() {
  const [credentials, setCredentials] = useState<CodexCredential[]>([]);
  const [stats, setStats] = useState<CodexStats | null>(null);
  const [loading, setLoading] = useState(false);
  const [addToken, setAddToken] = useState("");
  const [addName, setAddName] = useState("");
  const [adding, setAdding] = useState(false);
  const [showBatchImport, setShowBatchImport] = useState(false);
  const [batchText, setBatchText] = useState("");
  const [batchImporting, setBatchImporting] = useState(false);
  const [refreshing, setRefreshing] = useState(false);
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

  const baseUrl = useCallback(() => `http://${proxyHost}:${proxyPort}`, [proxyHost, proxyPort]);

  const fetchCredentials = useCallback(async () => {
    setLoading(true);
    try {
      const resp = await fetch(`${baseUrl()}/api/codex/credentials`, { headers: { "x-api-key": proxyApiKey } });
      if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
      const data = await resp.json();
      setCredentials(data.data || []);
    } catch (e) {
      Toast.warning({ content: `获取凭证失败: ${e}` });
    } finally {
      setLoading(false);
    }
  }, [baseUrl, proxyApiKey]);

  const fetchStats = useCallback(async () => {
    try {
      const resp = await fetch(`${baseUrl()}/api/codex/stats`, { headers: { "x-api-key": proxyApiKey } });
      if (!resp.ok) return;
      const data = await resp.json();
      setStats(data.data || null);
    } catch (_) {}
  }, [baseUrl, proxyApiKey]);

  useEffect(() => { loadProxyConfig(); }, [loadProxyConfig]);
  useEffect(() => {
    if (proxyHost && proxyPort) { fetchCredentials(); fetchStats(); }
  }, [proxyHost, proxyPort, fetchCredentials, fetchStats]);

  const handleAdd = async () => {
    if (!addToken.trim()) { Toast.warning({ content: "请输入 Session Token" }); return; }
    setAdding(true);
    try {
      const resp = await fetch(`${baseUrl()}/api/codex/credentials`, {
        method: "POST",
        headers: { "Content-Type": "application/json", "x-api-key": proxyApiKey },
        body: JSON.stringify({ name: addName.trim() || undefined, sessionToken: addToken.trim() }),
      });
      const data = await resp.json();
      if (!resp.ok) throw new Error(data.error || `HTTP ${resp.status}`);
      Toast.success({ content: `添加成功: ${data.data?.name || ""}` });
      setAddToken(""); setAddName("");
      fetchCredentials(); fetchStats();
    } catch (e) { Toast.error({ content: `添加失败: ${e}` }); }
    finally { setAdding(false); }
  };

  const handleBatchImport = async () => {
    if (!batchText.trim()) return;
    setBatchImporting(true);
    try {
      let accounts: { name?: string; sessionToken: string }[] = [];
      try {
        const parsed = JSON.parse(batchText.trim());
        if (Array.isArray(parsed)) {
          accounts = parsed.map((item: any) => ({
            name: item.name || item.email,
            sessionToken: item.sessionToken || item.session_token || item.token,
          }));
        }
      } catch (_) {
        accounts = batchText.trim().split("\n").filter(Boolean).map((line) => ({ sessionToken: line.trim() }));
      }
      const resp = await fetch(`${baseUrl()}/api/codex/credentials/batch-import`, {
        method: "POST",
        headers: { "Content-Type": "application/json", "x-api-key": proxyApiKey },
        body: JSON.stringify({ accounts }),
      });
      const data = await resp.json();
      if (!resp.ok) throw new Error(data.error || `HTTP ${resp.status}`);
      Toast.success({ content: `导入完成: 成功 ${data.data?.success || 0}, 失败 ${data.data?.failed || 0}` });
      setBatchText(""); setShowBatchImport(false);
      fetchCredentials(); fetchStats();
    } catch (e) { Toast.error({ content: `导入失败: ${e}` }); }
    finally { setBatchImporting(false); }
  };

  const handleRefreshAll = async () => {
    setRefreshing(true);
    try {
      const resp = await fetch(`${baseUrl()}/api/codex/refresh-all`, { method: "POST", headers: { "x-api-key": proxyApiKey } });
      const data = await resp.json();
      if (!resp.ok) throw new Error(data.error || `HTTP ${resp.status}`);
      Toast.success({ content: `刷新完成: 成功 ${data.data?.success || 0}, 失败 ${data.data?.failed || 0}` });
      fetchCredentials();
    } catch (e) { Toast.error({ content: `刷新失败: ${e}` }); }
    finally { setRefreshing(false); }
  };

  const columns = [
    { title: "ID", dataIndex: "id", width: 50 },
    { title: "名称", dataIndex: "name", width: 150 },
    { title: "邮箱", dataIndex: "email", width: 200 },
    {
      title: "状态", dataIndex: "disabled", width: 80,
      render: (disabled: boolean) => disabled
        ? <Tag color="red" size="small" type="light">禁用</Tag>
        : <Tag color="green" size="small" type="light">活跃</Tag>,
    },
    { title: "使用次数", dataIndex: "useCount", width: 80 },
    {
      title: "错误", dataIndex: "errorCount", width: 60,
      render: (count: number) => count > 0
        ? <Tag color="red" size="small" type="light">{count}</Tag>
        : <Text type="tertiary" size="small">0</Text>,
    },
  ];

  return (
    <div style={{ padding: "20px 24px 32px" }}>
      <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: 20 }}>
        <div>
          <Text strong style={{ fontSize: 18, display: "block" }}>Codex 模式</Text>
          <Text type="tertiary" size="small">通过 OpenAI Codex 代理访问 GPT 模型</Text>
        </div>
        {stats && (
          <Space>
            <Tag color="blue" size="large" type="light">总计 {stats.total}</Tag>
            <Tag color="green" size="large" type="light">活跃 {stats.active}</Tag>
          </Space>
        )}
      </div>

      {/* API 端点 */}
      <Card bodyStyle={{ padding: "14px 20px" }} style={{ marginBottom: 12, borderRadius: 10 }}>
        <Text strong size="small" style={{ display: "block", marginBottom: 8 }}>Codex API 端点</Text>
        <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
          {[
            { method: "POST", path: "/codex/v1/messages", desc: "Anthropic 格式" },
            { method: "POST", path: "/codex/v1/chat/completions", desc: "OpenAI 格式" },
            { method: "GET", path: "/codex/v1/models", desc: "模型列表" },
          ].map((ep) => (
            <div key={ep.path + ep.desc} style={{ display: "flex", alignItems: "center", gap: 8, padding: "6px 10px", borderRadius: 6, background: "var(--semi-color-fill-0)", border: "1px solid var(--semi-color-border)" }}>
              <Tag size="small" color={ep.method === "GET" ? "green" : "blue"} type="light" style={{ fontFamily: "monospace", fontSize: 11, minWidth: 42, textAlign: "center" }}>{ep.method}</Tag>
              <Text size="small" copyable style={{ fontFamily: "monospace", fontSize: 11, flex: 1 }}>{`http://${proxyHost}:${proxyPort}${ep.path}`}</Text>
              <Text type="tertiary" size="small">{ep.desc}</Text>
            </div>
          ))}
        </div>
      </Card>

      {/* 添加凭证 */}
      <Card bodyStyle={{ padding: "14px 20px" }} style={{ marginBottom: 12, borderRadius: 10 }}>
        <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: 10 }}>
          <Text strong size="small">添加 Codex 凭证</Text>
          <Button size="small" theme="borderless" icon={<IconUpload />} onClick={() => setShowBatchImport(!showBatchImport)}>批量导入</Button>
        </div>
        <div style={{ display: "flex", gap: 8, marginBottom: 8 }}>
          <Input size="small" placeholder="名称 (可选)" value={addName} onChange={setAddName} style={{ width: 120 }} />
          <Input size="small" placeholder="ChatGPT Session Token" value={addToken} onChange={setAddToken} style={{ flex: 1 }} />
          <Button size="small" theme="solid" type="primary" icon={<IconPlus />} loading={adding} onClick={handleAdd}>添加</Button>
        </div>
        {showBatchImport && (
          <div style={{ marginTop: 8 }}>
            <TextArea placeholder={'批量导入：每行一个 session token，或 JSON 数组\n[{"sessionToken":"xxx","name":"test"}]'} value={batchText} onChange={setBatchText} rows={4} style={{ fontSize: 12 }} />
            <Button size="small" theme="light" type="primary" loading={batchImporting} onClick={handleBatchImport} style={{ marginTop: 8 }}>导入</Button>
          </div>
        )}
      </Card>

      {/* 凭证列表 */}
      <Card bodyStyle={{ padding: "14px 20px" }} style={{ marginBottom: 12, borderRadius: 10 }}>
        <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: 10 }}>
          <Text strong size="small">凭证列表</Text>
          <Space>
            <Button size="small" theme="borderless" icon={<IconRefresh />} loading={refreshing} onClick={handleRefreshAll}>刷新全部 Token</Button>
            <Button size="small" theme="borderless" icon={<IconRefresh />} loading={loading} onClick={fetchCredentials}>刷新列表</Button>
          </Space>
        </div>
        <Table columns={columns} dataSource={credentials} pagination={false} size="small" rowKey="id"
          empty={<Text type="tertiary" size="small">暂无凭证，请添加 Codex 账号</Text>} />
      </Card>
    </div>
  );
}
