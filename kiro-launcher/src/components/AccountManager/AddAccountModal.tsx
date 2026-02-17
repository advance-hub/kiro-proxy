import React, { useState } from "react";
import { Button, Card, Input, Typography, Toast, Select, Space } from "@douyinfe/semi-ui";
import { IconClose, IconKey, IconDownload } from "@douyinfe/semi-icons";
import * as App from "../../../frontend/wailsjs/go/main/App";

const { Text } = Typography;

const wails = () => App;

interface Props {
  onClose: () => void;
  onSuccess: () => void;
}

export default function AddAccountModal({ onClose, onSuccess }: Props) {
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [addType, setAddType] = useState<"social" | "idc" | "enterprise">("social");
  const [form, setForm] = useState({ refreshToken: "", clientId: "", clientSecret: "", region: "us-east-1", provider: "Google" });

  const handleSaveLocal = async () => {
    setLoading(true);
    setError("");
    try {
      await wails().ImportLocalAccount();
      Toast.success({ content: "导入成功" });
      onSuccess();
      onClose();
    } catch (e) { setError(String(e)); }
    finally { setLoading(false); }
  };

  const handleAddManual = async () => {
    if (!form.refreshToken) { setError("请输入 Refresh Token"); return; }
    if (!form.refreshToken.startsWith("aor")) { setError("Token 格式错误，应以 aor 开头"); return; }
    if ((addType === "idc" || addType === "enterprise") && (!form.clientId || !form.clientSecret)) { setError("请填写 Client ID 和 Client Secret"); return; }

    setLoading(true);
    setError("");
    try {
      if (addType === "social") {
        await wails().AddAccountBySocial(form.refreshToken, form.provider);
      } else {
        const provider = addType === "enterprise" ? "Enterprise" : "BuilderId";
        await wails().AddAccountByIdCWithProvider(form.refreshToken, form.clientId, form.clientSecret, form.region, provider);
      }
      Toast.success({ content: "账号添加成功" });
      onSuccess();
      onClose();
    } catch (e) { setError(String(e)); }
    finally { setLoading(false); }
  };

  return (
    <div style={{ position: "fixed", top: 0, left: 0, right: 0, bottom: 0, background: "rgba(0,0,0,0.5)", display: "flex", alignItems: "center", justifyContent: "center", zIndex: 1000 }} onClick={onClose}>
      <div style={{ width: 420, borderRadius: 12, background: "var(--semi-color-bg-1)", boxShadow: "0 4px 20px rgba(0,0,0,0.15)" }} onClick={(e) => e.stopPropagation()}>
        {/* Header */}
        <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", padding: "16px 20px", borderBottom: "1px solid var(--semi-color-border)" }}>
          <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
            <div style={{ width: 36, height: 36, borderRadius: 8, background: "var(--semi-color-primary-light-default)", display: "flex", alignItems: "center", justifyContent: "center" }}>
              <IconKey style={{ color: "var(--semi-color-primary)" }} />
            </div>
            <Text strong style={{ fontSize: 16 }}>添加账号</Text>
          </div>
          <Button theme="borderless" icon={<IconClose />} onClick={onClose} />
        </div>

        <div style={{ padding: "20px" }}>
          {/* Import Local */}
          <Button block theme="light" icon={<IconDownload />} loading={loading} onClick={handleSaveLocal} style={{ marginBottom: 16, padding: "12px", height: "auto" }}>
            <div style={{ textAlign: "left", marginLeft: 8 }}>
              <div style={{ fontWeight: 500 }}>导入本地账号</div>
              <div style={{ fontSize: 11, color: "var(--semi-color-text-2)", marginTop: 2 }}>从 Kiro 本地凭证导入</div>
            </div>
          </Button>

          {/* Divider */}
          <div style={{ display: "flex", alignItems: "center", gap: 12, margin: "16px 0" }}>
            <div style={{ flex: 1, height: 1, background: "var(--semi-color-border)" }} />
            <Text size="small" type="tertiary">或手动添加</Text>
            <div style={{ flex: 1, height: 1, background: "var(--semi-color-border)" }} />
          </div>

          {/* Type Switch */}
          <div style={{ display: "flex", gap: 8, marginBottom: 16 }}>
            <Button size="small" theme={addType === "social" ? "solid" : "light"} onClick={() => setAddType("social")}>Social (Google/Github)</Button>
            <Button size="small" theme={addType === "idc" ? "solid" : "light"} onClick={() => setAddType("idc")}>BuilderId (IdC)</Button>
            <Button size="small" theme={addType === "enterprise" ? "solid" : "light"} onClick={() => setAddType("enterprise")}>Enterprise</Button>
          </div>

          {/* Form */}
          {addType === "social" && (
            <div style={{ marginBottom: 12 }}>
              <Text size="small" style={{ display: "block", marginBottom: 4 }}>Provider</Text>
              <Select size="small" value={form.provider} onChange={(v) => setForm({ ...form, provider: v as string })} style={{ width: "100%" }}>
                <Select.Option value="Google">Google</Select.Option>
                <Select.Option value="Github">Github</Select.Option>
              </Select>
            </div>
          )}

          <div style={{ marginBottom: 12 }}>
            <Text size="small" style={{ display: "block", marginBottom: 4 }}>Refresh Token</Text>
            <Input size="small" placeholder="aor..." value={form.refreshToken} onChange={(v) => setForm({ ...form, refreshToken: v })} />
          </div>

          {addType === "idc" && (
            <>
              <div style={{ marginBottom: 12 }}>
                <Text size="small" style={{ display: "block", marginBottom: 4 }}>Client ID</Text>
                <Input size="small" value={form.clientId} onChange={(v) => setForm({ ...form, clientId: v })} />
              </div>
              <div style={{ marginBottom: 12 }}>
                <Text size="small" style={{ display: "block", marginBottom: 4 }}>Client Secret</Text>
                <Input size="small" type="password" value={form.clientSecret} onChange={(v) => setForm({ ...form, clientSecret: v })} />
              </div>
              <div style={{ marginBottom: 16 }}>
                <Text size="small" style={{ display: "block", marginBottom: 4 }}>Region</Text>
                <Select size="small" value={form.region} onChange={(v) => setForm({ ...form, region: v as string })} style={{ width: "100%" }}>
                  <Select.Option value="us-east-1">us-east-1 (N. Virginia)</Select.Option>
                  <Select.Option value="us-west-2">us-west-2 (Oregon)</Select.Option>
                  <Select.Option value="eu-west-1">eu-west-1 (Ireland)</Select.Option>
                </Select>
              </div>
            </>
          )}

          {addType === "enterprise" && (
            <>
              <div style={{ marginBottom: 12 }}>
                <Text size="small" style={{ display: "block", marginBottom: 4 }}>Client ID</Text>
                <Input size="small" value={form.clientId} onChange={(v) => setForm({ ...form, clientId: v })} />
              </div>
              <div style={{ marginBottom: 12 }}>
                <Text size="small" style={{ display: "block", marginBottom: 4 }}>Client Secret</Text>
                <Input size="small" type="password" value={form.clientSecret} onChange={(v) => setForm({ ...form, clientSecret: v })} />
              </div>
              <div style={{ marginBottom: 16 }}>
                <Text size="small" style={{ display: "block", marginBottom: 4 }}>Region</Text>
                <Select size="small" value={form.region} onChange={(v) => setForm({ ...form, region: v as string })} style={{ width: "100%" }}>
                  <Select.Option value="us-east-1">us-east-1 (N. Virginia)</Select.Option>
                  <Select.Option value="us-west-2">us-west-2 (Oregon)</Select.Option>
                  <Select.Option value="eu-west-1">eu-west-1 (Ireland)</Select.Option>
                  <Select.Option value="eu-central-1">eu-central-1 (Frankfurt)</Select.Option>
                  <Select.Option value="ap-southeast-1">ap-southeast-1 (Singapore)</Select.Option>
                </Select>
              </div>
            </>
          )}

          {/* Error */}
          {error && <div style={{ padding: "8px 12px", borderRadius: 6, background: "var(--semi-color-danger-light-default)", color: "var(--semi-color-danger)", fontSize: 12, marginBottom: 12 }}>{error}</div>}

          {/* Submit */}
          <Button block theme="solid" type="primary" loading={loading} onClick={handleAddManual} disabled={!form.refreshToken}>添加</Button>
        </div>
      </div>
    </div>
  );
}
