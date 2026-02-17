import React, { useState, useRef } from "react";
import { Button, Typography, Toast, Select, Card } from "@douyinfe/semi-ui";
import { IconClose, IconUpload, IconFile, IconTick, IconClear } from "@douyinfe/semi-icons";
import * as App from "../../../frontend/wailsjs/go/main/App";

const { Text } = Typography;

const wails = () => App;

interface Props {
  onClose: () => void;
  onSuccess: () => void;
}

interface ParsedAccount {
  refreshToken: string;
  provider?: string;
  clientId?: string;
  clientSecret?: string;
  region?: string;
  _type: "social" | "idc";
  _index: number;
}

export default function ImportAccountModal({ onClose, onSuccess }: Props) {
  const fileInputRef = useRef<HTMLInputElement>(null);
  const [jsonText, setJsonText] = useState("");
  const [parseResult, setParseResult] = useState<{ valid: ParsedAccount[]; errors: string[] } | null>(null);
  const [importing, setImporting] = useState(false);
  const [progress, setProgress] = useState({ current: 0, total: 0 });
  const [result, setResult] = useState<{ success: string[]; failed: string[] } | null>(null);

  const parseJson = (text: string) => {
    if (!text.trim()) { setParseResult(null); return; }
    try {
      let data = JSON.parse(text);
      if (!Array.isArray(data)) data = [data];

      const valid: ParsedAccount[] = [];
      const errors: string[] = [];

      data.forEach((item: any, index: number) => {
        if (!item.refreshToken) {
          errors.push(`#${index + 1}: 缺少 refreshToken`);
          return;
        }
        if (!item.refreshToken.startsWith("aor")) {
          errors.push(`#${index + 1}: refreshToken 格式错误`);
          return;
        }
        const hasClient = item.clientId && item.clientSecret;
        valid.push({
          ...item,
          _type: hasClient ? "idc" : "social",
          _index: index,
        });
      });

      setParseResult({ valid, errors });
    } catch (e: any) {
      setParseResult({ valid: [], errors: [`JSON 解析失败: ${e.message}`] });
    }
  };

  const handleFileSelect = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    const text = await file.text();
    setJsonText(text);
    parseJson(text);
  };

  const handleImport = async () => {
    if (!parseResult?.valid.length) return;
    setImporting(true);
    setProgress({ current: 0, total: parseResult.valid.length });

    const success: string[] = [];
    const failed: string[] = [];

    for (let i = 0; i < parseResult.valid.length; i++) {
      const item = parseResult.valid[i];
      setProgress({ current: i + 1, total: parseResult.valid.length });

      try {
        let account;
        if (item._type === "social") {
          account = await wails().AddAccountBySocial(item.refreshToken, item.provider || "Google");
        } else {
          // IdC 账号：支持 BuilderId 和 Enterprise
          const provider = item.provider || "BuilderId";
          account = await wails().AddAccountByIdCWithProvider(item.refreshToken, item.clientId!, item.clientSecret!, item.region || "us-east-1", provider);
        }
        success.push(account.email);
      } catch (e) {
        failed.push(`#${item._index + 1}: ${String(e).slice(0, 50)}`);
      }

      if (i < parseResult.valid.length - 1) {
        await new Promise(r => setTimeout(r, 500));
      }
    }

    setResult({ success, failed });
    setImporting(false);
    if (success.length > 0) onSuccess();
  };

  const handleReset = () => {
    setResult(null);
    setJsonText("");
    setParseResult(null);
  };

  return (
    <div style={{ position: "fixed", inset: 0, background: "rgba(0,0,0,0.5)", display: "flex", alignItems: "center", justifyContent: "center", zIndex: 1000 }} onClick={onClose}>
      <div style={{ width: 520, maxHeight: "80vh", borderRadius: 12, background: "var(--semi-color-bg-1)", boxShadow: "0 4px 20px rgba(0,0,0,0.15)", display: "flex", flexDirection: "column" }} onClick={e => e.stopPropagation()}>
        {/* Header */}
        <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", padding: "16px 20px", borderBottom: "1px solid var(--semi-color-border)" }}>
          <Text strong style={{ fontSize: 16 }}>导入账号</Text>
          <Button theme="borderless" icon={<IconClose />} onClick={onClose} disabled={importing} />
        </div>

        {/* Content */}
        <div style={{ flex: 1, overflow: "auto", padding: 20 }}>
          {result ? (
            <div>
              {result.success.length > 0 && (
                <div style={{ padding: 16, borderRadius: 10, background: "var(--semi-color-success-light-default)", marginBottom: 12 }}>
                  <div style={{ display: "flex", alignItems: "center", gap: 8, marginBottom: 8 }}>
                    <IconTick style={{ color: "var(--semi-color-success)" }} />
                    <Text strong style={{ color: "var(--semi-color-success)" }}>成功导入 {result.success.length} 个账号</Text>
                  </div>
                  <Text size="small" style={{ color: "var(--semi-color-success)" }}>{result.success.join(", ")}</Text>
                </div>
              )}
              {result.failed.length > 0 && (
                <div style={{ padding: 16, borderRadius: 10, background: "var(--semi-color-danger-light-default)" }}>
                  <div style={{ display: "flex", alignItems: "center", gap: 8, marginBottom: 8 }}>
                    <IconClear style={{ color: "var(--semi-color-danger)" }} />
                    <Text strong style={{ color: "var(--semi-color-danger)" }}>失败 {result.failed.length} 个</Text>
                  </div>
                  {result.failed.map((f, i) => (
                    <Text key={i} size="small" style={{ display: "block", color: "var(--semi-color-danger)" }}>{f}</Text>
                  ))}
                </div>
              )}
            </div>
          ) : importing ? (
            <div>
              <Text style={{ display: "block", marginBottom: 12 }}>正在导入...</Text>
              <div style={{ height: 8, background: "var(--semi-color-fill-1)", borderRadius: 4, overflow: "hidden", marginBottom: 8 }}>
                <div style={{ height: "100%", width: `${(progress.current / progress.total) * 100}%`, background: "var(--semi-color-primary)", borderRadius: 4, transition: "width 0.3s" }} />
              </div>
              <Text size="small" type="tertiary">{progress.current} / {progress.total}</Text>
            </div>
          ) : (
            <>
              <input ref={fileInputRef} type="file" accept=".json" onChange={handleFileSelect} style={{ display: "none" }} />
              
              <div style={{ display: "flex", gap: 8, marginBottom: 16 }}>
                <Button icon={<IconFile />} onClick={() => fileInputRef.current?.click()}>选择 JSON 文件</Button>
                <Button onClick={() => {
                  const tpl = JSON.stringify([{ refreshToken: "", provider: "Google" }], null, 2);
                  setJsonText(tpl);
                }}>Social 模板</Button>
                <Button onClick={() => {
                  const tpl = JSON.stringify([{ refreshToken: "", clientId: "", clientSecret: "", region: "us-east-1", provider: "BuilderId" }], null, 2);
                  setJsonText(tpl);
                }}>IdC 模板</Button>
                <Button onClick={() => {
                  const tpl = JSON.stringify([{ refreshToken: "", clientId: "", clientSecret: "", region: "us-east-1", provider: "Enterprise" }], null, 2);
                  setJsonText(tpl);
                }}>Enterprise 模板</Button>
              </div>

              <div style={{ marginBottom: 16 }}>
                <Text size="small" style={{ display: "block", marginBottom: 4 }}>或直接粘贴 JSON</Text>
                <textarea
                  value={jsonText}
                  onChange={e => { setJsonText(e.target.value); parseJson(e.target.value); }}
                  placeholder={`[
  { "refreshToken": "aor...", "provider": "Google" },
  { "refreshToken": "aor...", "clientId": "...", "clientSecret": "...", "region": "us-east-1" }
]`}
                  style={{ width: "100%", height: 200, padding: 12, borderRadius: 8, border: "1px solid var(--semi-color-border)", background: "var(--semi-color-fill-0)", fontFamily: "monospace", fontSize: 12, resize: "none" }}
                />
              </div>

              {parseResult && (
                <div>
                  {parseResult.valid.length > 0 && (
                    <Text size="small" style={{ display: "block", color: "var(--semi-color-success)", marginBottom: 4 }}>
                      ✓ 解析成功: {parseResult.valid.length} 条有效记录
                    </Text>
                  )}
                  {parseResult.errors.length > 0 && (
                    <div style={{ padding: 12, borderRadius: 8, background: "var(--semi-color-danger-light-default)" }}>
                      {parseResult.errors.map((err, i) => (
                        <Text key={i} size="small" style={{ display: "block", color: "var(--semi-color-danger)" }}>{err}</Text>
                      ))}
                    </div>
                  )}
                </div>
              )}
            </>
          )}
        </div>

        {/* Footer */}
        <div style={{ display: "flex", justifyContent: "flex-end", gap: 8, padding: "16px 20px", borderTop: "1px solid var(--semi-color-border)" }}>
          {result ? (
            <>
              <Button onClick={handleReset}>继续导入</Button>
              <Button theme="solid" type="primary" onClick={onClose}>完成</Button>
            </>
          ) : (
            <>
              <Button onClick={onClose} disabled={importing}>取消</Button>
              <Button theme="solid" type="primary" icon={<IconUpload />} onClick={handleImport} disabled={importing || !parseResult?.valid.length} loading={importing}>
                导入 {parseResult?.valid.length ? `(${parseResult.valid.length})` : ""}
              </Button>
            </>
          )}
        </div>
      </div>
    </div>
  );
}
