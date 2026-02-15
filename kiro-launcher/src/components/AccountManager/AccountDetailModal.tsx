import React, { useState } from "react";
import { Button, Typography, Tag, Toast, Input } from "@douyinfe/semi-ui";
import { IconClose, IconRefresh, IconCopy, IconTick } from "@douyinfe/semi-icons";

const { Text } = Typography;

const wails = () => {
  if (!window.go?.main?.App) throw new Error("Wails runtime 尚未就绪");
  return window.go.main.App;
};

interface Props {
  account: any;
  onClose: () => void;
}

// 配额计算
const getBreakdown = (a: any) => a.usageData?.usageBreakdownList?.[0] || a.usageData?.usageBreakdown || null;
const getQuota = (a: any) => {
  const breakdown = getBreakdown(a);
  const main = breakdown?.usageLimit ?? breakdown?.usage_limit ?? 50;
  const freeTrialInfo = breakdown?.freeTrialInfo || breakdown?.free_trial_info;
  const freeTrial = freeTrialInfo?.usageLimit ?? freeTrialInfo?.usage_limit ?? 0;
  const bonuses = breakdown?.bonuses || [];
  const bonus = bonuses.reduce((sum: number, b: any) => sum + (b.usageLimit || b.usage_limit || 0), 0);
  return main + freeTrial + bonus;
};
const getUsed = (a: any) => {
  const breakdown = getBreakdown(a);
  const main = breakdown?.currentUsage ?? breakdown?.current_usage ?? 0;
  const freeTrialInfo = breakdown?.freeTrialInfo || breakdown?.free_trial_info;
  const freeTrial = freeTrialInfo?.currentUsage ?? freeTrialInfo?.current_usage ?? 0;
  const bonuses = breakdown?.bonuses || [];
  const bonus = bonuses.reduce((sum: number, b: any) => sum + (b.currentUsage || b.current_usage || 0), 0);
  return main + freeTrial + bonus;
};

export default function AccountDetailModal({ account, onClose }: Props) {
  const [refreshing, setRefreshing] = useState(false);
  const [copied, setCopied] = useState<string | null>(null);
  const [form, setForm] = useState({
    label: account.label || "",
    accessToken: account.accessToken || "",
    refreshToken: account.refreshToken || "",
  });
  const [saving, setSaving] = useState(false);

  const breakdown = getBreakdown(account);
  const quota = getQuota(account);
  const used = getUsed(account);
  const percent = quota > 0 ? Math.round((used / quota) * 100) : 0;

  const handleCopy = (text: string, field: string) => {
    navigator.clipboard.writeText(text);
    setCopied(field);
    setTimeout(() => setCopied(null), 1500);
  };

  const handleRefresh = async () => {
    setRefreshing(true);
    try {
      await wails().SyncAccount(account.id);
      Toast.success({ content: "刷新成功" });
    } catch (e) { Toast.error({ content: String(e) }); }
    finally { setRefreshing(false); }
  };

  const handleSave = async () => {
    setSaving(true);
    try {
      await wails().UpdateAccount(
        account.id,
        form.label || null,
        form.accessToken || null,
        form.refreshToken || null,
        null, null
      );
      Toast.success({ content: "保存成功" });
      onClose();
    } catch (e) { Toast.error({ content: String(e) }); }
    finally { setSaving(false); }
  };

  const CopyBtn = ({ text, field }: { text: string; field: string }) => (
    <Button size="small" theme="borderless" type="tertiary" icon={copied === field ? <IconTick style={{ color: "#00b365" }} /> : <IconCopy />} onClick={() => handleCopy(text, field)} />
  );

  return (
    <div style={{ position: "fixed", inset: 0, background: "rgba(0,0,0,0.5)", display: "flex", alignItems: "center", justifyContent: "center", zIndex: 1000 }} onClick={onClose}>
      <div style={{ width: 600, maxHeight: "85vh", borderRadius: 16, background: "var(--semi-color-bg-1)", boxShadow: "0 4px 24px rgba(0,0,0,0.15)", display: "flex", flexDirection: "column" }} onClick={e => e.stopPropagation()}>
        {/* Header */}
        <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", padding: "16px 20px", borderBottom: "1px solid var(--semi-color-border)" }}>
          <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
            <div style={{ 
              width: 44, height: 44, borderRadius: 12, 
              background: account.provider === "Google" 
                ? "linear-gradient(135deg, #fee2e2 0%, #fecaca 100%)" 
                : account.provider === "Github"
                  ? "linear-gradient(135deg, #e5e7eb 0%, #d1d5db 100%)"
                  : "linear-gradient(135deg, #dbeafe 0%, #bfdbfe 100%)", 
              display: "flex", alignItems: "center", justifyContent: "center", 
              fontWeight: 700, 
              color: account.provider === "Google" ? "#dc2626" : account.provider === "Github" ? "#374151" : "#2563eb",
              boxShadow: "0 2px 8px rgba(0,0,0,0.08)",
            }}>
              {account.email[0]?.toUpperCase()}
            </div>
            <div>
              <Text strong>{account.email}</Text>
              <div style={{ display: "flex", gap: 4, marginTop: 4 }}>
                <Tag size="small" color={account.provider === "Google" ? "red" : account.provider === "Github" ? "grey" : "blue"} type="light">{account.provider}</Tag>
                <Tag size="small" color={account.status === "正常" || account.status === "有效" ? "green" : "red"} type="light">{account.status}</Tag>
              </div>
            </div>
          </div>
          <Button theme="borderless" icon={<IconClose />} onClick={onClose} />
        </div>

        {/* Content */}
        <div style={{ flex: 1, overflow: "auto", padding: 20 }}>
          {/* 配额信息 */}
          <div style={{ padding: 16, borderRadius: 12, background: "var(--semi-color-fill-0)", marginBottom: 16 }}>
            <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: 12 }}>
              <Text strong>配额信息</Text>
              <Button size="small" theme="light" icon={<IconRefresh spin={refreshing} />} onClick={handleRefresh} loading={refreshing}>刷新</Button>
            </div>
            <div style={{ display: "flex", alignItems: "baseline", gap: 8, marginBottom: 10 }}>
              <Text style={{ fontSize: 32, fontWeight: 700 }}>{Math.round(used * 100) / 100}</Text>
              <Text type="tertiary" style={{ fontSize: 16 }}>/ {quota}</Text>
              <Text style={{ 
                marginLeft: "auto", 
                fontWeight: 600, 
                fontSize: 18,
                color: percent > 80 ? "#ef4444" : percent > 50 ? "#f59e0b" : "#22c55e" 
              }}>{percent}%</Text>
            </div>
            <div style={{ height: 10, background: "var(--semi-color-fill-1)", borderRadius: 5, overflow: "hidden" }}>
              <div style={{ 
                height: "100%", 
                width: `${Math.min(percent, 100)}%`, 
                background: percent > 80 
                  ? "linear-gradient(90deg, #ef4444 0%, #dc2626 100%)" 
                  : percent > 50 
                    ? "linear-gradient(90deg, #f59e0b 0%, #d97706 100%)" 
                    : "linear-gradient(90deg, #22c55e 0%, #16a34a 100%)", 
                borderRadius: 5,
                transition: "width 0.5s ease",
              }} />
            </div>
            <div style={{ display: "flex", justifyContent: "space-between", marginTop: 8 }}>
              <Text size="small" type="tertiary">剩余 {Math.round((quota - used) * 100) / 100}</Text>
              {breakdown?.nextDateReset && (
                <Text size="small" type="tertiary">🔄 {new Date(breakdown.nextDateReset * 1000).toLocaleDateString()} 重置</Text>
              )}
            </div>
          </div>

          {/* 基本信息 */}
          <div style={{ marginBottom: 16 }}>
            <Text strong style={{ display: "block", marginBottom: 8 }}>备注</Text>
            <Input value={form.label} onChange={v => setForm({ ...form, label: v })} placeholder="添加备注..." />
          </div>

          {/* Token 信息 */}
          <div style={{ padding: 16, borderRadius: 12, background: "var(--semi-color-fill-0)" }}>
            <Text strong style={{ display: "block", marginBottom: 12 }}>Token 凭证</Text>
            
            <div style={{ marginBottom: 12 }}>
              <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: 4 }}>
                <Text size="small" type="tertiary">Access Token</Text>
                <CopyBtn text={form.accessToken} field="access" />
              </div>
              <Input value={form.accessToken} onChange={v => setForm({ ...form, accessToken: v })} placeholder="eyJ..." style={{ fontFamily: "monospace", fontSize: 11 }} />
            </div>

            <div style={{ marginBottom: 12 }}>
              <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: 4 }}>
                <Text size="small" type="tertiary">Refresh Token</Text>
                <CopyBtn text={form.refreshToken} field="refresh" />
              </div>
              <Input value={form.refreshToken} onChange={v => setForm({ ...form, refreshToken: v })} placeholder="aor..." style={{ fontFamily: "monospace", fontSize: 11 }} />
            </div>

            {account.expiresAt && (
              <div style={{ display: "flex", alignItems: "center", gap: 4 }}>
                <Text size="small" type="tertiary">🕐 过期时间:</Text>
                <Text size="small">{new Date(account.expiresAt).toLocaleString("zh-CN")}</Text>
              </div>
            )}

            {/* IdC 专用字段 */}
            {(account.provider === "BuilderId" || account.clientId) && (
              <div style={{ marginTop: 16, paddingTop: 16, borderTop: "1px solid var(--semi-color-border)" }}>
                <Text size="small" type="tertiary" style={{ display: "block", marginBottom: 12 }}>SSO 凭证</Text>
                <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 12 }}>
                  <div>
                    <Text size="small" type="tertiary" style={{ display: "block", marginBottom: 4 }}>Region</Text>
                    <div style={{ padding: "8px 12px", borderRadius: 8, background: "var(--semi-color-bg-1)", fontFamily: "monospace", fontSize: 12 }}>{account.region || "us-east-1"}</div>
                  </div>
                  <div>
                    <Text size="small" type="tertiary" style={{ display: "block", marginBottom: 4 }}>Client ID Hash</Text>
                    <div style={{ padding: "8px 12px", borderRadius: 8, background: "var(--semi-color-bg-1)", fontFamily: "monospace", fontSize: 12 }}>{account.clientIdHash || "-"}</div>
                  </div>
                </div>
                {account.clientId && (
                  <div style={{ marginTop: 12 }}>
                    <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: 4 }}>
                      <Text size="small" type="tertiary">Client ID</Text>
                      <CopyBtn text={account.clientId} field="clientId" />
                    </div>
                    <div style={{ padding: "8px 12px", borderRadius: 8, background: "var(--semi-color-bg-1)", fontFamily: "monospace", fontSize: 11, wordBreak: "break-all" }}>{account.clientId}</div>
                  </div>
                )}
                {account.clientSecret && (
                  <div style={{ marginTop: 12 }}>
                    <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: 4 }}>
                      <Text size="small" type="tertiary">Client Secret</Text>
                      <CopyBtn text={account.clientSecret} field="clientSecret" />
                    </div>
                    <div style={{ padding: "8px 12px", borderRadius: 8, background: "var(--semi-color-bg-1)", fontFamily: "monospace", fontSize: 11, wordBreak: "break-all", maxHeight: 60, overflow: "auto" }}>{account.clientSecret}</div>
                  </div>
                )}
              </div>
            )}
          </div>
        </div>

        {/* Footer */}
        <div style={{ display: "flex", justifyContent: "flex-end", gap: 8, padding: "16px 20px", borderTop: "1px solid var(--semi-color-border)" }}>
          <Button onClick={onClose}>取消</Button>
          <Button theme="solid" type="primary" loading={saving} onClick={handleSave}>保存</Button>
        </div>
      </div>
    </div>
  );
}
