import React, { useState, useEffect } from "react";
import { Card, Typography, Tag, Divider, Space, Button, Toast } from "@douyinfe/semi-ui";
import { IconCopy, IconTickCircle, IconLink, IconKey, IconCalendar, IconDesktop } from "@douyinfe/semi-icons";

const { Text, Title } = Typography;

interface ActivationInfo {
  activated: boolean;
  code: string;
  machineId: string;
  time: string;
}

const wails = () => {
  if (!window.go?.main?.App) throw new Error("Wails runtime 尚未就绪");
  return window.go.main.App;
};

export default function AboutPanel() {
  const [activationInfo, setActivationInfo] = useState<ActivationInfo | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    loadActivationInfo();
  }, []);

  const loadActivationInfo = async () => {
    try {
      const data = await wails().CheckActivation();
      setActivationInfo(data);
    } catch (e) {
      Toast.error({ content: "获取激活信息失败" });
    } finally {
      setLoading(false);
    }
  };

  const copyToClipboard = (text: string, label: string) => {
    navigator.clipboard.writeText(text);
    Toast.success({ content: `已复制${label}` });
  };

  const formatDate = (dateStr: string) => {
    if (!dateStr) return "-";
    const date = new Date(dateStr);
    return date.toLocaleString("zh-CN", {
      year: "numeric",
      month: "2-digit",
      day: "2-digit",
      hour: "2-digit",
      minute: "2-digit",
    });
  };

  if (loading) {
    return (
      <div style={{ padding: "20px 24px 32px" }}>
        <Text type="tertiary">加载中...</Text>
      </div>
    );
  }

  return (
    <div style={{ padding: "20px 24px 32px" }}>
      {/* 标题 */}
      <div style={{ marginBottom: 24 }}>
        <div style={{ display: "flex", alignItems: "center", gap: 12, marginBottom: 8 }}>
          <div style={{
            width: 48,
            height: 48,
            borderRadius: 12,
            background: "linear-gradient(135deg, #3370ff, #5b8def)",
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
            color: "#fff",
            fontWeight: 700,
            fontSize: 24,
          }}>
            K
          </div>
          <div>
            <Title heading={3} style={{ margin: 0 }}>Kiro Launcher</Title>
            <Text type="tertiary" size="small">AI 代理工具 · 版本 1.0.0</Text>
          </div>
        </div>
      </div>

      {/* 激活信息卡片 */}
      <Card
        title={
          <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
            <IconTickCircle style={{ color: "#52c41a" }} />
            <Text strong>激活信息</Text>
          </div>
        }
        bodyStyle={{ padding: "20px" }}
        style={{ borderRadius: 12, marginBottom: 16 }}
      >
        <Space vertical spacing={16} style={{ width: "100%" }}>
          {/* 激活码 */}
          <div>
            <div style={{ display: "flex", alignItems: "center", gap: 8, marginBottom: 8 }}>
              <IconKey style={{ color: "var(--semi-color-text-2)" }} />
              <Text type="secondary" size="small">激活码</Text>
            </div>
            <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
              <Tag
                size="large"
                color="blue"
                style={{
                  fontFamily: "monospace",
                  fontSize: 16,
                  fontWeight: 600,
                  padding: "8px 16px",
                }}
              >
                {activationInfo?.code || "-"}
              </Tag>
              <Button
                size="small"
                theme="borderless"
                icon={<IconCopy />}
                onClick={() => copyToClipboard(activationInfo?.code || "", "激活码")}
              >
                复制
              </Button>
            </div>
          </div>

          <Divider margin="0" />

          {/* 机器码 */}
          <div>
            <div style={{ display: "flex", alignItems: "center", gap: 8, marginBottom: 8 }}>
              <IconDesktop style={{ color: "var(--semi-color-text-2)" }} />
              <Text type="secondary" size="small">机器码</Text>
            </div>
            <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
              <Text
                code
                style={{
                  fontSize: 12,
                  wordBreak: "break-all",
                  padding: "8px 12px",
                  background: "var(--semi-color-fill-0)",
                  borderRadius: 6,
                  flex: 1,
                }}
              >
                {activationInfo?.machineId || "-"}
              </Text>
              <Button
                size="small"
                theme="borderless"
                icon={<IconCopy />}
                onClick={() => copyToClipboard(activationInfo?.machineId || "", "机器码")}
              >
                复制
              </Button>
            </div>
          </div>

          <Divider margin="0" />

          {/* 激活时间 */}
          <div>
            <div style={{ display: "flex", alignItems: "center", gap: 8, marginBottom: 8 }}>
              <IconCalendar style={{ color: "var(--semi-color-text-2)" }} />
              <Text type="secondary" size="small">激活时间</Text>
            </div>
            <Text>{formatDate(activationInfo?.time || "")}</Text>
          </div>
        </Space>
      </Card>

      {/* 功能介绍卡片 */}
      <Card
        title={<Text strong>功能介绍</Text>}
        bodyStyle={{ padding: "16px 24px" }}
        style={{ borderRadius: 12, marginBottom: 16 }}
      >
        <FeatureItem icon="🚀" title="AI 代理服务" description="支持 OpenAI、Claude、Gemini 等多种 AI 模型的代理转发，统一管理 API 密钥" />
        <Divider margin="16px" />
        <FeatureItem icon="🔐" title="账号管理" description="安全存储和管理多个 AI 服务账号，支持快速切换和批量导入" />
        <Divider margin="16px" />
        <FeatureItem icon="🌐" title="内网穿透" description="通过 FRP 将本地代理暴露到公网，支持 HTTP 和 TCP 两种模式" />
        <Divider margin="16px" />
        <FeatureItem icon="📊" title="实时日志" description="查看代理请求日志，监控 API 调用情况和错误信息" />
        <Divider margin="16px" />
        <FeatureItem icon="⚙️" title="Droid 配置" description="自定义 Droid 服务配置，支持多种 AI 模型和参数调整" />
        <Divider margin="16px" />
        <FeatureItem icon="🔗" title="OpenCode / Claude Code" description="专为代码编辑器优化的 AI 代理配置，提升开发效率" />
      </Card>

      {/* 使用提示卡片 */}
      <Card
        bodyStyle={{ padding: "20px" }}
        style={{ borderRadius: 12, background: "var(--semi-color-fill-0)" }}
      >
        <Text strong size="small" style={{ display: "block", marginBottom: 12 }}>
          使用提示
        </Text>
        <Space vertical spacing={8}>
          <Text type="tertiary" size="small">
            • 请妥善保管您的激活码，切勿泄露给他人
          </Text>
          <Text type="tertiary" size="small">
            • 激活码与机器码绑定，更换设备需要重新激活
          </Text>
          <Text type="tertiary" size="small">
            • 内网穿透功能需要额外授权，请联系管理员开通
          </Text>
          <Text type="tertiary" size="small">
            • 如遇问题，请联系技术支持并提供机器码
          </Text>
        </Space>
      </Card>
    </div>
  );
}

interface FeatureItemProps {
  icon: string;
  title: string;
  description: string;
}

function FeatureItem({ icon, title, description }: FeatureItemProps) {
  return (
    <div style={{ display: "flex", gap: 16, alignItems: "center", justifyContent: "left" }}>
      <div
        style={{
          fontSize: 28,
          width: 56,
          height: 56,
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          background: "var(--semi-color-fill-0)",
          borderRadius: 12,
          flexShrink: 0,
        }}
      >
        {icon}
      </div>
      <div style={{ flex: 1 }}>
        <Text strong style={{ display: "block", marginBottom: 4, fontSize: 15 }}>
          {title}
        </Text>
        <Text type="tertiary" size="small" style={{ lineHeight: 1.6 }}>
          {description}
        </Text>
      </div>
    </div>
  );
}
