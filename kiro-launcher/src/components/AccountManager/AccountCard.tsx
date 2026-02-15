import React, { useState } from "react";
import { Card, Typography, Tag, Tooltip, Button } from "@douyinfe/semi-ui";
import { IconRefresh, IconStop, IconCopy, IconTick, IconEyeOpened, IconEdit, IconSync } from "@douyinfe/semi-icons";

const { Text } = Typography;

interface Account {
  id: string;
  email: string;
  label: string;
  status: string;
  provider: string;
  expiresAt?: string;
  usageData?: any;
  quota?: number;
  used?: number;
  subscriptionType?: string;
  subscriptionPlan?: string;
}

interface Props {
  account: Account;
  isSelected: boolean;
  isCurrentAccount?: boolean;
  onSelect: (checked: boolean) => void;
  onSwitch: () => void;
  onRefresh: () => void;
  onDelete: () => void;
  onViewDetail: () => void;
  onEditLabel?: () => void;
  refreshing: boolean;
  switching: boolean;
}

// ========== 配额计算工具函数 (与参考项目保持一致) ==========

// 兼容 usageBreakdownList（数组）和 usageBreakdown（单个对象）
const getBreakdown = (a: Account) => {
  return a.usageData?.usageBreakdownList?.[0] || a.usageData?.usageBreakdown || null;
};

// 计算总配额 - 兼容 camelCase 和 snake_case
const getQuota = (a: Account) => {
  const breakdown = getBreakdown(a);
  if (!breakdown) return a.quota ?? null; // 没有数据时返回 null
  const main = breakdown?.usageLimit ?? breakdown?.usage_limit ?? 0;
  const freeTrialInfo = breakdown?.freeTrialInfo || breakdown?.free_trial_info;
  const freeTrial = freeTrialInfo?.usageLimit ?? freeTrialInfo?.usage_limit ?? 0;
  const bonuses = breakdown?.bonuses || [];
  const bonus = bonuses.reduce((sum: number, b: any) => sum + (b.usageLimit || b.usage_limit || 0), 0);
  return main + freeTrial + bonus;
};

// 计算已用配额 - 兼容 camelCase 和 snake_case
const getUsed = (a: Account) => {
  const breakdown = getBreakdown(a);
  if (!breakdown) return a.used ?? null; // 没有数据时返回 null
  const main = breakdown?.currentUsage ?? breakdown?.current_usage ?? 0;
  const freeTrialInfo = breakdown?.freeTrialInfo || breakdown?.free_trial_info;
  const freeTrial = freeTrialInfo?.currentUsage ?? freeTrialInfo?.current_usage ?? 0;
  const bonuses = breakdown?.bonuses || [];
  const bonus = bonuses.reduce((sum: number, b: any) => sum + (b.currentUsage || b.current_usage || 0), 0);
  return main + freeTrial + bonus;
};

// 订阅类型 - 没有数据时返回 null
const getSubType = (a: Account) => a.usageData?.subscriptionInfo?.type ?? a.subscriptionType ?? null;
const getSubPlan = (a: Account) => a.usageData?.subscriptionInfo?.subscriptionTitle ?? a.subscriptionPlan ?? null;

// 计算使用百分比
const getUsagePercent = (used: number, quota: number) => {
  return quota === 0 ? 0 : Math.min(100, (used / quota) * 100);
};

// ========== 组件 ==========

export default function AccountCard({ account, isSelected, isCurrentAccount, onSelect, onSwitch, onRefresh, onDelete, onViewDetail, onEditLabel, refreshing, switching }: Props) {
  const [copied, setCopied] = useState(false);

  const quota = getQuota(account);
  const used = getUsed(account);
  const subType = getSubType(account);
  const subPlan = getSubPlan(account);
  const breakdown = getBreakdown(account);
  
  // 没有配额数据时显示 "--"
  const hasUsageData = quota !== null && used !== null;
  const percent = hasUsageData ? getUsagePercent(used, quota) : 0;
  
  // 判断过期 - 兼容 / 和 - 分隔符
  const isExpired = account.expiresAt && new Date(account.expiresAt.replace(/\//g, '-')) < new Date();
  const isBanned = account.status === "封禁" || account.status === "已封禁";
  const isNormal = account.status === "正常" || account.status === "有效";

  // 订阅标签显示 - 没有数据时显示 "--"
  const getSubLabel = () => {
    if (!subType && !subPlan) return "--"; // 没有数据
    if (subType?.includes("PRO+") || subPlan?.includes("PRO+")) return "PRO+";
    if (subType?.includes("PRO") || subPlan?.includes("PRO")) return "PRO";
    return subPlan || "Free";
  };
  const subLabel = getSubLabel();

  const handleCopy = () => {
    navigator.clipboard.writeText(account.email);
    setCopied(true);
    setTimeout(() => setCopied(false), 1500);
  };

  // 边框样式
  const getBorderStyle = () => {
    if (isSelected) return "2px solid var(--semi-color-primary)";
    if (isCurrentAccount) return "2px solid #22c55e";
    if (isBanned) return "1px solid var(--semi-color-danger)";
    if (!isNormal) return "1px solid var(--semi-color-warning)";
    return "1px solid var(--semi-color-border)";
  };

  return (
    <Card
      bodyStyle={{ padding: 16 }}
      style={{
        borderRadius: 12,
        border: getBorderStyle(),
        opacity: isBanned ? 0.7 : 1,
        position: "relative",
        transition: "all 0.2s",
        boxShadow: isCurrentAccount ? "0 0 16px rgba(34, 197, 94, 0.2)" : undefined,
      }}
    >
      {/* Checkbox */}
      <div style={{ position: "absolute", top: 12, left: 12 }}>
        <input type="checkbox" checked={isSelected} onChange={(e) => onSelect(e.target.checked)} style={{ width: 16, height: 16, cursor: "pointer" }} />
      </div>

      {/* Status Tag */}
      <div style={{ position: "absolute", top: 12, right: 12, display: "flex", gap: 4 }}>
        {isCurrentAccount && <Tag size="small" color="green" type="solid">当前使用</Tag>}
        <Tag size="small" color={isNormal ? "green" : isBanned ? "red" : "orange"} type={isNormal ? "light" : "solid"}>
          {account.status}
        </Tag>
      </div>

      {/* Header */}
      <div style={{ display: "flex", alignItems: "center", gap: 10, marginBottom: 12, paddingTop: 24 }}>
        <div style={{
          width: 40, height: 40, borderRadius: 10,
          background: account.provider === "Google" ? "#fee2e2" : account.provider === "Github" ? "#e5e7eb" : "#dbeafe",
          display: "flex", alignItems: "center", justifyContent: "center",
          fontWeight: 700, fontSize: 16,
          color: account.provider === "Google" ? "#dc2626" : account.provider === "Github" ? "#374151" : "#2563eb",
        }}>
          {account.email[0]?.toUpperCase() || "?"}
        </div>
        <div style={{ flex: 1, minWidth: 0 }}>
          <div style={{ display: "flex", alignItems: "center", gap: 4 }}>
            <Text strong size="small" ellipsis={{ showTooltip: true }} style={{ maxWidth: 180 }}>{account.email}</Text>
            <Tooltip content={copied ? "已复制" : "复制"}>
              <Button size="small" theme="borderless" type="tertiary" icon={copied ? <IconTick style={{ color: "#00b365" }} /> : <IconCopy />} onClick={handleCopy} style={{ padding: 2 }} />
            </Tooltip>
          </div>
          <div style={{ display: "flex", alignItems: "center", gap: 4, marginTop: 4 }}>
            {/* 订阅类型标签 */}
            <Tag 
              size="small" 
              type={subLabel.includes("PRO") ? "solid" : "light"}
              color={subLabel.includes("PRO+") ? "purple" : subLabel.includes("PRO") ? "blue" : "grey"}
            >
              {subLabel}
            </Tag>
            {/* Provider 标签 */}
            <Tag size="small" color={account.provider === "Google" ? "red" : account.provider === "Github" ? "grey" : "blue"} type="light">
              {account.provider}
            </Tag>
          </div>
        </div>
      </div>

      {/* Label */}
      {account.label && (
        <div style={{ display: "flex", alignItems: "center", gap: 4, marginBottom: 10 }}>
          <Text size="small" type="tertiary" ellipsis style={{ flex: 1 }}>{account.label}</Text>
          {onEditLabel && <Button size="small" theme="borderless" type="tertiary" icon={<IconEdit />} onClick={onEditLabel} style={{ padding: 2 }} />}
        </div>
      )}

      {/* Usage Progress */}
      <div style={{ padding: "10px 12px", borderRadius: 8, background: "var(--semi-color-fill-0)", marginBottom: 12 }}>
        <div style={{ display: "flex", justifyContent: "space-between", marginBottom: 6 }}>
          <Text size="small" type="tertiary">配额使用</Text>
          <Text size="small" style={{ fontWeight: 600, color: !hasUsageData ? "var(--semi-color-text-2)" : percent > 80 ? "#ef4444" : percent > 50 ? "#f59e0b" : "#22c55e" }}>
            {hasUsageData ? `${Math.round(percent)}%` : "--"}
          </Text>
        </div>
        <div style={{ height: 6, background: "var(--semi-color-fill-1)", borderRadius: 3, overflow: "hidden" }}>
          <div style={{
            height: "100%", borderRadius: 3, transition: "width 0.3s",
            width: hasUsageData ? `${Math.min(percent, 100)}%` : "0%",
            background: percent > 80 ? "#ef4444" : percent > 50 ? "#f59e0b" : "#22c55e",
          }} />
        </div>
        <div style={{ display: "flex", justifyContent: "space-between", marginTop: 4 }}>
          <Text size="small" type="tertiary">
            {hasUsageData ? `${Math.round(used! * 100) / 100} / ${quota}` : "点击刷新获取配额"}
          </Text>
          <Text size="small" type="tertiary">
            {hasUsageData ? `剩余 ${Math.round((quota! - used!) * 100) / 100}` : ""}
          </Text>
        </div>
        {breakdown?.nextDateReset && (
          <Text size="small" type="tertiary" style={{ display: "block", marginTop: 4 }}>
            🔄 {new Date(breakdown.nextDateReset * 1000).toLocaleDateString()} 重置
          </Text>
        )}
      </div>

      {/* Token Expiry */}
      {account.expiresAt && (
        <div style={{ fontSize: 11, color: isExpired ? "var(--semi-color-warning)" : "var(--semi-color-text-2)", marginBottom: 12 }}>
          🕐 Token: {account.expiresAt}
          {isExpired && <Tag size="small" color="orange" style={{ marginLeft: 4 }}>已过期</Tag>}
        </div>
      )}

      {/* Actions */}
      <div style={{ display: "flex", gap: 8, borderTop: "1px solid var(--semi-color-border)", paddingTop: 12 }}>
        <Button size="small" theme="solid" type="primary" style={{ flex: 1 }} loading={switching} onClick={onSwitch} icon={<IconSync />}>
          切换
        </Button>
        <Tooltip content="查看详情">
          <Button size="small" theme="light" icon={<IconEyeOpened />} onClick={onViewDetail} />
        </Tooltip>
        {onEditLabel && !account.label && (
          <Tooltip content="添加备注">
            <Button size="small" theme="light" icon={<IconEdit />} onClick={onEditLabel} />
          </Tooltip>
        )}
        <Tooltip content="刷新">
          <Button size="small" theme="light" icon={<IconRefresh spin={refreshing} />} onClick={onRefresh} />
        </Tooltip>
        <Tooltip content="删除">
          <Button size="small" theme="light" type="danger" icon={<IconStop />} onClick={onDelete} />
        </Tooltip>
      </div>
    </Card>
  );
}
