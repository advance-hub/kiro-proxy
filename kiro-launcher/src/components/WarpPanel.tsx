import React, { useState, useEffect, useCallback } from "react";
import { Button, Card, Input, Typography, Tag, Toast, Space, Divider, Table, Popconfirm, TextArea } from "@douyinfe/semi-ui";
import { IconPlus, IconRefresh, IconDelete, IconUpload, IconCheckCircleStroked, IconAlertCircle } from "@douyinfe/semi-icons";

const { Text } = Typography;

const wails = () => {
  if (!window.go?.main?.App) throw new Error("Wails runtime 尚未就绪");
  return window.go.main.App;
};

interface WarpCredential {
  id: number;
  name: string;
  email: string;
  disabled: boolean;
  useCount: number;
  errorCount: number;
  lastError: string;
}

interface WarpStats {
  total: number;
  active: number;
}

interface WarpQuota {
  id: number;
  name: string;
  email: string;
  error?: string;
  quota?: {
    requestLimit: number;
    requestsUsed: number;
    remaining: number;
    isUnlimited: boolean;
    nextRefreshTime?: string;
    refreshDuration?: string;
  };
}

export default function WarpPanel() {
  const [credentials, setCredentials] = useState<WarpCredential[]>([]);
  const [stats, setStats] = useState<WarpStats | null>(null);
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
  const [quotas, setQuotas] = useState<WarpQuota[]>([]);
  const [quotasLoading, setQuotasLoading] = useState(false);

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
      const resp = await fetch(`${baseUrl()}/api/warp/credentials`, { headers: { "x-api-key": proxyApiKey } });
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
      const resp = await fetch(`${baseUrl()}/api/warp/stats`, { headers: { "x-api-key": proxyApiKey } });
      if (!resp.ok) return;
      const data = await resp.json();
      setStats(data.data || null);
    } catch (_) {}
  }, [baseUrl, proxyApiKey]);

  const fetchQuotas = useCallback(async () => {
    setQuotasLoading(true);
    try {
      const resp = await fetch(`${baseUrl()}/api/warp/quotas`, { headers: { "x-api-key": proxyApiKey } });
      if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
      const data = await resp.json();
      setQuotas(data.data || []);
    } catch (e) {
      Toast.warning({ content: `获取配额失败: ${e}` });
    } finally {
      setQuotasLoading(false);
    }
  }, [baseUrl, proxyApiKey]);

  useEffect(() => {
    loadProxyConfig();
  }, [loadProxyConfig]);

  useEffect(() => {
    if (proxyHost && proxyPort) {
      fetchCredentials();
      fetchStats();
    }
  }, [proxyHost, proxyPort, fetchCredentials, fetchStats]);

  const handleAdd = async () => {
    if (!addToken.trim()) {
      Toast.warning({ content: "请输入 Refresh Token" });
      return;
    }
    setAdding(true);
    try {
      const resp = await fetch(`${baseUrl()}/api/warp/credentials`, {
        method: "POST",
        headers: { "Content-Type": "application/json", "x-api-key": proxyApiKey },
        body: JSON.stringify({ name: addName.trim() || undefined, refreshToken: addToken.trim() }),
      });
      const data = await resp.json();
      if (!resp.ok) throw new Error(data.error || `HTTP ${resp.status}`);
      Toast.success({ content: `添加成功: ${data.data?.name || ""}` });
      setAddToken("");
      setAddName("");
      fetchCredentials();
      fetchStats();
    } catch (e) {
      Toast.error({ content: `添加失败: ${e}` });
    } finally {
      setAdding(false);
    }
  };

  const handleBatchImport = async () => {
    if (!batchText.trim()) return;
    setBatchImporting(true);
    try {
      let accounts: { name?: string; refreshToken: string }[] = [];
      try {
        const parsed = JSON.parse(batchText.trim());
        if (Array.isArray(parsed)) {
          accounts = parsed.map((item: any) => ({
            name: item.name || item.email,
            refreshToken: item.refreshToken || item.refresh_token || item.token,
          }));
        }
      } catch (_) {
        accounts = batchText.trim().split("\n").filter(Boolean).map((line) => ({ refreshToken: line.trim() }));
      }
      const resp = await fetch(`${baseUrl()}/api/warp/credentials/batch-import`, {
        method: "POST",
        headers: { "Content-Type": "application/json", "x-api-key": proxyApiKey },
        body: JSON.stringify({ accounts }),
      });
      const data = await resp.json();
      if (!resp.ok) throw new Error(data.error || `HTTP ${resp.status}`);
      Toast.success({ content: `导入完成: 成功 ${data.data?.success || 0}, 失败 ${data.data?.failed || 0}` });
      setBatchText("");
      setShowBatchImport(false);
      fetchCredentials();
      fetchStats();
    } catch (e) {
      Toast.error({ content: `导入失败: ${e}` });
    } finally {
      setBatchImporting(false);
    }
  };

  const handleRefreshAll = async () => {
    setRefreshing(true);
    try {
      const resp = await fetch(`${baseUrl()}/api/warp/refresh-all`, {
        method: "POST",
        headers: { "x-api-key": proxyApiKey },
      });
      const data = await resp.json();
      if (!resp.ok) throw new Error(data.error || `HTTP ${resp.status}`);
      Toast.success({ content: `刷新完成: 成功 ${data.data?.success || 0}, 失败 ${data.data?.failed || 0}` });
      fetchCredentials();
    } catch (e) {
      Toast.error({ content: `刷新失败: ${e}` });
    } finally {
      setRefreshing(false);
    }
  };

  const columns = [
    { title: "ID", dataIndex: "id", width: 50 },
    { title: "名称", dataIndex: "name", width: 150 },
    { title: "邮箱", dataIndex: "email", width: 200 },
    {
      title: "状态",
      dataIndex: "disabled",
      width: 80,
      render: (disabled: boolean) =>
        disabled ? (
          <Tag color="red" size="small" type="light">禁用</Tag>
        ) : (
          <Tag color="green" size="small" type="light">活跃</Tag>
        ),
    },
    { title: "使用次数", dataIndex: "useCount", width: 80 },
    {
      title: "错误",
      dataIndex: "errorCount",
      width: 60,
      render: (count: number) =>
        count > 0 ? <Tag color="red" size="small" type="light">{count}</Tag> : <Text type="tertiary" size="small">0</Text>,
    },
  ];

  return (
    <div style={{ padding: "20px 24px 32px" }}>
      <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: 20 }}>
        <div>
          <Text strong style={{ fontSize: 18, display: "block" }}>Warp 模式</Text>
          <Text type="tertiary" size="small">通过 Warp 代理访问 Claude/GPT/Gemini 模型</Text>
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
        <Text strong size="small" style={{ display: "block", marginBottom: 8 }}>Warp API 端点</Text>
        <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
          {[
            { method: "POST", path: "/w/v1/messages", desc: "Warp Claude 格式" },
            { method: "GET", path: "/w/v1/models", desc: "Warp 模型列表" },
            { method: "POST", path: "/v1/messages", desc: "主端点 (backend=warp 时走 Warp)" },
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
          <Text strong size="small">添加 Warp 凭证</Text>
          <Space>
            <Button size="small" theme="borderless" icon={<IconUpload />} onClick={() => setShowBatchImport(!showBatchImport)}>
              批量导入
            </Button>
          </Space>
        </div>
        <div style={{ display: "flex", gap: 8, marginBottom: 8 }}>
          <Input size="small" placeholder="名称 (可选)" value={addName} onChange={setAddName} style={{ width: 120 }} />
          <Input size="small" placeholder="Warp Refresh Token" value={addToken} onChange={setAddToken} style={{ flex: 1 }} />
          <Button size="small" theme="solid" type="primary" icon={<IconPlus />} loading={adding} onClick={handleAdd}>添加</Button>
        </div>
        {showBatchImport && (
          <div style={{ marginTop: 8 }}>
            <TextArea placeholder={'批量导入：每行一个 refresh token，或 JSON 数组\n[{"refreshToken":"xxx","name":"test"}]'} value={batchText} onChange={setBatchText} rows={4} style={{ fontSize: 12 }} />
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
        <Table
          columns={columns}
          dataSource={credentials}
          pagination={false}
          size="small"
          rowKey="id"
          empty={<Text type="tertiary" size="small">暂无凭证，请添加 Warp 账号</Text>}
        />
      </Card>

      {/* 配额查询 */}
      <Card bodyStyle={{ padding: "14px 20px" }} style={{ marginBottom: 12, borderRadius: 10 }}>
        <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: 10 }}>
          <Text strong size="small">账号配额</Text>
          <Button size="small" theme="borderless" icon={<IconRefresh />} loading={quotasLoading} onClick={fetchQuotas}>查询配额</Button>
        </div>
        {quotas.length > 0 ? (
          <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
            {quotas.map((q) => (
              <div key={q.id} style={{ padding: "8px 12px", borderRadius: 6, background: "var(--semi-color-fill-0)", border: "1px solid var(--semi-color-border)" }}>
                <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: 4 }}>
                  <Text strong size="small">{q.name || q.email || `#${q.id}`}</Text>
                  {q.error ? (
                    <Tag color="red" size="small" type="light">错误</Tag>
                  ) : q.quota?.isUnlimited ? (
                    <Tag color="green" size="small" type="light">无限制</Tag>
                  ) : (
                    <Tag color={q.quota && q.quota.remaining > 10 ? "green" : q.quota && q.quota.remaining > 0 ? "orange" : "red"} size="small" type="light">
                      剩余 {q.quota?.remaining ?? 0}/{q.quota?.requestLimit ?? 0}
                    </Tag>
                  )}
                </div>
                {q.error ? (
                  <Text type="danger" size="small">{q.error}</Text>
                ) : q.quota ? (
                  <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
                    <Text type="tertiary" size="small">已用 {q.quota.requestsUsed}</Text>
                    {q.quota.refreshDuration && <Text type="tertiary" size="small">| 刷新周期: {q.quota.refreshDuration}</Text>}
                    {q.quota.nextRefreshTime && <Text type="tertiary" size="small">| 下次刷新: {new Date(q.quota.nextRefreshTime).toLocaleString()}</Text>}
                  </div>
                ) : null}
              </div>
            ))}
          </div>
        ) : (
          <div style={{ textAlign: "center", padding: "12px 0" }}>
            <Text type="tertiary" size="small">{quotasLoading ? "正在查询配额..." : "点击查询配额查看各账号用量"}</Text>
          </div>
        )}
      </Card>
    </div>
  );
}
