import React, { useState, useEffect, useCallback } from "react";
import { Button, Card, Input, Typography, Toast, Space, Select, Switch } from "@douyinfe/semi-ui";
import { IconRefresh, IconSave } from "@douyinfe/semi-icons";

const { Text } = Typography;

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

const wails = () => {
  if (!window.go?.main?.App) throw new Error("Wails runtime 尚未就绪");
  return window.go.main.App;
};

const MODEL_OPTIONS = ["opus", "sonnet", "haiku", "gpt-5", "gpt-5-codex", "gpt-5-codex-max", "gemini-3-pro", "droid-core", "custom-model"];
const REASONING_OPTIONS = ["off", "none", "low", "medium", "high"];
const AUTONOMY_OPTIONS = ["normal", "spec", "auto-low", "auto-medium", "auto-high"];
const DIFF_OPTIONS = ["github", "unified"];
const SOUND_OPTIONS = ["off", "bell", "fx-ok01", "fx-ack01"];
const FOCUS_OPTIONS = ["always", "focused", "unfocused"];
const TODO_DISPLAY_OPTIONS = ["pinned", "inline"];

export default function SettingsPanel() {
  const [settings, setSettings] = useState<DroidSettings>({});
  const [loading, setLoading] = useState(false);
  const [dirty, setDirty] = useState(false);

  const loadSettings = useCallback(async () => {
    try {
      const s = await wails().ReadDroidSettings() as DroidSettings;
      setSettings(s || {});
      setDirty(false);
    } catch (e) { Toast.error({ content: String(e) }); }
  }, []);

  useEffect(() => { loadSettings(); }, [loadSettings]);

  const update = (key: string, value: any) => {
    setSettings(prev => ({ ...prev, [key]: value }));
    setDirty(true);
  };

  const handleSave = async () => {
    setLoading(true);
    try {
      const msg = await wails().WriteDroidSettings(settings);
      Toast.success({ content: msg });
      setDirty(false);
    } catch (e) { Toast.error({ content: String(e) }); }
    finally { setLoading(false); }
  };

  const labelStyle: React.CSSProperties = { fontSize: 13, color: "var(--semi-color-text-1)", fontWeight: 500 };
  const descStyle: React.CSSProperties = { fontSize: 11, color: "var(--semi-color-text-2)", marginTop: 2 };
  const rowStyle: React.CSSProperties = { display: "flex", alignItems: "center", justifyContent: "space-between", padding: "10px 0", borderBottom: "1px solid var(--semi-color-border)" };

  return (
    <div style={{ padding: "20px 24px 32px" }}>
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: 20 }}>
        <div>
          <Text strong style={{ fontSize: 18, display: "block" }}>Droid Settings</Text>
          <Text type="tertiary" size="small">配置 Droid CLI 行为和偏好</Text>
        </div>
        <Space>
          <Button size="small" theme="borderless" icon={<IconRefresh />} onClick={loadSettings}>刷新</Button>
          <Button size="small" theme="solid" type="primary" icon={<IconSave />} loading={loading} disabled={!dirty} onClick={handleSave}>保存</Button>
        </Space>
      </div>

      <Card bodyStyle={{ padding: "4px 16px" }} style={{ borderRadius: 10, marginBottom: 12 }}>
        <Text strong size="small" style={{ display: "block", padding: "10px 0 4px", color: "var(--semi-color-primary)" }}>模型与推理</Text>
        <div style={rowStyle}>
          <div><div style={labelStyle}>Model</div><div style={descStyle}>默认 AI 模型</div></div>
          <Select size="small" style={{ width: 200 }} value={settings.model || "opus"} onChange={v => update("model", v)}
            optionList={[...MODEL_OPTIONS.map(m => ({ label: m, value: m })), ...((settings.customModels || []) as any[]).map((cm: any) => ({ label: cm.displayName || cm.model, value: cm.model }))]} />
        </div>
        <div style={rowStyle}>
          <div><div style={labelStyle}>Reasoning Effort</div><div style={descStyle}>思考深度级别</div></div>
          <Select size="small" style={{ width: 180 }} value={settings.reasoningEffort || "off"} onChange={v => update("reasoningEffort", v)}>
            {REASONING_OPTIONS.map(m => <Select.Option key={m} value={m}>{m}</Select.Option>)}
          </Select>
        </div>
        <div style={{ ...rowStyle, borderBottom: "none" }}>
          <div><div style={labelStyle}>Show Thinking</div><div style={descStyle}>在主视图显示思考过程</div></div>
          <Switch checked={settings.showThinkingInMainView ?? false} onChange={v => update("showThinkingInMainView", v)} />
        </div>
      </Card>

      <Card bodyStyle={{ padding: "4px 16px" }} style={{ borderRadius: 10, marginBottom: 12 }}>
        <Text strong size="small" style={{ display: "block", padding: "10px 0 4px", color: "var(--semi-color-primary)" }}>行为与自动化</Text>
        <div style={rowStyle}>
          <div><div style={labelStyle}>Autonomy Level</div><div style={descStyle}>自动执行命令的级别</div></div>
          <Select size="small" style={{ width: 180 }} value={settings.autonomyLevel || "normal"} onChange={v => update("autonomyLevel", v)}>
            {AUTONOMY_OPTIONS.map(m => <Select.Option key={m} value={m}>{m}</Select.Option>)}
          </Select>
        </div>
        <div style={rowStyle}>
          <div><div style={labelStyle}>Hooks Disabled</div><div style={descStyle}>全局禁用所有 hooks</div></div>
          <Switch checked={settings.hooksDisabled ?? false} onChange={v => update("hooksDisabled", v)} />
        </div>
        <div style={rowStyle}>
          <div><div style={labelStyle}>Enable Custom Droids</div><div style={descStyle}>启用自定义 Droids</div></div>
          <Switch checked={settings.enableCustomDroids ?? true} onChange={v => update("enableCustomDroids", v)} />
        </div>
        <div style={{ ...rowStyle, borderBottom: "none" }}>
          <div><div style={labelStyle}>Allow Background Processes</div><div style={descStyle}>允许后台进程 (实验性)</div></div>
          <Switch checked={settings.allowBackgroundProcesses ?? false} onChange={v => update("allowBackgroundProcesses", v)} />
        </div>
      </Card>

      <Card bodyStyle={{ padding: "4px 16px" }} style={{ borderRadius: 10, marginBottom: 12 }}>
        <Text strong size="small" style={{ display: "block", padding: "10px 0 4px", color: "var(--semi-color-primary)" }}>界面与显示</Text>
        <div style={rowStyle}>
          <div><div style={labelStyle}>Diff Mode</div><div style={descStyle}>代码差异显示方式</div></div>
          <Select size="small" style={{ width: 180 }} value={settings.diffMode || "github"} onChange={v => update("diffMode", v)}>
            {DIFF_OPTIONS.map(m => <Select.Option key={m} value={m}>{m}</Select.Option>)}
          </Select>
        </div>
        <div style={{ ...rowStyle, borderBottom: "none" }}>
          <div><div style={labelStyle}>Todo Display Mode</div><div style={descStyle}>Todo 列表显示方式</div></div>
          <Select size="small" style={{ width: 180 }} value={settings.todoDisplayMode || "pinned"} onChange={v => update("todoDisplayMode", v)}>
            {TODO_DISPLAY_OPTIONS.map(m => <Select.Option key={m} value={m}>{m}</Select.Option>)}
          </Select>
        </div>
      </Card>

      <Card bodyStyle={{ padding: "4px 16px" }} style={{ borderRadius: 10, marginBottom: 12 }}>
        <Text strong size="small" style={{ display: "block", padding: "10px 0 4px", color: "var(--semi-color-primary)" }}>声音通知</Text>
        <div style={rowStyle}>
          <div><div style={labelStyle}>Completion Sound</div><div style={descStyle}>响应完成提示音</div></div>
          <Select size="small" style={{ width: 180 }} value={settings.completionSound || "fx-ok01"} onChange={v => update("completionSound", v)}>
            {SOUND_OPTIONS.map(m => <Select.Option key={m} value={m}>{m}</Select.Option>)}
          </Select>
        </div>
        <div style={rowStyle}>
          <div><div style={labelStyle}>Awaiting Input Sound</div><div style={descStyle}>等待输入提示音</div></div>
          <Select size="small" style={{ width: 180 }} value={settings.awaitingInputSound || "fx-ack01"} onChange={v => update("awaitingInputSound", v)}>
            {SOUND_OPTIONS.map(m => <Select.Option key={m} value={m}>{m}</Select.Option>)}
          </Select>
        </div>
        <div style={{ ...rowStyle, borderBottom: "none" }}>
          <div><div style={labelStyle}>Sound Focus Mode</div><div style={descStyle}>声音播放时机</div></div>
          <Select size="small" style={{ width: 180 }} value={settings.soundFocusMode || "always"} onChange={v => update("soundFocusMode", v)}>
            {FOCUS_OPTIONS.map(m => <Select.Option key={m} value={m}>{m}</Select.Option>)}
          </Select>
        </div>
      </Card>

      <Card bodyStyle={{ padding: "4px 16px" }} style={{ borderRadius: 10, marginBottom: 12 }}>
        <Text strong size="small" style={{ display: "block", padding: "10px 0 4px", color: "var(--semi-color-primary)" }}>同步与安全</Text>
        <div style={rowStyle}>
          <div><div style={labelStyle}>Cloud Session Sync</div><div style={descStyle}>同步会话到 Factory Web</div></div>
          <Switch checked={settings.cloudSessionSync ?? true} onChange={v => update("cloudSessionSync", v)} />
        </div>
        <div style={rowStyle}>
          <div><div style={labelStyle}>Include Co-Authored-By</div><div style={descStyle}>提交时附加 Droid 共同作者</div></div>
          <Switch checked={settings.includeCoAuthoredByDroid ?? true} onChange={v => update("includeCoAuthoredByDroid", v)} />
        </div>
        <div style={{ ...rowStyle, borderBottom: "none" }}>
          <div><div style={labelStyle}>Enable Droid Shield</div><div style={descStyle}>启用密钥扫描和 git 防护</div></div>
          <Switch checked={settings.enableDroidShield ?? true} onChange={v => update("enableDroidShield", v)} />
        </div>
      </Card>

      <Card bodyStyle={{ padding: "4px 16px" }} style={{ borderRadius: 10, marginBottom: 12 }}>
        <Text strong size="small" style={{ display: "block", padding: "10px 0 4px", color: "var(--semi-color-primary)" }}>IDE 与 Spec</Text>
        <div style={rowStyle}>
          <div><div style={labelStyle}>IDE Auto Connect</div><div style={descStyle}>从外部终端自动连接 IDE</div></div>
          <Switch checked={settings.ideAutoConnect ?? false} onChange={v => update("ideAutoConnect", v)} />
        </div>
        <div style={rowStyle}>
          <div><div style={labelStyle}>Spec Save Enabled</div><div style={descStyle}>将 spec 输出保存到磁盘</div></div>
          <Switch checked={settings.specSaveEnabled ?? false} onChange={v => update("specSaveEnabled", v)} />
        </div>
        <div style={{ ...rowStyle, borderBottom: "none" }}>
          <div><div style={labelStyle}>Spec Save Dir</div><div style={descStyle}>Spec 保存目录</div></div>
          <Input size="small" style={{ width: 180 }} value={settings.specSaveDir || ".factory/docs"} onChange={v => update("specSaveDir", v)} />
        </div>
      </Card>

      <Card bodyStyle={{ padding: "4px 16px" }} style={{ borderRadius: 10 }}>
        <Text strong size="small" style={{ display: "block", padding: "10px 0 4px", color: "var(--semi-color-primary)" }}>实验性功能</Text>
        <div style={{ ...rowStyle, borderBottom: "none" }}>
          <div><div style={labelStyle}>Enable Readiness Report</div><div style={descStyle}>启用 /readiness-report 命令</div></div>
          <Switch checked={settings.enableReadinessReport ?? false} onChange={v => update("enableReadinessReport", v)} />
        </div>
      </Card>
    </div>
  );
}
