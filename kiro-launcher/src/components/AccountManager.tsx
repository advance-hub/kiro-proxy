import React, { useState, useEffect, useCallback } from "react";
import { Button, Card, Input, Typography, Tag, Toast, Space, Empty } from "@douyinfe/semi-ui";
import { IconRefresh, IconCopy, IconTick, IconCheckCircleStroked, IconAlertCircle, IconChevronDown, IconChevronRight } from "@douyinfe/semi-icons";

const { Text } = Typography;

const wails = () => {
  if (!window.go?.main?.App) throw new Error("Wails runtime 尚未就绪");
  return window.go.main.App;
};

// 配额计算工具函数（参考 kiro-account-manager）
const getBreakdown = (a: any) => {
  return a.usageData?.usageBreakdownList?.[0] || a.usageData?.usageBreakdown || null;
};

const getQuota = (a: any) => {
  const breakdown = getBreakdown(a);
  if (!breakdown) return 50; // 默认值
  
  const main = breakdown.usageLimit ?? breakdown.usage_limit ?? 0;
  const freeTrialInfo = breakdown.freeTrialInfo || breakdown.free_trial_info;
  const freeTrial = freeTrialInfo?.usageLimit ?? freeTrialInfo?.usage_limit ?? 0;
  const bonuses = breakdown.bonuses || [];
  const bonus = bonuses.reduce((sum: number, b: any) => sum + (b.usageLimit || b.usage_limit || 0), 0);
  return main + freeTrial + bonus;
};

const getUsed = (a: any) => {
  const breakdown = getBreakdown(a);
  if (!breakdown) return 0;
  
  const main = breakdown.currentUsage ?? breakdown.current_usage ?? 0;
  const freeTrialInfo = breakdown.freeTrialInfo || breakdown.free_trial_info;
  const freeTrial = freeTrialInfo?.currentUsage ?? freeTrialInfo?.current_usage ?? 0;
  const bonuses = breakdown.bonuses || [];
  const bonus = bonuses.reduce((sum: number, b: any) => sum + (b.currentUsage || b.current_usage || 0), 0);
  return main + freeTrial + bonus;
};

const getSubType = (a: any) => a.usageData?.subscriptionInfo?.type ?? a.subscriptionType ?? "";
const getSubPlan = (a: any) => a.usageData?.subscriptionInfo?.subscriptionTitle ?? a.subscriptionPlan ?? "";

interface AccountCardProps {
  account: any;
  isCurrentAccount: boolean;
  onRefresh: (id: string) => void;
  onSwitch: (account: any) => void;
  refreshingId: string | null;
  switchingId: string | null;
}

function AccountCard({ account, isCurrentAccount, onRefresh, onSwitch, refreshingId, switchingId }: AccountCardProps) {
  const [copied, setCopied] = useState(false);
  const [expanded, setExpanded] = useState(false);

  const breakdown = getBreakdown(account);
  const quota = getQuota(account);
  const used = getUsed(account);
  const subType = getSubType(account);
  const subPlan = getSubPlan(account);
  const percent = quota > 0 ? Math.min(100, (used / quota) * 100) : 0;
  const remaining = quota - used;

  const handleCopy = () => {
    navigator.clipboard.writeText(account.email);
    setCopied(true);
    setTimeout(() => setCopied(false), 1500);
  };

  const getProgressColor = () => {
    if (percent > 80) return "#f5222d";
    if (percent > 50) return "#faad14";
    return "#52c41a";
  };

  const displayName = account.source?.includes("social") ? "Google 账号" :
                      account.source?.includes("Enterprise") ? "企业账号" : "IDC 账号";

  return (
    <Card
      bodyStyle={{ padding: 0 }}
      style={{
        marginBottom: 12,
        borderRadius: 12,
        border: isCurrentAccount ? "2px solid var(--semi-color-success)" : "1px solid var(--semi-color-border)",
        background: isCurrentAccount ? "var(--semi-color-success-light-default)" : "var(--semi-color-bg-1)",
      }}
    >
      <div
        style={{
          padding: "14px 18px",
          cursor: "pointer",
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
        }}
        onClick={() => setExpanded(!expanded)}
      >
        <div style={{ display: "flex", alignItems: "center", gap: 12, flex: 1 }}>
          <div
            style={{
              width: 40,
              height: 40,
              borderRadius: 10,
              background: account.provider === "Google" ? "#ea4335" : "#0078d4",
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              color: "#fff",
              fontWeight: 700,
              fontSize: 16,
            }}
          >
            {account.email?.[0]?.toUpperCase() || "K"}
          </div>
          <div style={{ flex: 1, minWidth: 0 }}>
            <div style={{ display: "flex", alignItems: "center", gap: 6 }}>
              <Text strong style={{ fontSize: 14 }}>{displayName}</Text>
              {isCurrentAccount && <Tag color="green" size="small">当前使用</Tag>}
              {(subType.includes("PRO+") || subPlan.includes("PRO+")) && (
                <Tag color="purple" size="small">PRO+</Tag>
              )}
              {(subType.includes("PRO") || subPlan.includes("PRO")) && !subType.includes("PRO+") && !subPlan.includes("PRO+") && (
                <Tag color="blue" size="small">PRO</Tag>
              )}
            </div>
            <Text type="tertiary" size="small" style={{ display: "block", marginTop: 2 }}>
              {account.email}
            </Text>
          </div>
          <div style={{ textAlign: "right" }}>
            <Text strong style={{ fontSize: 14, color: getProgressColor() }}>
              {Math.round(percent)}%
            </Text>
            <Text type="tertiary" size="small" style={{ display: "block", marginTop: 2 }}>
              {Math.round(used * 100) / 100} / {quota}
            </Text>
          </div>
          {expanded ? <IconChevronDown /> : <IconChevronRight />}
        </div>
      </div>

      {expanded && (
        <div style={{ padding: "0 18px 14px", borderTop: "1px solid var(--semi-color-border)" }}>
          {/* 配额进度条 */}
          <div style={{ marginTop: 12, marginBottom: 12 }}>
            <div style={{ display: "flex", justifyContent: "space-between", marginBottom: 6 }}>
              <Text size="small" type="secondary">配额使用</Text>
              <Text size="small" type="secondary">剩余 {Math.round(remaining * 100) / 100}</Text>
            </div>
            <div style={{ height: 8, background: "var(--semi-color-fill-0)", borderRadius: 4, overflow: "hidden" }}>
              <div
                style={{
                  height: "100%",
                  width: `${Math.min(percent, 100)}%`,
                  background: getProgressColor(),
                  transition: "width 0.3s",
                  borderRadius: 4,
                }}
              />
            </div>
          </div>

          {/* 详细信息 */}
          <div style={{ display: "flex", flexDirection: "column", gap: 8, marginBottom: 12 }}>
            <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
              <Text size="small" type="secondary">Provider:</Text>
              <Text size="small" style={{ fontFamily: "monospace" }}>{account.provider || account.source}</Text>
            </div>
            {account.expiresAt && (
              <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
                <Text size="small" type="secondary">Token 过期:</Text>
                <Text size="small">{new Date(account.expiresAt).toLocaleString("zh-CN")}</Text>
              </div>
            )}
            {breakdown?.nextDateReset && (
              <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
                <Text size="small" type="secondary">配额重置:</Text>
                <Text size="small">{new Date(breakdown.nextDateReset * 1000).toLocaleDateString("zh-CN")}</Text>
              </div>
            )}
            <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
              <Text size="small" type="secondary">邮箱:</Text>
              <div style={{ display: "flex", alignItems: "center", gap: 4 }}>
                <Text size="small" style={{ fontFamily: "monospace", fontSize: 11 }}>{account.email}</Text>
                <Button
                  size="small"
                  theme="borderless"
                  icon={copied ? <IconTick style={{ color: "#52c41a" }} /> : <IconCopy />}
                  onClick={handleCopy}
                />
              </div>
            </div>
          </div>

          {/* 操作按钮 */}
          <div style={{ display: "flex", gap: 8 }}>
            <Button
              size="small"
              theme="solid"
              type="primary"
              block
              loading={switchingId === account.id}
              disabled={isCurrentAccount || switchingId === account.id}
              onClick={() => onSwitch(account)}
            >
              {isCurrentAccount ? "当前账号" : "切换"}
            </Button>
            <Button
              size="small"
              theme="light"
              icon={<IconRefresh />}
              loading={refreshingId === account.id}
              onClick={() => onRefresh(account.id)}
            >
              刷新
            </Button>
          </div>
        </div>
      )}
    </Card>
  );
}

export default function AccountManager() {
  const [accounts, setAccounts] = useState<any[]>([]);
  const [currentToken, setCurrentToken] = useState<string | null>(null);
  const [refreshingId, setRefreshingId] = useState<string | null>(null);
  const [switchingId, setSwitchingId] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  const loadAccounts = useCallback(async () => {
    try {
      const sources = await wails().ListKeychainSources();
      setAccounts(sources || []);
    } catch (e) {
      Toast.error({ content: `加载账号失败: ${e}` });
    }
  }, []);

  const loadCurrentToken = useCallback(async () => {
    try {
      const info = await wails().GetCredentialsInfo();
      setCurrentToken(info?.source || null);
    } catch (e) {
      setCurrentToken(null);
    }
  }, []);

  useEffect(() => {
    loadAccounts();
    loadCurrentToken();
    const timer = setInterval(() => {
      loadAccounts();
      loadCurrentToken();
    }, 5000);
    return () => clearInterval(timer);
  }, [loadAccounts, loadCurrentToken]);

  const handleRefresh = async (id: string) => {
    setRefreshingId(id);
    try {
      await wails().SyncAccount(id);
      Toast.success({ content: "刷新成功" });
      await loadAccounts();
    } catch (e) {
      Toast.error({ content: `刷新失败: ${e}` });
    } finally {
      setRefreshingId(null);
    }
  };

  const handleSwitch = async (account: any) => {
    setSwitchingId(account.id);
    try {
      const msg = await wails().UseKeychainSource(account.source);
      Toast.success({ content: msg });
      await loadCurrentToken();
      await loadAccounts();
    } catch (e) {
      Toast.error({ content: `切换失败: ${e}` });
    } finally {
      setSwitchingId(null);
    }
  };

  const handleRefreshAll = async () => {
    setLoading(true);
    try {
      await wails().RefreshNow();
      Toast.success({ content: "批量刷新成功" });
      await loadAccounts();
    } catch (e) {
      Toast.error({ content: `批量刷新失败: ${e}` });
    } finally {
      setLoading(false);
    }
  };

  // 统计数据
  const totalQuota = Math.round(accounts.reduce((sum, a) => sum + getQuota(a), 0));
  const totalUsed = Math.round(accounts.reduce((sum, a) => sum + getUsed(a), 0));
  const stats = {
    total: accounts.length,
    totalQuota,
    totalUsed,
    remaining: totalQuota - totalUsed,
  };
  const usagePercent = stats.totalQuota > 0 ? (stats.totalUsed / stats.totalQuota * 100).toFixed(1) : "0";

  return (
    <div style={{ padding: "16px 16px 24px" }}>
      {/* 统计卡片 */}
      <Card bodyStyle={{ padding: "16px 20px" }} style={{ marginBottom: 16, borderRadius: 12 }}>
        <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: 12 }}>
          <Text strong style={{ fontSize: 16 }}>账号统计</Text>
          <Button size="small" theme="borderless" icon={<IconRefresh />} loading={loading} onClick={handleRefreshAll}>
            刷新全部
          </Button>
        </div>
        <div style={{ display: "grid", gridTemplateColumns: "repeat(4, 1fr)", gap: 16 }}>
          <div style={{ textAlign: "center", padding: "12px", background: "var(--semi-color-fill-0)", borderRadius: 8 }}>
            <Text type="secondary" size="small" style={{ display: "block", marginBottom: 4 }}>总账号</Text>
            <Text strong style={{ fontSize: 20, color: "var(--semi-color-primary)" }}>{stats.total}</Text>
          </div>
          <div style={{ textAlign: "center", padding: "12px", background: "var(--semi-color-fill-0)", borderRadius: 8 }}>
            <Text type="secondary" size="small" style={{ display: "block", marginBottom: 4 }}>总配额</Text>
            <Text strong style={{ fontSize: 20, color: "var(--semi-color-success)" }}>{stats.totalQuota}</Text>
          </div>
          <div style={{ textAlign: "center", padding: "12px", background: "var(--semi-color-fill-0)", borderRadius: 8 }}>
            <Text type="secondary" size="small" style={{ display: "block", marginBottom: 4 }}>已使用</Text>
            <Text strong style={{ fontSize: 20, color: "var(--semi-color-warning)" }}>{stats.totalUsed}</Text>
          </div>
          <div style={{ textAlign: "center", padding: "12px", background: "var(--semi-color-fill-0)", borderRadius: 8 }}>
            <Text type="secondary" size="small" style={{ display: "block", marginBottom: 4 }}>使用率</Text>
            <Text strong style={{ fontSize: 20, color: Number(usagePercent) > 80 ? "var(--semi-color-danger)" : "var(--semi-color-info)" }}>
              {usagePercent}%
            </Text>
          </div>
        </div>
      </Card>

      {/* 账号列表 */}
      <div>
        <Text strong style={{ display: "block", marginBottom: 12, fontSize: 15 }}>
          账号列表 ({accounts.length})
        </Text>
        {accounts.length === 0 ? (
          <Empty description="暂无账号" style={{ padding: "40px 0" }} />
        ) : (
          accounts.map((account) => (
            <AccountCard
              key={account.source}
              account={account}
              isCurrentAccount={account.source === currentToken}
              onRefresh={handleRefresh}
              onSwitch={handleSwitch}
              refreshingId={refreshingId}
              switchingId={switchingId}
            />
          ))
        )}
      </div>
    </div>
  );
}
