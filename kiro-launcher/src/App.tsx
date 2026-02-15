import React, { useState, useEffect } from "react";
import { Button, Typography, Toast } from "@douyinfe/semi-ui";
import { IconPlay, IconKey, IconInfoCircle, IconSetting, IconLink, IconMoon, IconSun, IconHelpCircle } from "@douyinfe/semi-icons";

import ProxyPanel from "./components/ProxyPanel";
import AccountManager from "./components/AccountManager/index";
import LogsPanel from "./components/LogsPanel";
import SettingsPanel from "./components/SettingsPanel";
import OpenCodePanel from "./components/OpenCodePanel";
import ClaudeCodePanel from "./components/ClaudeCodePanel";
import TunnelPanel from "./components/TunnelPanel";
import AboutPanel from "./components/AboutPanel";

const { Text } = Typography;

const wails = () => {
  if (!window.go?.main?.App) throw new Error("Wails runtime 尚未就绪");
  return window.go.main.App;
};

function waitForWailsRuntime(timeout = 15000): Promise<void> {
  return new Promise((resolve, reject) => {
    if (window.go?.main?.App) { resolve(); return; }
    const start = Date.now();
    const timer = setInterval(() => {
      if (window.go?.main?.App) { clearInterval(timer); resolve(); return; }
      if (Date.now() - start > timeout) { clearInterval(timer); reject(new Error("Wails runtime 加载超时，请重启应用")); }
    }, 100);
  });
}

function useTheme() {
  const [dark, setDark] = useState(() => localStorage.getItem("kiro-theme") === "dark");
  useEffect(() => {
    const mode = dark ? "dark" : "light";
    document.body.setAttribute("theme-mode", mode);
    localStorage.setItem("kiro-theme", mode);
  }, [dark]);
  return { dark, toggle: () => setDark(d => !d) };
}

function ActivationGate({ onActivated }: { onActivated: () => void }) {
  const [code, setCode] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  const handleActivate = async () => {
    if (!code.trim()) { setError("请输入激活码"); return; }
    setLoading(true);
    setError("");
    try {
      await wails().Activate(code.trim());
      Toast.success({ content: "激活成功" });
      onActivated();
    } catch (e) { setError(String(e)); }
    finally { setLoading(false); }
  };

  return (
    <div style={{ minHeight: "100vh", display: "flex", alignItems: "center", justifyContent: "center", background: "var(--semi-color-bg-0)" }}>
      <div style={{ width: 380, padding: "32px 24px", borderRadius: 12, background: "var(--semi-color-bg-1)", boxShadow: "0 4px 20px rgba(0,0,0,0.1)" }}>
        <div style={{ textAlign: "center", marginBottom: 24 }}>
          <div style={{ width: 48, height: 48, borderRadius: 10, margin: "0 auto 12px", background: "var(--semi-color-primary)", display: "flex", alignItems: "center", justifyContent: "center", color: "#fff", fontWeight: 700, fontSize: 20 }}>K</div>
          <Text strong style={{ fontSize: 18, display: "block" }}>Kiro Launcher</Text>
          <Text type="tertiary" size="small" style={{ marginTop: 4, display: "block" }}>请输入激活码以使用本软件</Text>
        </div>
        <input
          type="text"
          placeholder="请输入激活码"
          value={code}
          onChange={(e) => setCode(e.target.value)}
          onKeyDown={(e) => e.key === "Enter" && handleActivate()}
          style={{ width: "100%", padding: "12px 16px", borderRadius: 8, border: "1px solid var(--semi-color-border)", marginBottom: 8, fontSize: 14 }}
        />
        {error && <Text type="danger" size="small" style={{ display: "block", marginBottom: 8 }}>{error}</Text>}
        <Button type="primary" theme="solid" size="large" block loading={loading} onClick={handleActivate} style={{ marginTop: 8 }}>激活</Button>
      </div>
    </div>
  );
}

function MainApp() {
  const { dark, toggle } = useTheme();
  const [activeTab, setActiveTab] = useState("proxy");

  const menuItems = [
    { key: "proxy", label: "代理", icon: <IconPlay /> },
    { key: "tunnel", label: "穿透", icon: <IconLink /> },
    { key: "accounts", label: "账号", icon: <IconKey /> },
    { key: "logs", label: "日志", icon: <IconInfoCircle /> },
    { key: "settings", label: "Droid", icon: <IconSetting /> },
    { key: "opencode", label: "OpenCode", icon: <IconLink /> },
    { key: "claudecode", label: "Claude Code", icon: <IconLink /> },
    { key: "about", label: "关于", icon: <IconHelpCircle /> },
  ];

  return (
    <div style={{ display: "flex", minHeight: "100vh", background: dark ? "#1a1a1a" : "#f5f5f5", transition: "background 0.3s" }}>
      {/* Sidebar */}
      <div style={{ width: 220, background: dark ? "#242424" : "#fff", borderRight: `1px solid ${dark ? "#333" : "#e8e8e8"}`, display: "flex", flexDirection: "column", transition: "background 0.3s, border-color 0.3s" }}>
        {/* Logo */}
        <div style={{ padding: "20px 16px", borderBottom: `1px solid ${dark ? "#333" : "#e8e8e8"}`, display: "flex", alignItems: "center", gap: 10 }}>
          <div style={{ width: 32, height: 32, borderRadius: 8, background: "linear-gradient(135deg, #3370ff, #5b8def)", display: "flex", alignItems: "center", justifyContent: "center", color: "#fff", fontWeight: 700, fontSize: 16 }}>K</div>
          <Text strong style={{ fontSize: 15 }}>Kiro Launcher</Text>
        </div>

        {/* Menu Items */}
        <div style={{ flex: 1, padding: "12px 8px" }}>
          {menuItems.map((item) => (
            <div
              key={item.key}
              onClick={() => setActiveTab(item.key)}
              style={{
                padding: "10px 12px", marginBottom: 4, borderRadius: 8, cursor: "pointer", display: "flex", alignItems: "center", gap: 10,
                background: activeTab === item.key ? (dark ? "#3370ff20" : "#3370ff15") : "transparent",
                color: activeTab === item.key ? "#3370ff" : (dark ? "#aaa" : "#666"),
                fontWeight: activeTab === item.key ? 600 : 400, transition: "all 0.2s",
              }}
            >
              {item.icon}
              <span style={{ fontSize: 14 }}>{item.label}</span>
            </div>
          ))}
        </div>

        {/* Theme Toggle */}
        <div style={{ padding: "12px 16px", borderTop: `1px solid ${dark ? "#333" : "#e8e8e8"}` }}>
          <Button theme="borderless" icon={dark ? <IconSun style={{ color: "#f5a623" }} /> : <IconMoon style={{ color: "#666" }} />} onClick={toggle} block style={{ borderRadius: 8 }}>
            {dark ? "浅色模式" : "深色模式"}
          </Button>
        </div>
      </div>

      {/* Main Content */}
      <div style={{ flex: 1, overflow: "auto" }}>
        {activeTab === "proxy" && <ProxyPanel />}
        {activeTab === "tunnel" && <TunnelPanel />}
        {activeTab === "accounts" && <AccountManager />}
        {activeTab === "logs" && <LogsPanel />}
        {activeTab === "settings" && <SettingsPanel />}
        {activeTab === "opencode" && <OpenCodePanel />}
        {activeTab === "claudecode" && <ClaudeCodePanel />}
        {activeTab === "about" && <AboutPanel />}
      </div>
    </div>
  );
}

export default function App() {
  const [runtimeReady, setRuntimeReady] = useState(false);
  const [runtimeError, setRuntimeError] = useState("");
  const [activated, setActivated] = useState<boolean | null>(null);

  useEffect(() => {
    waitForWailsRuntime()
      .then(() => { setRuntimeReady(true); return wails().CheckActivation(); })
      .then((data: any) => { setActivated(data.activated === true); })
      .catch((e) => {
        if (!runtimeReady) { setRuntimeError(String(e)); }
        else { Toast.error({ content: `检查激活状态失败: ${e}` }); setActivated(false); }
      });
  }, []);

  if (runtimeError) {
    return (
      <div style={{ minHeight: "100vh", display: "flex", alignItems: "center", justifyContent: "center", flexDirection: "column", gap: 12, background: "var(--semi-color-bg-0)", padding: 24 }}>
        <Text type="danger" style={{ fontSize: 14 }}>{runtimeError}</Text>
        <Text type="tertiary" size="small">请尝试重启应用，或检查 WebView2 Runtime 是否已安装</Text>
      </div>
    );
  }

  if (activated === null) {
    return <div style={{ minHeight: "100vh", display: "flex", alignItems: "center", justifyContent: "center", background: "var(--semi-color-bg-0)" }}><Text type="tertiary">加载中...</Text></div>;
  }

  if (!activated) {
    return <ActivationGate onActivated={() => setActivated(true)} />;
  }

  return <MainApp />;
}
