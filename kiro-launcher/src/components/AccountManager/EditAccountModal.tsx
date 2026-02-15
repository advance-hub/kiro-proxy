import React, { useState } from "react";
import { Button, Typography, Input, Toast } from "@douyinfe/semi-ui";
import { IconClose, IconEdit } from "@douyinfe/semi-icons";

const { Text } = Typography;

const wails = () => {
  if (!window.go?.main?.App) throw new Error("Wails runtime 尚未就绪");
  return window.go.main.App;
};

interface Props {
  account: any;
  onClose: () => void;
  onSuccess: () => void;
}

export default function EditAccountModal({ account, onClose, onSuccess }: Props) {
  const [label, setLabel] = useState(account.label || "");
  const [saving, setSaving] = useState(false);

  const handleSave = async () => {
    setSaving(true);
    try {
      await wails().UpdateAccount(account.id, label || null, null, null, null, null);
      Toast.success({ content: "备注已更新" });
      onSuccess();
      onClose();
    } catch (e) {
      Toast.error({ content: String(e) });
    } finally {
      setSaving(false);
    }
  };

  return (
    <div style={{ position: "fixed", inset: 0, background: "rgba(0,0,0,0.5)", display: "flex", alignItems: "center", justifyContent: "center", zIndex: 1000 }} onClick={onClose}>
      <div style={{ width: 400, borderRadius: 16, background: "var(--semi-color-bg-1)", boxShadow: "0 4px 24px rgba(0,0,0,0.15)" }} onClick={e => e.stopPropagation()}>
        {/* Header */}
        <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", padding: "16px 20px", borderBottom: "1px solid var(--semi-color-border)" }}>
          <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
            <div style={{ width: 36, height: 36, borderRadius: 10, background: "var(--semi-color-primary-light-default)", display: "flex", alignItems: "center", justifyContent: "center" }}>
              <IconEdit style={{ color: "var(--semi-color-primary)" }} />
            </div>
            <Text strong style={{ fontSize: 16 }}>编辑备注</Text>
          </div>
          <Button theme="borderless" icon={<IconClose />} onClick={onClose} />
        </div>

        {/* Content */}
        <div style={{ padding: 20 }}>
          {/* Account Info */}
          <div style={{ display: "flex", alignItems: "center", gap: 10, marginBottom: 16, padding: 12, borderRadius: 10, background: "var(--semi-color-fill-0)" }}>
            <div style={{
              width: 36, height: 36, borderRadius: 8,
              background: account.provider === "Google" ? "#fee2e2" : account.provider === "Github" ? "#e5e7eb" : "#dbeafe",
              display: "flex", alignItems: "center", justifyContent: "center",
              fontWeight: 700, fontSize: 14,
              color: account.provider === "Google" ? "#dc2626" : account.provider === "Github" ? "#374151" : "#2563eb",
            }}>
              {account.email[0]?.toUpperCase()}
            </div>
            <div>
              <Text strong size="small">{account.email}</Text>
              <Text type="tertiary" size="small" style={{ display: "block" }}>{account.provider}</Text>
            </div>
          </div>

          {/* Label Input */}
          <div style={{ marginBottom: 16 }}>
            <Text size="small" style={{ display: "block", marginBottom: 6 }}>备注名称</Text>
            <Input 
              value={label} 
              onChange={setLabel} 
              placeholder="输入备注，方便识别账号..." 
              size="large"
              showClear
            />
            <Text type="tertiary" size="small" style={{ display: "block", marginTop: 4 }}>
              备注可以帮助你快速识别不同账号的用途
            </Text>
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
