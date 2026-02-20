import React, { useState, useEffect, useMemo, useRef } from "react";
import { Button, Card, Typography, Tag, Switch, Space, Input, Toast } from "@douyinfe/semi-ui";
import { IconRefresh, IconCopy, IconFolder, IconSearch, IconDelete, IconChevronDown, IconChevronRight } from "@douyinfe/semi-icons";

const { Text } = Typography;

const wails = () => {
  if (!window.go?.main?.App) throw new Error("Wails runtime 尚未就绪");
  return window.go.main.App;
};

const stripAnsi = (s: string) => s.replace(/\x1b\[[0-9;]*m/g, "");

// ── 日志标签定义 ──
interface LogTag { key: string; label: string; color: string; match: (line: string) => boolean; collapsible?: boolean }

const LOG_TAGS: LogTag[] = [
  { key: "error",    label: "ERROR",     color: "#f5222d", match: l => /\[ERROR\]|❌|失败|error/i.test(l) },
  { key: "warn",     label: "WARN",      color: "#fa8c16", match: l => /\[WARN\]|⚠️|警告/i.test(l) },
  { key: "auth",     label: "AUTH",      color: "#722ed1", match: l => l.includes("[AUTH]") },
  { key: "conv",     label: "CONV",      color: "#13c2c2", match: l => /\[CONV\]|\[KIRO_PRE\]|\[KIRO_POST\]/.test(l), collapsible: true },
  { key: "stream",   label: "STREAM",    color: "#1890ff", match: l => /\[STREAM|STREAM_END|STREAM_OUTPUT\]/.test(l) },
  { key: "raw",      label: "RAW",       color: "#8c8c8c", match: l => /\[RAW_REQ\]|\[RAW_KIRO_REQUEST\]/.test(l), collapsible: true },
  { key: "info",     label: "INFO",      color: "#52c41a", match: l => /\[INFO\]|✅|成功/.test(l) },
  { key: "request",  label: "REQUEST",   color: "#3370ff", match: l => /^[\d/: ]*POST |^[\d/: ]*GET /.test(l) },
];

function getLogTag(line: string): LogTag | null {
  for (const tag of LOG_TAGS) {
    if (tag.match(line)) return tag;
  }
  return null;
}

function getLineColor(line: string, isSystem: boolean): string | undefined {
  const tag = getLogTag(line);
  if (tag) return tag.color;
  if (isSystem) {
    if (line.includes("[ERROR]")) return "#f5222d";
    if (line.includes("[WARN]")) return "#fa8c16";
    if (line.includes("✅")) return "#52c41a";
  }
  return undefined;
}

// 任何超过 120 字符的行都可折叠
const COLLAPSE_THRESHOLD = 120;
function isCollapsible(line: string): boolean {
  return line.length > COLLAPSE_THRESHOLD;
}

// ── 日志行组件 ──
function LogLine({ line, index, isSystem }: { line: string; index: number; isSystem: boolean }) {
  const [expanded, setExpanded] = useState(false);
  const [hovered, setHovered] = useState(false);
  const tag = getLogTag(line);
  const color = getLineColor(line, isSystem);
  const canCollapse = isCollapsible(line);
  const displayLine = canCollapse && !expanded ? line.slice(0, COLLAPSE_THRESHOLD) + " …" : line;

  return (
    <div
      style={{
        padding: "3px 8px", borderBottom: "1px solid var(--semi-color-border)",
        wordBreak: "break-all", whiteSpace: "pre-wrap", color,
        display: "flex", alignItems: "flex-start", gap: 6,
        background: hovered ? "var(--semi-color-fill-1)" : index % 2 === 0 ? "transparent" : "var(--semi-color-fill-0)",
        transition: "background 0.1s",
      }}
      onMouseEnter={() => setHovered(true)}
      onMouseLeave={() => setHovered(false)}
    >
      {/* 行号 */}
      <span style={{ color: "var(--semi-color-text-3)", minWidth: 32, textAlign: "right", userSelect: "none", flexShrink: 0, fontSize: 10 }}>
        {index + 1}
      </span>
      {/* 标签 */}
      {tag && (
        <span style={{
          fontSize: 9, padding: "1px 5px", borderRadius: 3, flexShrink: 0,
          background: tag.color + "18", color: tag.color, fontWeight: 600, lineHeight: "16px", marginTop: 1,
        }}>{tag.label}</span>
      )}
      {/* 折叠按钮 */}
      {canCollapse && (
        <span onClick={() => setExpanded(!expanded)} style={{ cursor: "pointer", flexShrink: 0, marginTop: 1, color: "var(--semi-color-text-3)" }}>
          {expanded ? <IconChevronDown size="extra-small" /> : <IconChevronRight size="extra-small" />}
        </span>
      )}
      {/* 内容 */}
      <span style={{ flex: 1, minWidth: 0 }}>{displayLine}</span>
      {/* 复制按钮 */}
      {hovered && (
        <span
          style={{ cursor: "pointer", flexShrink: 0, opacity: 0.5, marginTop: 1 }}
          onClick={() => { navigator.clipboard.writeText(line); Toast.success({ content: "已复制", duration: 1 }); }}
          title="复制此行"
        >
          <IconCopy size="extra-small" />
        </span>
      )}
    </div>
  );
}

// ── 主面板 ──
export default function LogsPanel() {
  const [tab, setTab] = useState<"proxy" | "system">("proxy");
  const [proxyLogs, setProxyLogs] = useState<string[]>([]);
  const [systemLogs, setSystemLogs] = useState<string[]>([]);
  const [logFilePath, setLogFilePath] = useState("");
  const [autoScroll, setAutoScroll] = useState(true);
  const [search, setSearch] = useState("");
  const [activeFilters, setActiveFilters] = useState<Set<string>>(new Set());
  const [showFilters, setShowFilters] = useState(false);
  const scrollRef = useRef<HTMLDivElement>(null);
  const clearUntilRef = useRef<number>(0);

  // 自动滚动
  useEffect(() => {
    if (autoScroll && scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
    }
  }, [proxyLogs, systemLogs, tab, autoScroll]);

  useEffect(() => { wails().GetLogFilePath().then(setLogFilePath).catch(() => {}); }, []);

  // 代理日志轮询
  useEffect(() => {
    if (tab !== "proxy") return;
    const load = async () => { if (Date.now() < clearUntilRef.current) return; try { const l = await wails().GetProxyLogs(); setProxyLogs((l || []).map(stripAnsi)); } catch (_) {} };
    load();
    const t = setInterval(load, 1000);
    return () => clearInterval(t);
  }, [tab]);

  // 系统日志轮询
  useEffect(() => {
    if (tab !== "system") return;
    const load = async () => { if (Date.now() < clearUntilRef.current) return; try { const text = await wails().GetRecentLogs(200); setSystemLogs(text ? text.split("\n").filter((l: string) => l.trim()) : []); } catch (_) {} };
    load();
    const t = setInterval(load, 2000);
    return () => clearInterval(t);
  }, [tab]);

  const currentLogs = tab === "proxy" ? proxyLogs : systemLogs;

  // 过滤日志
  const filteredLogs = useMemo(() => {
    let logs = currentLogs;
    // 标签过滤
    if (activeFilters.size > 0) {
      logs = logs.filter(line => {
        const tag = getLogTag(line);
        return tag ? activeFilters.has(tag.key) : false;
      });
    }
    // 搜索过滤
    if (search.trim()) {
      const q = search.toLowerCase();
      logs = logs.filter(l => l.toLowerCase().includes(q));
    }
    return logs;
  }, [currentLogs, activeFilters, search]);

  // 统计各标签数量
  const tagCounts = useMemo(() => {
    const counts: Record<string, number> = {};
    for (const line of currentLogs) {
      const tag = getLogTag(line);
      if (tag) counts[tag.key] = (counts[tag.key] || 0) + 1;
    }
    return counts;
  }, [currentLogs]);

  const toggleFilter = (key: string) => {
    setActiveFilters(prev => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key); else next.add(key);
      return next;
    });
  };

  const clearLogs = async () => {
    clearUntilRef.current = Date.now() + 3000;
    if (tab === "proxy") {
      try { await wails().ClearProxyLogs(); } catch (_) {}
      setProxyLogs([]);
    } else {
      try { await wails().ClearLogFile(); } catch (_) {}
      setSystemLogs([]);
    }
    Toast.success({ content: "日志已清空", duration: 1 });
  };

  const copyAll = () => {
    navigator.clipboard.writeText(filteredLogs.join("\n"));
    Toast.success({ content: `已复制 ${filteredLogs.length} 条日志`, duration: 1 });
  };

  const tabStyle = (active: boolean): React.CSSProperties => ({
    padding: "8px 16px", cursor: "pointer", fontSize: 13,
    fontWeight: active ? 600 : 400,
    color: active ? "#3370ff" : "var(--semi-color-text-2)",
    borderBottom: active ? "2px solid #3370ff" : "2px solid transparent",
    transition: "all 0.15s",
  });

  return (
    <div style={{ padding: "20px 24px 32px" }}>
      {/* 头部 */}
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: 12 }}>
        <div>
          <Text strong style={{ fontSize: 18, display: "block" }}>日志中心</Text>
          <Text type="tertiary" size="small">查看代理请求日志和系统运行日志</Text>
        </div>
        <Space>
          <Text size="small" type="tertiary">自动滚动</Text>
          <Switch size="small" checked={autoScroll} onChange={setAutoScroll} />
        </Space>
      </div>

      {/* Tab + 工具栏 */}
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "flex-end", borderBottom: "1px solid var(--semi-color-border)", marginBottom: 0 }}>
        <div style={{ display: "flex", gap: 0 }}>
          <div style={tabStyle(tab === "proxy")} onClick={() => setTab("proxy")}>
            代理日志 {proxyLogs.length > 0 && <Tag size="small" color="blue" type="light" style={{ marginLeft: 4 }}>{proxyLogs.length}</Tag>}
          </div>
          <div style={tabStyle(tab === "system")} onClick={() => setTab("system")}>
            系统日志 {systemLogs.length > 0 && <Tag size="small" color="green" type="light" style={{ marginLeft: 4 }}>{systemLogs.length}</Tag>}
          </div>
        </div>
        <Space style={{ marginBottom: 6 }}>
          <Button size="small" theme="borderless" icon={<IconSearch size="small" />} onClick={() => setShowFilters(!showFilters)}>
            {showFilters ? "收起" : "过滤"}
          </Button>
          <Button size="small" theme="borderless" icon={<IconCopy size="small" />} onClick={copyAll}>复制</Button>
          <Button size="small" theme="borderless" icon={<IconDelete size="small" />} onClick={clearLogs} type="danger">清空</Button>
          <Button size="small" theme="borderless" icon={<IconRefresh size="small" />} onClick={async () => {
            if (tab === "proxy") { const l = await wails().GetProxyLogs(); setProxyLogs((l || []).map(stripAnsi)); }
            else { const text = await wails().GetRecentLogs(200); setSystemLogs(text ? text.split("\n").filter((l: string) => l.trim()) : []); }
          }}>刷新</Button>
        </Space>
      </div>

      {/* 过滤器面板 */}
      {showFilters && (
        <div style={{ padding: "10px 12px", background: "var(--semi-color-fill-0)", borderBottom: "1px solid var(--semi-color-border)" }}>
          <div style={{ display: "flex", alignItems: "center", gap: 8, marginBottom: 8, flexWrap: "wrap" }}>
            <Text size="small" type="tertiary" style={{ marginRight: 4 }}>标签过滤:</Text>
            {LOG_TAGS.map(tag => (
              <Tag
                key={tag.key} size="small" type={activeFilters.has(tag.key) ? "solid" : "ghost"}
                style={{
                  cursor: "pointer", borderColor: tag.color,
                  background: activeFilters.has(tag.key) ? tag.color : "transparent",
                  color: activeFilters.has(tag.key) ? "#fff" : tag.color,
                }}
                onClick={() => toggleFilter(tag.key)}
              >
                {tag.label} {tagCounts[tag.key] ? `(${tagCounts[tag.key]})` : ""}
              </Tag>
            ))}
            {activeFilters.size > 0 && (
              <Button size="small" theme="borderless" type="tertiary" onClick={() => setActiveFilters(new Set())} style={{ fontSize: 11 }}>
                清除过滤
              </Button>
            )}
          </div>
          <Input size="small" prefix={<IconSearch />} placeholder="搜索日志内容..." value={search} onChange={setSearch}
            style={{ maxWidth: 360 }} showClear
          />
        </div>
      )}

      {/* 日志文件路径 */}
      {tab === "system" && logFilePath && (
        <div style={{ padding: "6px 12px", display: "flex", alignItems: "center", gap: 6, background: "var(--semi-color-fill-0)", borderBottom: "1px solid var(--semi-color-border)" }}>
          <IconFolder size="small" style={{ color: "var(--semi-color-text-2)" }} />
          <Text size="small" type="tertiary" style={{ fontFamily: "monospace", fontSize: 11 }}>{logFilePath}</Text>
          <Button size="small" theme="borderless" icon={<IconCopy size="small" />} style={{ padding: "0 4px", height: 20 }}
            onClick={() => { navigator.clipboard.writeText(logFilePath); }}
          />
        </div>
      )}

      {/* 日志内容 */}
      <Card bodyStyle={{ padding: 0 }} style={{ borderRadius: "0 0 10px 10px", borderTop: "none" }}>
        <div ref={scrollRef} style={{
          height: showFilters ? 400 : 480, overflowY: "auto",
          fontFamily: "'SF Mono', 'Fira Code', monospace", fontSize: 11, lineHeight: 1.7,
          color: "var(--semi-color-text-1)",
        }}>
          {filteredLogs.length === 0 ? (
            <div style={{ padding: 40, textAlign: "center" }}>
              <Text type="tertiary" size="small">
                {currentLogs.length === 0
                  ? (tab === "proxy" ? "暂无日志，启动代理后将在此显示请求日志" : "暂无系统日志")
                  : `${currentLogs.length} 条日志已被过滤`}
              </Text>
            </div>
          ) : (
            filteredLogs.map((line, i) => <LogLine key={i} line={line} index={i} isSystem={tab === "system"} />)
          )}
        </div>
        {/* 底部状态栏 */}
        <div style={{
          padding: "4px 12px", borderTop: "1px solid var(--semi-color-border)",
          display: "flex", justifyContent: "space-between", alignItems: "center",
          background: "var(--semi-color-fill-0)", borderRadius: "0 0 10px 10px",
        }}>
          <Text size="small" type="tertiary">
            {activeFilters.size > 0 || search ? `${filteredLogs.length} / ${currentLogs.length} 条` : `${currentLogs.length} 条`}
          </Text>
          <Text size="small" type="tertiary">
            {activeFilters.size > 0 && `过滤: ${[...activeFilters].map(k => LOG_TAGS.find(t => t.key === k)?.label).join(", ")}`}
          </Text>
        </div>
      </Card>
    </div>
  );
}
