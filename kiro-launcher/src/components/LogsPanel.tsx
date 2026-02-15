import React, { useState, useEffect, useCallback } from "react";
import { Button, Card, Typography, Tag, Switch, Space } from "@douyinfe/semi-ui";
import { IconRefresh } from "@douyinfe/semi-icons";

const { Text } = Typography;

const wails = () => {
  if (!window.go?.main?.App) throw new Error("Wails runtime 尚未就绪");
  return window.go.main.App;
};

const stripAnsi = (s: string) => s.replace(/\x1b\[[0-9;]*m/g, "");

export default function LogsPanel() {
  const [logs, setLogs] = useState<string[]>([]);
  const [autoScroll, setAutoScroll] = useState(true);
  const logsEndRef = useCallback((node: HTMLDivElement | null) => {
    if (node && autoScroll) node.scrollIntoView({ behavior: "smooth" });
  }, [autoScroll, logs]);

  useEffect(() => {
    const fetchLogs = async () => {
      try {
        const l = await wails().GetProxyLogs();
        setLogs((l || []).map(stripAnsi));
      } catch (_) {}
    };
    fetchLogs();
    const t = setInterval(fetchLogs, 1000);
    return () => clearInterval(t);
  }, []);

  return (
    <div style={{ padding: "20px 24px 32px" }}>
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: 20 }}>
        <div>
          <Text strong style={{ fontSize: 18, display: "block" }}>请求日志</Text>
          <Text type="tertiary" size="small">
            实时查看代理请求记录
            {logs.length > 0 && <Tag size="small" color="blue" type="light" style={{ marginLeft: 8 }}>{logs.length}</Tag>}
          </Text>
        </div>
        <Space>
          <Text size="small" type="tertiary">自动滚动</Text>
          <Switch size="small" checked={autoScroll} onChange={setAutoScroll} />
          <Button size="small" theme="borderless" icon={<IconRefresh />} onClick={async () => {
            const l = await wails().GetProxyLogs();
            setLogs((l || []).map(stripAnsi));
          }}>刷新</Button>
        </Space>
      </div>
      <Card bodyStyle={{ padding: 0 }} style={{ borderRadius: 10 }}>
        <div style={{
          height: 480, overflowY: "auto", padding: "10px 14px",
          fontFamily: "'SF Mono', 'Fira Code', monospace", fontSize: 11, lineHeight: 1.7,
          color: "var(--semi-color-text-1)", background: "var(--semi-color-fill-0)", borderRadius: 10,
        }}>
          {logs.length === 0 ? (
            <Text type="tertiary" size="small">暂无日志，启动代理后将在此显示请求日志</Text>
          ) : (
            logs.map((line, i) => (
              <div key={i} style={{ padding: "2px 0", borderBottom: "1px solid var(--semi-color-border)", wordBreak: "break-all", whiteSpace: "pre-wrap" }}>{line}</div>
            ))
          )}
          <div ref={logsEndRef} />
        </div>
      </Card>
    </div>
  );
}
