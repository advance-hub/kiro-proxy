import React, { useState, useEffect, useCallback } from "react";
import { Button, Card, Typography, Tag, Switch, Space } from "@douyinfe/semi-ui";
import { IconRefresh, IconCopy, IconFolder } from "@douyinfe/semi-icons";

const { Text } = Typography;

const wails = () => {
  if (!window.go?.main?.App) throw new Error("Wails runtime 尚未就绪");
  return window.go.main.App;
};

const stripAnsi = (s: string) => s.replace(/\x1b\[[0-9;]*m/g, "");

function colorForLevel(line: string): string | undefined {
  if (line.includes("[ERROR]")) return "#f5222d";
  if (line.includes("[WARN]")) return "#fa8c16";
  if (line.includes("✅")) return "#52c41a";
  return undefined;
}

export default function LogsPanel() {
  const [tab, setTab] = useState<"proxy" | "system">("proxy");
  const [proxyLogs, setProxyLogs] = useState<string[]>([]);
  const [systemLogs, setSystemLogs] = useState<string[]>([]);
  const [logFilePath, setLogFilePath] = useState("");
  const [autoScroll, setAutoScroll] = useState(true);

  const logsEndRef = useCallback((node: HTMLDivElement | null) => {
    if (node && autoScroll) node.scrollIntoView({ behavior: "smooth" });
  }, [autoScroll, proxyLogs, systemLogs, tab]);

  // 获取日志文件路径
  useEffect(() => {
    wails().GetLogFilePath().then(setLogFilePath).catch(() => {});
  }, []);

  // 代理日志轮询
  useEffect(() => {
    if (tab !== "proxy") return;
    const fetch = async () => {
      try {
        const l = await wails().GetProxyLogs();
        setProxyLogs((l || []).map(stripAnsi));
      } catch (_) {}
    };
    fetch();
    const t = setInterval(fetch, 1000);
    return () => clearInterval(t);
  }, [tab]);

  // 系统日志轮询
  useEffect(() => {
    if (tab !== "system") return;
    const fetch = async () => {
      try {
        const text = await wails().GetRecentLogs(200);
        setSystemLogs(text ? text.split("\n").filter((l: string) => l.trim()) : []);
      } catch (_) {}
    };
    fetch();
    const t = setInterval(fetch, 2000);
    return () => clearInterval(t);
  }, [tab]);

  const currentLogs = tab === "proxy" ? proxyLogs : systemLogs;

  const tabStyle = (active: boolean): React.CSSProperties => ({
    padding: "8px 16px", cursor: "pointer", fontSize: 13,
    fontWeight: active ? 600 : 400,
    color: active ? "#3370ff" : "var(--semi-color-text-2)",
    borderBottom: active ? "2px solid #3370ff" : "2px solid transparent",
    transition: "all 0.15s",
  });

  return (
    <div style={{ padding: "20px 24px 32px" }}>
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: 16 }}>
        <div>
          <Text strong style={{ fontSize: 18, display: "block" }}>日志中心</Text>
          <Text type="tertiary" size="small">查看代理请求日志和系统运行日志</Text>
        </div>
        <Space>
          <Text size="small" type="tertiary">自动滚动</Text>
          <Switch size="small" checked={autoScroll} onChange={setAutoScroll} />
          <Button size="small" theme="borderless" icon={<IconRefresh />} onClick={async () => {
            if (tab === "proxy") {
              const l = await wails().GetProxyLogs();
              setProxyLogs((l || []).map(stripAnsi));
            } else {
              const text = await wails().GetRecentLogs(200);
              setSystemLogs(text ? text.split("\n").filter((l: string) => l.trim()) : []);
            }
          }}>刷新</Button>
        </Space>
      </div>

      {/* Tab 切换 */}
      <div style={{ display: "flex", gap: 0, borderBottom: "1px solid var(--semi-color-border)", marginBottom: 12 }}>
        <div style={tabStyle(tab === "proxy")} onClick={() => setTab("proxy")}>
          代理日志 {proxyLogs.length > 0 && <Tag size="small" color="blue" type="light" style={{ marginLeft: 4 }}>{proxyLogs.length}</Tag>}
        </div>
        <div style={tabStyle(tab === "system")} onClick={() => setTab("system")}>
          系统日志 {systemLogs.length > 0 && <Tag size="small" color="green" type="light" style={{ marginLeft: 4 }}>{systemLogs.length}</Tag>}
        </div>
      </div>

      {/* 日志文件路径 */}
      {tab === "system" && logFilePath && (
        <div style={{ marginBottom: 10, display: "flex", alignItems: "center", gap: 6 }}>
          <IconFolder size="small" style={{ color: "var(--semi-color-text-2)" }} />
          <Text size="small" type="tertiary" style={{ fontFamily: "monospace", fontSize: 11 }}>{logFilePath}</Text>
          <Button size="small" theme="borderless" icon={<IconCopy size="small" />} style={{ padding: "0 4px", height: 20 }}
            onClick={() => { navigator.clipboard.writeText(logFilePath); }}
          />
        </div>
      )}

      <Card bodyStyle={{ padding: 0 }} style={{ borderRadius: 10 }}>
        <div style={{
          height: 520, overflowY: "auto", padding: "10px 14px",
          fontFamily: "'SF Mono', 'Fira Code', monospace", fontSize: 11, lineHeight: 1.7,
          color: "var(--semi-color-text-1)", background: "var(--semi-color-fill-0)", borderRadius: 10,
        }}>
          {currentLogs.length === 0 ? (
            <Text type="tertiary" size="small">
              {tab === "proxy" ? "暂无日志，启动代理后将在此显示请求日志" : "暂无系统日志"}
            </Text>
          ) : (
            currentLogs.map((line, i) => (
              <div key={i} style={{
                padding: "2px 0", borderBottom: "1px solid var(--semi-color-border)",
                wordBreak: "break-all", whiteSpace: "pre-wrap",
                color: tab === "system" ? colorForLevel(line) : undefined,
              }}>{line}</div>
            ))
          )}
          <div ref={logsEndRef} />
        </div>
      </Card>
    </div>
  );
}
