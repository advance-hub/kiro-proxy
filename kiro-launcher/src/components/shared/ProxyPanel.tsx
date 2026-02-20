import React, { useState, useEffect, useCallback } from "react";
import { Button, Card, Typography, Tag, Toast, Space, Divider, Tooltip, Collapsible } from "@douyinfe/semi-ui";
import { IconPlay, IconStop, IconCopy, IconTick, IconCheckCircleStroked, IconAlertCircle, IconRefresh, IconChevronDown, IconChevronRight } from "@douyinfe/semi-icons";

const { Text } = Typography;

const wails = () => {
  if (!window.go?.main?.App) throw new Error("Wails runtime 尚未就绪");
  return window.go.main.App;
};

interface ProxyConfig { host: string; port: number; apiKey: string; region: string; }
interface StatusInfo { running: boolean; has_credentials: boolean; config: ProxyConfig; }

interface ProxyPanelProps {
  onNavigate?: (tab: string, subTab?: string) => void;
}

export default function ProxyPanel({ onNavigate }: ProxyPanelProps) {
  const [status, setStatus] = useState<StatusInfo | null>(null);
  const [backend, setBackend] = useState("kiro");
  const [loading, setLoading] = useState(false);
  const [copied, setCopied] = useState("");
  const [expanded, setExpanded] = useState<Record<string, boolean>>({ kiro: true, warp: true, codex: true, claudecode: true });
  // 各后端凭证统计
  const [kiroCount, setKiroCount] = useState(0);
  const [warpStats, setWarpStats] = useState<{ total: number; active: number } | null>(null);
  const [codexStats, setCodexStats] = useState<{ total: number; active: number } | null>(null);
  const [claudeCodeReady, setClaudeCodeReady] = useState(false);
  // 各后端模型数据
  const [kiroModels, setKiroModels] = useState<any[]>([]);
  const [warpModels, setWarpModels] = useState<any[]>([]);
  const [codexModels, setCodexModels] = useState<any[]>([]);
  const [claudeCodeModels, setClaudeCodeModels] = useState<any[]>([]);

  const refreshStatus = useCallback(async () => {
    try { const s = await wails().GetStatus() as StatusInfo; setStatus(s); } catch (_) {}
    try { const b = await wails().GetBackend(); setBackend(b || "kiro"); } catch (_) {}
  }, []);

  const getBase = useCallback(() => {
    const h = status?.config?.host || "127.0.0.1";
    const p = status?.config?.port || 13000;
    return `http://${h}:${p}`;
  }, [status]);

  const getApiKey = useCallback(() => status?.config?.apiKey || "kiro-proxy-123", [status]);

  const fetchBackendStats = useCallback(async () => {
    const base = getBase();
    const headers = { "x-api-key": getApiKey() };
    const authHeaders = { ...headers, Authorization: `Bearer ${getApiKey()}` };
    
    // 获取凭证统计
    try { const r = await fetch(`${base}/api/admin/user-credentials/stats`, { headers }); if (r.ok) { const d = await r.json(); setKiroCount(d.total_users || 0); } } catch (_) {}
    try { const r = await fetch(`${base}/api/warp/stats`, { headers }); if (r.ok) { const d = await r.json(); setWarpStats(d.data || null); } } catch (_) {}
    try { const r = await fetch(`${base}/api/codex/stats`, { headers }); if (r.ok) { const d = await r.json(); setCodexStats(d.data || null); } } catch (_) {}
    try { const r = await fetch(`${base}/claudecode/v1/models`, { headers: authHeaders }); setClaudeCodeReady(r.ok); } catch (_) { setClaudeCodeReady(false); }
    
    // 获取模型数据
    try { const r = await fetch(`${base}/kiro/v1/models`, { headers: authHeaders }); if (r.ok) { const d = await r.json(); setKiroModels(d.data || []); } } catch (_) { setKiroModels([]); }
    try { const r = await fetch(`${base}/warp/v1/models`, { headers: authHeaders }); if (r.ok) { const d = await r.json(); setWarpModels(d.data || []); } } catch (_) { setWarpModels([]); }
    try { const r = await fetch(`${base}/codex/v1/models`, { headers: authHeaders }); if (r.ok) { const d = await r.json(); setCodexModels(d.data || []); } } catch (_) { setCodexModels([]); }
    try { const r = await fetch(`${base}/claudecode/v1/models`, { headers: authHeaders }); if (r.ok) { const d = await r.json(); setClaudeCodeModels(d.data || []); } } catch (_) { setClaudeCodeModels([]); }
  }, [getBase, getApiKey]);

  useEffect(() => { refreshStatus(); const t = setInterval(refreshStatus, 3000); return () => clearInterval(t); }, [refreshStatus]);
  useEffect(() => { if (status?.running) fetchBackendStats(); }, [status?.running, fetchBackendStats]);

  const handleStart = async () => {
    setLoading(true);
    try { const r = await wails().OneClickStart(); Toast.success({ content: r }); await refreshStatus(); }
    catch (e) { Toast.error({ content: String(e) }); }
    finally { setLoading(false); }
  };

  const handleStop = async () => {
    try { const r = await wails().StopProxy(); Toast.success({ content: r }); await refreshStatus(); }
    catch (e) { Toast.error({ content: String(e) }); }
  };

  const copyText = (text: string, id: string) => {
    navigator.clipboard.writeText(text);
    setCopied(id);
    setTimeout(() => setCopied(""), 2000);
  };

  const toggle = (key: string) => setExpanded(prev => ({ ...prev, [key]: !prev[key] }));

  const running = status?.running ?? false;
  const host = status?.config?.host || "127.0.0.1";
  const port = status?.config?.port || 13000;
  const apiKey = status?.config?.apiKey || "kiro-proxy-123";
  const region = status?.config?.region || "us-east-1";
  const baseUrl = `http://${host}:${port}`;

  // 端点渲染
  const renderEndpoints = (prefix: string) => (
    <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
      {[
        { method: "POST", path: `${prefix}/messages`, desc: "Anthropic 格式" },
        { method: "POST", path: `${prefix}/chat/completions`, desc: "OpenAI 格式" },
        { method: "GET", path: `${prefix}/models`, desc: "模型列表" },
      ].map((ep) => (
        <div key={ep.path} style={{ display: "flex", alignItems: "center", gap: 8, padding: "6px 10px", borderRadius: 6, background: "var(--semi-color-fill-0)", border: "1px solid var(--semi-color-border)" }}>
          <Tag size="small" color={ep.method === "GET" ? "green" : "blue"} type="light" style={{ fontFamily: "monospace", fontSize: 11, minWidth: 42, textAlign: "center" }}>{ep.method}</Tag>
          <Text size="small" copyable style={{ fontFamily: "monospace", fontSize: 11, flex: 1 }}>{baseUrl}{ep.path}</Text>
          <Text type="tertiary" size="small">{ep.desc}</Text>
        </div>
      ))}
    </div>
  );

  // 后端折叠卡片
  const renderBackendCard = (
    key: string,
    name: string,
    desc: string,
    color: "blue" | "purple" | "green" | "orange",
    prefix: string,
    statusText: string,
    statusOk: boolean,
    models: any[],
  ) => (
    <Card bodyStyle={{ padding: 0 }} style={{ marginBottom: 12, borderRadius: 10 }}>
      <div style={{ padding: "12px 20px", cursor: "pointer", userSelect: "none", display: "flex", alignItems: "center", justifyContent: "space-between" }} onClick={() => toggle(key)}>
        <Space>
          <Tag color={color} size="small">{name}</Tag>
          <Text strong size="small">{desc}</Text>
        </Space>
        <Space>
          <Tag color={statusOk ? "green" : "grey"} size="small" type="light">{statusText}</Tag>
          {models.length > 0 && <Tag color="blue" size="small" type="light">{models.length} 个模型</Tag>}
          {expanded[key] ? <IconChevronDown size="small" /> : <IconChevronRight size="small" />}
        </Space>
      </div>
      <Collapsible isOpen={expanded[key]}>
        <Divider style={{ margin: 0 }} />
        <div style={{ padding: "14px 20px" }}>
          <Text strong size="small" style={{ display: "block", marginBottom: 8 }}>API 端点</Text>
          {renderEndpoints(prefix)}
          
          {models.length > 0 && (
            <>
              <Divider style={{ margin: "12px 0" }} />
              <Text strong size="small" style={{ display: "block", marginBottom: 8 }}>可用模型 ({models.length})</Text>
              <div style={{ overflowX: "auto" }}>
                <table style={{ width: "100%", fontSize: 11, borderCollapse: "collapse", minWidth: 1200 }}>
                  <thead>
                    <tr style={{ background: "var(--semi-color-fill-0)", borderBottom: "2px solid var(--semi-color-border)" }}>
                      <th style={{ padding: "6px 8px", textAlign: "left", fontWeight: 600 }}>模型 ID</th>
                      <th style={{ padding: "6px 8px", textAlign: "left", fontWeight: 600 }}>名称</th>
                      <th style={{ padding: "6px 8px", textAlign: "left", fontWeight: 600, minWidth: 180 }}>描述</th>
                      <th style={{ padding: "6px 8px", textAlign: "center", fontWeight: 600 }}>Max Input</th>
                      <th style={{ padding: "6px 8px", textAlign: "center", fontWeight: 600 }}>Max Output</th>
                      <th style={{ padding: "6px 8px", textAlign: "center", fontWeight: 600 }}>提供商</th>
                      <th style={{ padding: "6px 8px", textAlign: "center", fontWeight: 600 }}>倍率</th>
                      <th style={{ padding: "6px 8px", textAlign: "center", fontWeight: 600 }}>单位</th>
                      <th style={{ padding: "6px 8px", textAlign: "center", fontWeight: 600 }}>输入类型</th>
                      <th style={{ padding: "6px 8px", textAlign: "center", fontWeight: 600 }}>Prompt Cache</th>
                      <th style={{ padding: "6px 8px", textAlign: "center", fontWeight: 600 }}>Cache Min</th>
                    </tr>
                  </thead>
                  <tbody>
                    {models.map((model: any, idx: number) => (
                      <tr key={idx} style={{ borderBottom: "1px solid var(--semi-color-border)" }}>
                        <td style={{ padding: "6px 8px" }}>
                          <code style={{ fontSize: 10, background: "var(--semi-color-fill-0)", padding: "2px 6px", borderRadius: 3, color: "#0891b2" }}>
                            {model.id || model.modelId}
                          </code>
                        </td>
                        <td style={{ padding: "6px 8px", fontWeight: 500, whiteSpace: "nowrap" }}>{model.display_name || model.modelName}</td>
                        <td style={{ padding: "6px 8px", color: "var(--semi-color-text-2)", maxWidth: 200, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                          {model.description || "-"}
                        </td>
                        <td style={{ padding: "6px 8px", textAlign: "center", whiteSpace: "nowrap" }}>
                          {model.max_tokens?.toLocaleString() || model.tokenLimits?.maxInputTokens?.toLocaleString() || "-"}
                        </td>
                        <td style={{ padding: "6px 8px", textAlign: "center", whiteSpace: "nowrap" }}>
                          {model.tokenLimits?.maxOutputTokens?.toLocaleString() || "-"}
                        </td>
                        <td style={{ padding: "6px 8px", textAlign: "center" }}>
                          <Tag size="small" color="purple" type="light">{model.owned_by || model.provider || "-"}</Tag>
                        </td>
                        <td style={{ padding: "6px 8px", textAlign: "center" }}>
                          {model.rateMultiplier ? <Tag size="small" color="orange" type="light">×{model.rateMultiplier}</Tag> : "-"}
                        </td>
                        <td style={{ padding: "6px 8px", textAlign: "center" }}>
                          {model.rateUnit ? <Tag size="small" type="light">{model.rateUnit}</Tag> : "-"}
                        </td>
                        <td style={{ padding: "6px 8px", textAlign: "center" }}>
                          {model.supportedInputTypes ? (
                            <div style={{ display: "flex", gap: 2, justifyContent: "center", flexWrap: "nowrap" }}>
                              {model.supportedInputTypes.map((t: string) => (
                                <Tag key={t} size="small" color="green" type="light" style={{ fontSize: 9 }}>{t}</Tag>
                              ))}
                            </div>
                          ) : "-"}
                        </td>
                        <td style={{ padding: "6px 8px", textAlign: "center" }}>
                          {model.promptCaching?.supportsPromptCaching ? (
                            <Tag size="small" color="cyan" type="light">✓ {model.promptCaching.maximumCacheCheckpointsPerRequest || 0}</Tag>
                          ) : "-"}
                        </td>
                        <td style={{ padding: "6px 8px", textAlign: "center", whiteSpace: "nowrap" }}>
                          {model.promptCaching?.minimumTokensPerCacheCheckpoint?.toLocaleString() || "-"}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </>
          )}
          
          <Divider style={{ margin: "12px 0" }} />
          <Text
            type="tertiary"
            size="small"
            style={{ cursor: "pointer" }}
            onClick={(e: React.MouseEvent) => { e.stopPropagation(); onNavigate?.("config", key); }}
          >
            前往 <Text link={{ onClick: () => onNavigate?.("config", key) }} size="small">{name} 配置</Text> 管理凭证
          </Text>
        </div>
      </Collapsible>
    </Card>
  );

  return (
    <div style={{ padding: "20px 24px 32px" }}>
      {/* 页面标题 + 启停 */}
      <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: 20 }}>
        <div>
          <Text strong style={{ fontSize: 18, display: "block" }}>代理服务</Text>
          <Text type="tertiary" size="small">所有后端并行挂载，按模型名自动路由</Text>
        </div>
        <Space>
          {running && <Button size="small" theme="borderless" icon={<IconRefresh />} onClick={fetchBackendStats}>刷新状态</Button>}
          {running ? (
            <Button type="danger" size="large" icon={<IconStop />} onClick={handleStop}>停止服务</Button>
          ) : (
            <Button type="primary" theme="solid" size="large" icon={<IconPlay />} loading={loading} onClick={handleStart}>启动服务</Button>
          )}
        </Space>
      </div>

      {/* 服务概览 */}
      <Card bodyStyle={{ padding: "14px 20px" }} style={{ marginBottom: 12, borderRadius: 10 }}>
        <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
          {running ? <IconCheckCircleStroked style={{ color: "#52c41a", fontSize: 20 }} /> : <IconAlertCircle style={{ color: "#d9d9d9", fontSize: 20 }} />}
          <div style={{ flex: 1 }}>
            <Text strong size="small">{running ? "服务运行中" : "服务已停止"}</Text>
            <Text type="tertiary" size="small" style={{ display: "block", marginTop: 2 }}>
              {running ? `${baseUrl} | Region: ${region}` : "点击启动按钮开始服务"}
            </Text>
          </div>
        </div>
      </Card>

      {/* 统一端点 + 环境变量 */}
      <Card bodyStyle={{ padding: "14px 20px" }} style={{ marginBottom: 12, borderRadius: 10 }}>
        <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: 8 }}>
          <Text strong size="small">统一端点（按模型名自动路由）</Text>
          <Tooltip content={copied === "env" ? "已复制" : "复制环境变量"}>
            <Button size="small" theme="borderless" icon={copied === "env" ? <IconTick style={{ color: "#52c41a" }} /> : <IconCopy />}
              onClick={() => copyText(`ANTHROPIC_BASE_URL=${baseUrl}\nANTHROPIC_API_KEY=${apiKey}`, "env")} />
          </Tooltip>
        </div>
        {renderEndpoints("/v1")}
        <Divider style={{ margin: "10px 0 8px" }} />
        <pre style={{ margin: 0, padding: "8px 12px", borderRadius: 6, background: "var(--semi-color-fill-0)", border: "1px solid var(--semi-color-border)", fontSize: 11, fontFamily: "monospace", lineHeight: 1.5, color: "var(--semi-color-text-0)" }}>
ANTHROPIC_BASE_URL={baseUrl}{"\n"}ANTHROPIC_API_KEY={apiKey}
        </pre>
      </Card>

      {/* 各后端折叠卡片 */}
      {renderBackendCard("kiro", "Kiro", "AWS CodeWhisperer 后端", "blue", "/kiro/v1",
        kiroCount > 0 ? `${kiroCount} 个用户凭证` : "未配置凭证", kiroCount > 0, kiroModels)}

      {renderBackendCard("warp", "Warp", "Cloudflare Warp 多模型代理", "purple", "/warp/v1",
        warpStats ? `${warpStats.active}/${warpStats.total} 活跃` : "未配置凭证", !!(warpStats && warpStats.active > 0), warpModels)}

      {renderBackendCard("codex", "Codex", "OpenAI Codex 后端", "green", "/codex/v1",
        codexStats ? `${codexStats.active}/${codexStats.total} 活跃` : "未配置凭证", !!(codexStats && codexStats.active > 0), codexModels)}

      {renderBackendCard("claudecode", "Claude Code", "Claude Code 反向代理", "orange", "/claudecode/v1",
        claudeCodeReady ? "API Key 已配置" : "需要配置 API Key", claudeCodeReady, claudeCodeModels)}
    </div>
  );
}
