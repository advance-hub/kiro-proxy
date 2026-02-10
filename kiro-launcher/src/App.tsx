import { useState, useEffect, useCallback } from "react";
import { invoke } from "@tauri-apps/api/core";
import {
  Button, Card, Input, Typography, Tag, Toast,
  Collapsible, Space, Divider, Tooltip, Tabs, TabPane,
  Select, Switch, InputNumber,
} from "@douyinfe/semi-ui";
import {
  IconPlay, IconStop, IconRefresh,
  IconSetting, IconCopy, IconTick, IconChevronDown,
  IconChevronRight, IconLink, IconCheckCircleStroked,
  IconAlertCircle, IconInfoCircle, IconSave, IconMoon, IconSun,
  IconKey,
} from "@douyinfe/semi-icons";

const { Text } = Typography;

interface ProxyConfig {
  host: string;
  port: number;
  apiKey: string;
  region: string;
  tlsBackend: string;
}

interface StatusInfo {
  running: boolean;
  has_credentials: boolean;
  config: ProxyConfig;
}

interface DroidSettings {
  model?: string;
  reasoningEffort?: string;
  autonomyLevel?: string;
  diffMode?: string;
  cloudSessionSync?: boolean;
  completionSound?: string;
  awaitingInputSound?: string;
  soundFocusMode?: string;
  commandAllowlist?: string[];
  commandDenylist?: string[];
  includeCoAuthoredByDroid?: boolean;
  enableDroidShield?: boolean;
  hooksDisabled?: boolean;
  ideAutoConnect?: boolean;
  todoDisplayMode?: string;
  specSaveEnabled?: boolean;
  specSaveDir?: string;
  enableCustomDroids?: boolean;
  showThinkingInMainView?: boolean;
  allowBackgroundProcesses?: boolean;
  enableReadinessReport?: boolean;
  customModels?: any[];
  [key: string]: any;
}

const MODEL_OPTIONS = [
  "opus", "sonnet", "haiku", "gpt-5", "gpt-5-codex", "gpt-5-codex-max",
  "gemini-3-pro", "droid-core", "custom-model",
];
const REASONING_OPTIONS = ["off", "none", "low", "medium", "high"];
const AUTONOMY_OPTIONS = ["normal", "spec", "auto-low", "auto-medium", "auto-high"];
const DIFF_OPTIONS = ["github", "unified"];
const SOUND_OPTIONS = ["off", "bell", "fx-ok01", "fx-ack01"];
const FOCUS_OPTIONS = ["always", "focused", "unfocused"];
const TODO_DISPLAY_OPTIONS = ["pinned", "inline"];

function SettingsPanel() {
  const [settings, setSettings] = useState<DroidSettings>({});
  const [loading, setLoading] = useState(false);
  const [dirty, setDirty] = useState(false);

  const loadSettings = useCallback(async () => {
    try {
      const s = await invoke<DroidSettings>("read_droid_settings");
      setSettings(s);
      setDirty(false);
    } catch (e) {
      Toast.error({ content: String(e) });
    }
  }, []);

  useEffect(() => { loadSettings(); }, [loadSettings]);

  const update = (key: string, value: any) => {
    setSettings(prev => ({ ...prev, [key]: value }));
    setDirty(true);
  };

  const handleSave = async () => {
    setLoading(true);
    try {
      const msg = await invoke<string>("write_droid_settings", { settings });
      Toast.success({ content: msg });
      setDirty(false);
    } catch (e) {
      Toast.error({ content: String(e) });
    } finally {
      setLoading(false);
    }
  };

  const labelStyle: React.CSSProperties = {
    fontSize: 13, color: "var(--semi-color-text-1)", fontWeight: 500,
  };
  const descStyle: React.CSSProperties = {
    fontSize: 11, color: "var(--semi-color-text-2)", marginTop: 2,
  };
  const rowStyle: React.CSSProperties = {
    display: "flex", alignItems: "center", justifyContent: "space-between",
    padding: "10px 0", borderBottom: "1px solid var(--semi-color-border)",
  };

  return (
    <div style={{ padding: "16px 16px 24px" }}>
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: 12 }}>
        <Text strong>Droid Settings</Text>
        <Space>
          <Button size="small" theme="borderless" icon={<IconRefresh />} onClick={loadSettings}>
            刷新
          </Button>
          <Button
            size="small" theme="solid" type="primary"
            icon={<IconSave />}
            loading={loading}
            disabled={!dirty}
            onClick={handleSave}
          >
            保存
          </Button>
        </Space>
      </div>

      <Card bodyStyle={{ padding: "4px 16px" }} style={{ borderRadius: 10, marginBottom: 12 }}>
        <Text strong size="small" style={{ display: "block", padding: "10px 0 4px", color: "var(--semi-color-primary)" }}>
          模型与推理
        </Text>
        <div style={rowStyle}>
          <div>
            <div style={labelStyle}>Model</div>
            <div style={descStyle}>默认 AI 模型</div>
          </div>
          <Select
            size="small" style={{ width: 200 }}
            value={settings.model || "opus"}
            onChange={v => update("model", v)}
            optionList={[
              ...MODEL_OPTIONS.map(m => ({ label: m, value: m })),
              ...((settings.customModels || []) as any[]).map((cm: any) => ({
                label: cm.displayName || cm.model,
                value: cm.model,
              })),
            ]}
          />
        </div>
        <div style={rowStyle}>
          <div>
            <div style={labelStyle}>Reasoning Effort</div>
            <div style={descStyle}>思考深度级别</div>
          </div>
          <Select
            size="small" style={{ width: 180 }}
            value={settings.reasoningEffort || "off"}
            onChange={v => update("reasoningEffort", v)}
          >
            {REASONING_OPTIONS.map(m => <Select.Option key={m} value={m}>{m}</Select.Option>)}
          </Select>
        </div>
        <div style={{ ...rowStyle, borderBottom: "none" }}>
          <div>
            <div style={labelStyle}>Show Thinking</div>
            <div style={descStyle}>在主视图显示思考过程</div>
          </div>
          <Switch
            checked={settings.showThinkingInMainView ?? false}
            onChange={v => update("showThinkingInMainView", v)}
          />
        </div>
      </Card>

      <Card bodyStyle={{ padding: "4px 16px" }} style={{ borderRadius: 10, marginBottom: 12 }}>
        <Text strong size="small" style={{ display: "block", padding: "10px 0 4px", color: "var(--semi-color-primary)" }}>
          行为与自动化
        </Text>
        <div style={rowStyle}>
          <div>
            <div style={labelStyle}>Autonomy Level</div>
            <div style={descStyle}>自动执行命令的级别</div>
          </div>
          <Select
            size="small" style={{ width: 180 }}
            value={settings.autonomyLevel || "normal"}
            onChange={v => update("autonomyLevel", v)}
          >
            {AUTONOMY_OPTIONS.map(m => <Select.Option key={m} value={m}>{m}</Select.Option>)}
          </Select>
        </div>
        <div style={rowStyle}>
          <div>
            <div style={labelStyle}>Hooks Disabled</div>
            <div style={descStyle}>全局禁用所有 hooks</div>
          </div>
          <Switch
            checked={settings.hooksDisabled ?? false}
            onChange={v => update("hooksDisabled", v)}
          />
        </div>
        <div style={rowStyle}>
          <div>
            <div style={labelStyle}>Enable Custom Droids</div>
            <div style={descStyle}>启用自定义 Droids</div>
          </div>
          <Switch
            checked={settings.enableCustomDroids ?? true}
            onChange={v => update("enableCustomDroids", v)}
          />
        </div>
        <div style={{ ...rowStyle, borderBottom: "none" }}>
          <div>
            <div style={labelStyle}>Allow Background Processes</div>
            <div style={descStyle}>允许后台进程 (实验性)</div>
          </div>
          <Switch
            checked={settings.allowBackgroundProcesses ?? false}
            onChange={v => update("allowBackgroundProcesses", v)}
          />
        </div>
      </Card>

      <Card bodyStyle={{ padding: "4px 16px" }} style={{ borderRadius: 10, marginBottom: 12 }}>
        <Text strong size="small" style={{ display: "block", padding: "10px 0 4px", color: "var(--semi-color-primary)" }}>
          界面与显示
        </Text>
        <div style={rowStyle}>
          <div>
            <div style={labelStyle}>Diff Mode</div>
            <div style={descStyle}>代码差异显示方式</div>
          </div>
          <Select
            size="small" style={{ width: 180 }}
            value={settings.diffMode || "github"}
            onChange={v => update("diffMode", v)}
          >
            {DIFF_OPTIONS.map(m => <Select.Option key={m} value={m}>{m}</Select.Option>)}
          </Select>
        </div>
        <div style={{ ...rowStyle, borderBottom: "none" }}>
          <div>
            <div style={labelStyle}>Todo Display Mode</div>
            <div style={descStyle}>Todo 列表显示方式</div>
          </div>
          <Select
            size="small" style={{ width: 180 }}
            value={settings.todoDisplayMode || "pinned"}
            onChange={v => update("todoDisplayMode", v)}
          >
            {TODO_DISPLAY_OPTIONS.map(m => <Select.Option key={m} value={m}>{m}</Select.Option>)}
          </Select>
        </div>
      </Card>

      <Card bodyStyle={{ padding: "4px 16px" }} style={{ borderRadius: 10, marginBottom: 12 }}>
        <Text strong size="small" style={{ display: "block", padding: "10px 0 4px", color: "var(--semi-color-primary)" }}>
          声音通知
        </Text>
        <div style={rowStyle}>
          <div>
            <div style={labelStyle}>Completion Sound</div>
            <div style={descStyle}>响应完成提示音</div>
          </div>
          <Select
            size="small" style={{ width: 180 }}
            value={settings.completionSound || "fx-ok01"}
            onChange={v => update("completionSound", v)}
          >
            {SOUND_OPTIONS.map(m => <Select.Option key={m} value={m}>{m}</Select.Option>)}
          </Select>
        </div>
        <div style={rowStyle}>
          <div>
            <div style={labelStyle}>Awaiting Input Sound</div>
            <div style={descStyle}>等待输入提示音</div>
          </div>
          <Select
            size="small" style={{ width: 180 }}
            value={settings.awaitingInputSound || "fx-ack01"}
            onChange={v => update("awaitingInputSound", v)}
          >
            {SOUND_OPTIONS.map(m => <Select.Option key={m} value={m}>{m}</Select.Option>)}
          </Select>
        </div>
        <div style={{ ...rowStyle, borderBottom: "none" }}>
          <div>
            <div style={labelStyle}>Sound Focus Mode</div>
            <div style={descStyle}>声音播放时机</div>
          </div>
          <Select
            size="small" style={{ width: 180 }}
            value={settings.soundFocusMode || "always"}
            onChange={v => update("soundFocusMode", v)}
          >
            {FOCUS_OPTIONS.map(m => <Select.Option key={m} value={m}>{m}</Select.Option>)}
          </Select>
        </div>
      </Card>

      <Card bodyStyle={{ padding: "4px 16px" }} style={{ borderRadius: 10, marginBottom: 12 }}>
        <Text strong size="small" style={{ display: "block", padding: "10px 0 4px", color: "var(--semi-color-primary)" }}>
          同步与安全
        </Text>
        <div style={rowStyle}>
          <div>
            <div style={labelStyle}>Cloud Session Sync</div>
            <div style={descStyle}>同步会话到 Factory Web</div>
          </div>
          <Switch
            checked={settings.cloudSessionSync ?? true}
            onChange={v => update("cloudSessionSync", v)}
          />
        </div>
        <div style={rowStyle}>
          <div>
            <div style={labelStyle}>Include Co-Authored-By</div>
            <div style={descStyle}>提交时附加 Droid 共同作者</div>
          </div>
          <Switch
            checked={settings.includeCoAuthoredByDroid ?? true}
            onChange={v => update("includeCoAuthoredByDroid", v)}
          />
        </div>
        <div style={{ ...rowStyle, borderBottom: "none" }}>
          <div>
            <div style={labelStyle}>Enable Droid Shield</div>
            <div style={descStyle}>启用密钥扫描和 git 防护</div>
          </div>
          <Switch
            checked={settings.enableDroidShield ?? true}
            onChange={v => update("enableDroidShield", v)}
          />
        </div>
      </Card>

      <Card bodyStyle={{ padding: "4px 16px" }} style={{ borderRadius: 10, marginBottom: 12 }}>
        <Text strong size="small" style={{ display: "block", padding: "10px 0 4px", color: "var(--semi-color-primary)" }}>
          IDE 与 Spec
        </Text>
        <div style={rowStyle}>
          <div>
            <div style={labelStyle}>IDE Auto Connect</div>
            <div style={descStyle}>从外部终端自动连接 IDE</div>
          </div>
          <Switch
            checked={settings.ideAutoConnect ?? false}
            onChange={v => update("ideAutoConnect", v)}
          />
        </div>
        <div style={rowStyle}>
          <div>
            <div style={labelStyle}>Spec Save Enabled</div>
            <div style={descStyle}>将 spec 输出保存到磁盘</div>
          </div>
          <Switch
            checked={settings.specSaveEnabled ?? false}
            onChange={v => update("specSaveEnabled", v)}
          />
        </div>
        <div style={{ ...rowStyle, borderBottom: "none" }}>
          <div>
            <div style={labelStyle}>Spec Save Dir</div>
            <div style={descStyle}>Spec 保存目录</div>
          </div>
          <Input
            size="small" style={{ width: 180 }}
            value={settings.specSaveDir || ".factory/docs"}
            onChange={v => update("specSaveDir", v)}
          />
        </div>
      </Card>

      <Card bodyStyle={{ padding: "4px 16px" }} style={{ borderRadius: 10 }}>
        <Text strong size="small" style={{ display: "block", padding: "10px 0 4px", color: "var(--semi-color-primary)" }}>
          实验性功能
        </Text>
        <div style={{ ...rowStyle, borderBottom: "none" }}>
          <div>
            <div style={labelStyle}>Enable Readiness Report</div>
            <div style={descStyle}>启用 /readiness-report 命令</div>
          </div>
          <Switch
            checked={settings.enableReadinessReport ?? false}
            onChange={v => update("enableReadinessReport", v)}
          />
        </div>
      </Card>
    </div>
  );
}

interface ProxyModel {
  id: string;
  display_name: string;
  owned_by: string;
  max_tokens: number;
  type: string;
}

interface CredentialsInfo {
  exists: boolean;
  source: string;
  access_token: string;
  refresh_token: string;
  expires_at: string;
  auth_method: string;
  client_id: string;
  client_secret: string;
  expired: boolean;
}

function DataDirDisplay() {
  const [dir, setDir] = useState("");
  useEffect(() => {
    invoke<string>("get_data_dir_path").then(setDir).catch(() => {});
  }, []);
  return (
    <div style={{
      padding: "6px 10px", borderRadius: 4, marginBottom: 10,
      background: "var(--semi-color-fill-0)", fontFamily: "monospace", fontSize: 11,
      wordBreak: "break-all", color: "var(--semi-color-text-2)",
    }}>
      {dir ? `${dir}/credentials.json` : "加载中..."}
    </div>
  );
}

function ProxyPanel() {
  const [status, setStatus] = useState<StatusInfo | null>(null);
  const [host, setHost] = useState("127.0.0.1");
  const [port, setPort] = useState("8000");
  const [apiKey, setApiKey] = useState("kiro-proxy-123");
  const [region, setRegion] = useState("us-east-1");
  const [loading, setLoading] = useState("");
  const [showConfig, setShowConfig] = useState(false);
  const [copied, setCopied] = useState(false);
  const [models, setModels] = useState<ProxyModel[]>([]);
  const [modelsLoading, setModelsLoading] = useState(false);
  const [credsInfo, setCredsInfo] = useState<CredentialsInfo | null>(null);
  const [credsPath, setCredsPath] = useState("");
  const [showCredsEditor, setShowCredsEditor] = useState(false);
  const [keychainSources, setKeychainSources] = useState<any[]>([]);

  const refreshKeychainSources = useCallback(async () => {
    try {
      const sources = await invoke<any[]>("list_keychain_sources");
      setKeychainSources(sources);
    } catch (_) {}
  }, []);

  const refreshCredentials = useCallback(async () => {
    try {
      const info = await invoke<CredentialsInfo>("get_credentials_info");
      setCredsInfo(info);
    } catch (_) {}
  }, []);

  const handleImportCreds = async () => {
    if (!credsPath.trim()) {
      Toast.warning({ content: "请输入 credentials.json 路径" });
      return;
    }
    try {
      const msg = await invoke<string>("import_credentials", { path: credsPath.trim() });
      Toast.success({ content: msg });
      setCredsPath("");
      await refreshCredentials();
    } catch (e) {
      Toast.error({ content: String(e) });
    }
  };

  const refreshStatus = useCallback(async () => {
    try {
      const s = await invoke<StatusInfo>("get_status");
      setStatus(s);
      if (s.config) {
        setHost(s.config.host);
        setPort(String(s.config.port));
        setApiKey(s.config.apiKey);
        setRegion(s.config.region);
      }
    } catch (_) {}
  }, []);

  useEffect(() => {
    refreshStatus();
    refreshCredentials();
    refreshKeychainSources();
    const t = setInterval(refreshStatus, 3000);
    return () => clearInterval(t);
  }, [refreshStatus, refreshCredentials, refreshKeychainSources]);

  const wrap = async (key: string, fn: () => Promise<void>) => {
    setLoading(key);
    try { await fn(); } catch (e) { Toast.error({ content: String(e) }); } finally { setLoading(""); }
  };

  const fetchModels = async (proxyHost: string, proxyPort: string, proxyApiKey: string) => {
    const baseUrl = `http://${proxyHost}:${proxyPort}`;
    setModelsLoading(true);
    try {
      const resp = await fetch(`${baseUrl}/v1/models`, {
        headers: { "x-api-key": proxyApiKey },
      });
      if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
      const data = await resp.json();
      const list: ProxyModel[] = (data.data || []).map((m: any) => ({
        id: m.id,
        display_name: m.display_name || m.id,
        owned_by: m.owned_by || "unknown",
        max_tokens: m.max_tokens || 8192,
        type: m.type || "chat",
      }));
      setModels(list);
      return list;
    } catch (e) {
      Toast.warning({ content: `获取模型列表失败: ${e}` });
      return [];
    } finally {
      setModelsLoading(false);
    }
  };

  const syncModelsToFactory = async (proxyHost: string, proxyPort: string, proxyApiKey: string) => {
    const baseUrl = `http://${proxyHost}:${proxyPort}`;
    const list = await fetchModels(proxyHost, proxyPort, proxyApiKey);
    if (list.length === 0) return;
    try {
      // Write ~/.factory/config.json (custom_models for Droid CLI)
      const configModels = list.map((m) => ({
        model_display_name: `${m.display_name} [Kiro]`,
        model: m.id,
        base_url: baseUrl,
        api_key: proxyApiKey,
        provider: "anthropic",
        supports_vision: true,
        max_tokens: m.max_tokens,
      }));
      await invoke("write_factory_config", { config: { custom_models: configModels } });

      // Write ~/.factory/settings.json (customModels for Droid settings)
      const customModels = list.map((m, i) => ({
        displayName: `${m.display_name} [Kiro]`,
        id: `custom:${m.display_name.replace(/[\s()]/g, "-")}-[Kiro]-${i}`,
        index: i,
        model: m.id,
        baseUrl: baseUrl,
        apiKey: proxyApiKey,
        provider: "anthropic",
        noImageSupport: false,
        maxOutputTokens: m.max_tokens,
      }));
      const settings = await invoke<any>("read_droid_settings");
      settings.customModels = customModels;
      await invoke("write_droid_settings", { settings });

      Toast.success({ content: `已同步 ${list.length} 个模型到 config.json 和 settings.json` });
    } catch (e) {
      Toast.warning({ content: `模型同步失败: ${e}` });
    }
  };

  const handleOneClick = () => wrap("start", async () => {
    await invoke("ensure_factory_api_key");
    await invoke("save_config", { host, port: Number(port), apiKey, region });
    const r = await invoke<string>("one_click_start");
    Toast.success({ content: r });
    await refreshStatus();
    setTimeout(() => syncModelsToFactory(host, port, apiKey), 1500);
  });

  const handleStop = async () => {
    await invoke<string>("stop_proxy");
    Toast.success({ content: "代理已停止" });
    await refreshStatus();
  };

  const handleSaveConfig = async () => {
    try {
      await invoke<string>("save_config", { host, port: Number(port), apiKey, region });
      Toast.success({ content: "配置已保存" });
    } catch (e) { Toast.error({ content: String(e) }); }
  };

  const handleCopy = () => {
    navigator.clipboard.writeText(
      `ANTHROPIC_BASE_URL=http://${host}:${port}\nANTHROPIC_API_KEY=${apiKey}`
    );
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  const running = status?.running ?? false;

  return (
    <div style={{ padding: "16px 16px 24px" }}>
      <Card bodyStyle={{ padding: 0 }} style={{ marginBottom: 12, overflow: "hidden", borderRadius: 10 }}>
        <div style={{ padding: "16px 20px", display: "flex", alignItems: "center", gap: 12 }}>
          <div className="status-icon-bg" style={{
            width: 40, height: 40, borderRadius: "50%",
            background: running ? "#e8f5e9" : "var(--semi-color-fill-0)",
            display: "flex", alignItems: "center", justifyContent: "center",
          }}>
            {running
              ? <IconCheckCircleStroked style={{ color: "#00b365", fontSize: 20 }} />
              : <IconAlertCircle style={{ color: "#bbb", fontSize: 20 }} />
            }
          </div>
          <div style={{ flex: 1 }}>
            <Text strong>{running ? "代理服务运行中" : "代理服务未启动"}</Text>
            <div style={{ marginTop: 2 }}>
              {running ? (
                <Text type="tertiary" size="small" copyable style={{ fontFamily: "monospace" }}>
                  {`http://${host}:${port}`}
                </Text>
              ) : (
                <Text type="tertiary" size="small">点击下方按钮启动服务</Text>
              )}
            </div>
          </div>
        </div>
        <Divider style={{ margin: 0 }} />
        <div style={{ padding: "12px 20px" }}>
          {running ? (
            <Button type="danger" theme="solid" block icon={<IconStop />} onClick={handleStop}>
              停止代理
            </Button>
          ) : (
            <Button
              type="primary" theme="solid" block
              icon={<IconPlay />}
              loading={loading === "start"}
              onClick={handleOneClick}
            >
              一键启动
            </Button>
          )}
        </div>
      </Card>

      <Card bodyStyle={{ padding: "14px 20px" }} style={{ marginBottom: 12, borderRadius: 10 }}>
        <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: 10 }}>
          <Space>
            <IconKey style={{ color: "var(--semi-color-text-2)" }} />
            <Text strong>凭证信息</Text>
          </Space>
          <Space>
            <Button size="small" theme="borderless" type="tertiary" icon={<IconRefresh />} onClick={async () => {
              await refreshCredentials();
              await refreshKeychainSources();
            }}>
              刷新
            </Button>
            <Button size="small" theme="light" type="primary" loading={loading === "refresh"} onClick={async () => {
              setLoading("refresh");
              try {
                const msg = await invoke<string>("refresh_now");
                Toast.success({ content: msg });
                await refreshCredentials();
              } catch (e) { Toast.error({ content: String(e) }); }
              finally { setLoading(""); }
            }}>
              刷新 Token
            </Button>
            <Button size="small" theme="borderless" type="danger" onClick={async () => {
              try {
                const msg = await invoke<string>("clear_credentials");
                Toast.success({ content: msg });
                await refreshCredentials();
              } catch (e) { Toast.error({ content: String(e) }); }
            }}>
              清空
            </Button>
          </Space>
        </div>

        {keychainSources.length > 0 && (
          <div style={{
            padding: "8px 10px", borderRadius: 6, marginBottom: 8,
            background: "var(--semi-color-fill-0)", border: "1px solid var(--semi-color-border)",
          }}>
            <Text size="small" type="secondary" style={{ display: "block", marginBottom: 6 }}>
              Keychain 中找到 {keychainSources.length} 个凭据源，点击切换:
            </Text>
            <div style={{ display: "flex", gap: 6, flexWrap: "wrap" }}>
              {keychainSources.map((s: any) => (
                <Tag
                  key={s.source}
                  size="small"
                  color={s.source === "idc" ? "blue" : "cyan"}
                  type={credsInfo?.auth_method === s.source ? "solid" : "light"}
                  style={{ cursor: "pointer" }}
                  onClick={async () => {
                    try {
                      const msg = await invoke<string>("use_keychain_source", { source: s.source });
                      Toast.success({ content: msg });
                      await refreshCredentials();
                    } catch (e) { Toast.error({ content: String(e) }); }
                  }}
                >
                  {s.source.toUpperCase()}
                  {s.provider ? ` (${s.provider})` : ""}
                  {s.expired ? " [过期]" : ""}
                </Tag>
              ))}
            </div>
          </div>
        )}

        {!credsInfo || !credsInfo.exists ? (
          <div style={{
            padding: "12px", borderRadius: 6,
            background: "var(--semi-color-fill-0)", border: "1px solid var(--semi-color-border)",
            textAlign: "center",
          }}>
            <Text type="warning" size="small">未找到凭证，请先通过 Kiro IDE 登录</Text>
          </div>
        ) : (
          <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
            <div style={{
              display: "flex", alignItems: "center", gap: 8, marginBottom: 4,
            }}>
              <Tag size="small" color={credsInfo.expired ? "orange" : "green"} type="light">
                {credsInfo.expired ? "待刷新" : "有效"}
              </Tag>
              <Tag size="small" color="blue" type="light">
                {credsInfo.source === "file" ? "本地文件" : "Keychain"}
              </Tag>
              <Tag size="small" color="grey" type="light">
                {credsInfo.auth_method}
              </Tag>
            </div>
            {[
              { label: "Access Token", value: credsInfo.access_token },
              { label: "Refresh Token", value: credsInfo.refresh_token },
              { label: "Client ID", value: credsInfo.client_id },
              { label: "Client Secret", value: credsInfo.client_secret },
              { label: "过期时间", value: credsInfo.expires_at },
            ].map((item) => (
              <div key={item.label} style={{
                display: "flex", alignItems: "center", justifyContent: "space-between",
                padding: "6px 10px", borderRadius: 4, background: "var(--semi-color-fill-0)",
              }}>
                <Text type="secondary" size="small" style={{ minWidth: 100 }}>{item.label}</Text>
                <Text size="small" style={{ fontFamily: "'SF Mono', 'Fira Code', monospace", wordBreak: "break-all" }}>
                  {item.value || "-"}
                </Text>
              </div>
            ))}
          </div>
        )}
        <Divider style={{ margin: "10px 0" }} />
        <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: 8 }}>
          <Text size="small" type="secondary">凭据文件位置</Text>
          <Space>
            <Button size="small" theme="borderless" type="tertiary" onClick={async () => {
              try { await invoke("open_data_dir"); } catch (e) { Toast.error({ content: String(e) }); }
            }}>
              打开目录
            </Button>
          </Space>
        </div>
        <DataDirDisplay />

        <Collapsible isOpen={showCredsEditor}>
          <div style={{ marginBottom: 8 }}>
            <Text size="small" type="secondary" style={{ display: "block", marginBottom: 4 }}>
              获取 Token 命令参考
            </Text>
            <pre style={{
              margin: 0, padding: "8px 10px", borderRadius: 4, fontSize: 11,
              background: "var(--semi-color-fill-0)", border: "1px solid var(--semi-color-border)",
              fontFamily: "'SF Mono', 'Fira Code', monospace", whiteSpace: "pre-wrap",
              color: "var(--semi-color-text-1)", lineHeight: 1.6,
            }}>
{`# ── macOS ──
# 从 Keychain 读取 Social Token
security find-generic-password -s "kirocli:social:token" -w

# 从 Keychain 读取 IdC Token
security find-generic-password -s "kirocli:odic:token" -w

# 从 Keychain 读取 Device Registration (IdC)
security find-generic-password -s "kirocli:odic:device-registration" -w

# ── Windows (PowerShell) ──
# Kiro 凭据存储在 Windows Credential Manager
# 查看所有 Kiro 相关凭据
cmdkey /list | findstr kiro

# 或者用 PowerShell 读取
[System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String((cmdkey /generic:"kirocli:social:token" /pass 2>$null)))

# 如果上面不行，可以在以下路径找到缓存文件:
# %LOCALAPPDATA%\\kiro\\credentials.json
# %APPDATA%\\Kiro\\credentials.json`}
            </pre>
          </div>
          <Text size="small" type="secondary" style={{ display: "block", marginBottom: 4 }}>
            直接编辑 credentials.json
          </Text>
          <Input
            size="small"
            placeholder='粘贴完整 JSON，如 {"accessToken":"...","refreshToken":"...","expiresAt":"...","authMethod":"social"}'
            value={credsPath}
            onChange={setCredsPath}
            style={{ marginBottom: 8, fontFamily: "monospace", fontSize: 11 }}
          />
          <div style={{ display: "flex", gap: 8 }}>
            <Button size="small" theme="light" onClick={async () => {
              try {
                const raw = await invoke<string>("read_credentials_raw");
                setCredsPath(raw);
                Toast.success({ content: "已加载当前凭据" });
              } catch (e) { Toast.error({ content: String(e) }); }
            }}>
              加载当前
            </Button>
            <Button size="small" theme="solid" type="primary" onClick={async () => {
              if (!credsPath.trim()) { Toast.warning({ content: "请输入 JSON" }); return; }
              try {
                const msg = await invoke<string>("save_credentials_raw", { json: credsPath.trim() });
                Toast.success({ content: msg });
                await refreshCredentials();
              } catch (e) { Toast.error({ content: String(e) }); }
            }}>
              保存凭据
            </Button>
            <Button size="small" theme="light" onClick={handleImportCreds}>
              从文件导入
            </Button>
          </div>
        </Collapsible>
        <div
          style={{ textAlign: "center", padding: "6px 0", cursor: "pointer", marginTop: 6 }}
          onClick={() => setShowCredsEditor(!showCredsEditor)}
        >
          <Text size="small" type="tertiary">
            {showCredsEditor ? "收起编辑器" : "手动编辑凭据"}
          </Text>
          {showCredsEditor ? <IconChevronDown size="small" style={{ marginLeft: 4 }} /> : <IconChevronRight size="small" style={{ marginLeft: 4 }} />}
        </div>
      </Card>

      {models.length > 0 && (
        <Card bodyStyle={{ padding: "14px 20px" }} style={{ marginBottom: 12, borderRadius: 10 }}>
          <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: 10 }}>
            <Space>
              <IconInfoCircle style={{ color: "var(--semi-color-text-2)" }} />
              <Text strong>可用模型</Text>
              <Tag size="small" color="blue" type="light">{models.length}</Tag>
            </Space>
            <Button
              size="small" theme="borderless" type="tertiary"
              icon={<IconRefresh />}
              loading={modelsLoading}
              onClick={() => fetchModels(host, port, apiKey)}
            >
              刷新
            </Button>
          </div>
          <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
            {models.map((m) => (
              <div key={m.id} style={{
                padding: "8px 12px", borderRadius: 6,
                background: "var(--semi-color-fill-0)",
                border: "1px solid var(--semi-color-border)",
              }}>
                <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: 4 }}>
                  <Text strong size="small">{m.display_name}</Text>
                  <Tag size="small" color="green" type="light">{m.type}</Tag>
                </div>
                <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
                  <Text type="tertiary" size="small" style={{ fontFamily: "monospace" }}>{m.id}</Text>
                  <Text type="tertiary" size="small">|</Text>
                  <Text type="tertiary" size="small">{m.owned_by}</Text>
                  <Text type="tertiary" size="small">|</Text>
                  <Text type="tertiary" size="small">max: {m.max_tokens.toLocaleString()}</Text>
                </div>
              </div>
            ))}
          </div>
        </Card>
      )}

      <Card bodyStyle={{ padding: "14px 20px" }} style={{ marginBottom: 12, borderRadius: 10 }}>
        <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: 10 }}>
          <Space>
            <IconLink style={{ color: "var(--semi-color-text-2)" }} />
            <Text strong>客户端配置</Text>
          </Space>
          <Tooltip content={copied ? "已复制" : "复制到剪贴板"}>
            <Button
              size="small" theme="borderless" type="tertiary"
              icon={copied ? <IconTick style={{ color: "#00b365" }} /> : <IconCopy />}
              onClick={handleCopy}
            />
          </Tooltip>
        </div>
        <pre className="code-block" style={{
          margin: 0, padding: "10px 12px", borderRadius: 6,
          background: "var(--semi-color-fill-0)", border: "1px solid var(--semi-color-border)",
          fontSize: 12, fontFamily: "'SF Mono', 'Fira Code', monospace",
          color: "var(--semi-color-text-0)", lineHeight: 1.8, whiteSpace: "pre-wrap", wordBreak: "break-all",
        }}>
{`ANTHROPIC_BASE_URL=http://${host}:${port}
ANTHROPIC_API_KEY=${apiKey}`}
        </pre>
      </Card>

      <Card bodyStyle={{ padding: "14px 20px" }} style={{ marginBottom: 12, borderRadius: 10 }}>
        <Space style={{ marginBottom: 10 }}>
          <IconInfoCircle style={{ color: "var(--semi-color-text-2)" }} />
          <Text strong>API 端点</Text>
        </Space>
        <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
          {[
            { method: "GET", path: "/v1/models", desc: "模型列表" },
            { method: "POST", path: "/v1/messages", desc: "对话" },
            { method: "POST", path: "/v1/messages/count_tokens", desc: "Token 估算" },
            { method: "POST", path: "/cc/v1/messages", desc: "对话 (Claude Code)" },
            { method: "POST", path: "/cc/v1/messages/count_tokens", desc: "Token 估算 (CC)" },
          ].map((ep) => (
            <div key={ep.path} className="endpoint-row" style={{
              display: "flex", alignItems: "center", gap: 8,
              padding: "6px 10px", borderRadius: 4, background: "var(--semi-color-fill-0)",
            }}>
              <Tag
                size="small"
                color={ep.method === "GET" ? "green" : "blue"}
                type="light"
                style={{ fontFamily: "monospace", fontSize: 11, minWidth: 42, textAlign: "center" }}
              >
                {ep.method}
              </Tag>
              <Text style={{ fontFamily: "monospace", fontSize: 12, flex: 1 }}>
                {ep.path}
              </Text>
              <Text type="tertiary" size="small">{ep.desc}</Text>
            </div>
          ))}
        </div>
      </Card>

      <Card bodyStyle={{ padding: 0 }} style={{ borderRadius: 10 }}>
        <div
          style={{
            padding: "12px 20px", cursor: "pointer", userSelect: "none",
            display: "flex", alignItems: "center", justifyContent: "space-between",
          }}
          onClick={() => setShowConfig(!showConfig)}
        >
          <Space>
            <IconSetting style={{ color: "var(--semi-color-text-2)", fontSize: 14 }} />
            <Text strong size="small">高级配置</Text>
          </Space>
          {showConfig ? <IconChevronDown size="small" /> : <IconChevronRight size="small" />}
        </div>
        <Collapsible isOpen={showConfig}>
          <Divider style={{ margin: 0 }} />
          <div style={{ padding: "14px 20px" }}>
            <div style={{ display: "grid", gridTemplateColumns: "1fr 100px", gap: 8, marginBottom: 10 }}>
              <div>
                <Text size="small" type="secondary" style={{ marginBottom: 2, display: "block" }}>监听地址</Text>
                <Input size="small" value={host} onChange={setHost} />
              </div>
              <div>
                <Text size="small" type="secondary" style={{ marginBottom: 2, display: "block" }}>端口</Text>
                <Input size="small" value={port} onChange={setPort} />
              </div>
            </div>
            <div style={{ marginBottom: 10 }}>
              <Text size="small" type="secondary" style={{ marginBottom: 2, display: "block" }}>API Key</Text>
              <Input size="small" value={apiKey} onChange={setApiKey} />
            </div>
            <div style={{ marginBottom: 14 }}>
              <Text size="small" type="secondary" style={{ marginBottom: 2, display: "block" }}>Region</Text>
              <Input size="small" value={region} onChange={setRegion} />
            </div>
            <Button size="small" theme="light" block onClick={handleSaveConfig}>保存配置</Button>
          </div>
        </Collapsible>
      </Card>
    </div>
  );
}

const stripAnsi = (s: string) => s.replace(/\x1b\[[0-9;]*m/g, "");

function LogsPanel() {
  const [logs, setLogs] = useState<string[]>([]);
  const [autoScroll, setAutoScroll] = useState(true);
  const logsEndRef = useCallback((node: HTMLDivElement | null) => {
    if (node && autoScroll) node.scrollIntoView({ behavior: "smooth" });
  }, [autoScroll, logs]);

  useEffect(() => {
    const fetchLogs = async () => {
      try {
        const l = await invoke<string[]>("get_proxy_logs");
        setLogs(l.map(stripAnsi));
      } catch (_) {}
    };
    fetchLogs();
    const t = setInterval(fetchLogs, 1000);
    return () => clearInterval(t);
  }, []);

  return (
    <div style={{ padding: "16px 16px 24px" }}>
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: 12 }}>
        <Space>
          <Text strong>请求日志</Text>
          <Tag size="small" color="blue" type="light">{logs.length}</Tag>
        </Space>
        <Space>
          <Text size="small" type="tertiary">自动滚动</Text>
          <Switch size="small" checked={autoScroll} onChange={setAutoScroll} />
          <Button size="small" theme="borderless" icon={<IconRefresh />} onClick={async () => {
            const l = await invoke<string[]>("get_proxy_logs");
            setLogs(l.map(stripAnsi));
          }}>刷新</Button>
        </Space>
      </div>
      <Card bodyStyle={{ padding: 0 }} style={{ borderRadius: 10 }}>
        <div style={{
          height: 480, overflowY: "auto", padding: "10px 14px",
          fontFamily: "'SF Mono', 'Fira Code', monospace", fontSize: 11, lineHeight: 1.7,
          color: "var(--semi-color-text-1)", background: "var(--semi-color-fill-0)",
          borderRadius: 10,
        }}>
          {logs.length === 0 ? (
            <Text type="tertiary" size="small">暂无日志，启动代理后将在此显示请求日志</Text>
          ) : (
            logs.map((line, i) => (
              <div key={i} style={{
                padding: "2px 0",
                borderBottom: "1px solid var(--semi-color-border)",
                wordBreak: "break-all",
                whiteSpace: "pre-wrap",
              }}>{line}</div>
            ))
          )}
          <div ref={logsEndRef} />
        </div>
      </Card>
    </div>
  );
}

function useTheme() {
  const [dark, setDark] = useState(() => {
    const saved = localStorage.getItem("kiro-theme");
    return saved === "dark";
  });

  useEffect(() => {
    const mode = dark ? "dark" : "light";
    document.body.setAttribute("theme-mode", mode);
    localStorage.setItem("kiro-theme", mode);
  }, [dark]);

  return { dark, toggle: () => setDark(d => !d) };
}

export default function App() {
  const { dark, toggle } = useTheme();

  return (
    <div className="app-root" style={{ background: dark ? "#1a1a1a" : "#f5f5f5", minHeight: "100vh", transition: "background 0.3s" }}>
      <div className="app-header" style={{
        background: dark ? "#242424" : "#fff", borderBottom: `1px solid ${dark ? "#333" : "#e8e8e8"}`,
        padding: "14px 20px", display: "flex", alignItems: "center", justifyContent: "space-between",
        position: "sticky", top: 0, zIndex: 10, transition: "background 0.3s, border-color 0.3s",
      }}>
        <Space>
          <div style={{
            width: 28, height: 28, borderRadius: 6,
            background: "linear-gradient(135deg, #3370ff, #5b8def)",
            display: "flex", alignItems: "center", justifyContent: "center",
            color: "#fff", fontWeight: 700, fontSize: 13,
          }}>K</div>
          <Text strong style={{ fontSize: 15 }}>Kiro Launcher</Text>
        </Space>
        <Button
          theme="borderless"
          icon={dark ? <IconSun style={{ color: "#f5a623" }} /> : <IconMoon style={{ color: "#666" }} />}
          onClick={toggle}
          style={{ borderRadius: 8 }}
        />
      </div>

      <Tabs type="line" className="app-tabs" style={{ background: dark ? "#242424" : "#fff", transition: "background 0.3s", padding: "0 16px" }}>
        <TabPane tab="代理" itemKey="proxy">
          <ProxyPanel />
        </TabPane>
        <TabPane tab="日志" itemKey="logs">
          <LogsPanel />
        </TabPane>
        <TabPane tab="Settings" itemKey="settings">
          <SettingsPanel />
        </TabPane>
      </Tabs>
    </div>
  );
}
