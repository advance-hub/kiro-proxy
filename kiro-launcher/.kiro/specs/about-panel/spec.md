# Spec: AboutPanel 关于页面

## Requirement

提供一个"关于"面板，展示 Kiro Launcher 的激活信息（激活码、机器码、激活时间）、功能介绍列表和使用提示，帮助用户了解产品功能并管理激活状态。

## Design

### 组件结构

- `AboutPanel`：主面板组件，负责加载激活信息并渲染整体布局。
- `FeatureItem`：功能介绍子组件，展示单个功能的图标、标题和描述。

### 数据模型

```typescript
interface ActivationInfo {
  activated: boolean;   // 是否已激活
  code: string;         // 激活码
  machineId: string;    // 机器码（设备唯一标识）
  time: string;         // 激活时间（ISO 日期字符串）
}
```

### 页面布局（从上到下）

1. 标题区域 — 应用图标 + 名称 "Kiro Launcher" + 版本号
2. 激活信息卡片 — 激活码（可复制）、机器码（可复制）、激活时间
3. 功能介绍卡片 — 6 项功能列表（AI 代理、账号管理、内网穿透、实时日志、Droid 配置、OpenCode/Claude Code）
4. 使用提示卡片 — 4 条注意事项

### 交互行为

- 页面加载时通过 `wails().CheckActivation()` 获取激活信息
- 激活码和机器码旁提供"复制"按钮，点击后写入剪贴板并弹出 Toast 提示
- 加载中显示 "加载中..." 占位文本
- 获取激活信息失败时弹出错误 Toast

### 依赖

- UI 库：`@douyinfe/semi-ui`（Card, Typography, Tag, Divider, Space, Button, Toast）
- 图标：`@douyinfe/semi-icons`（IconCopy, IconTickCircle, IconLink, IconKey, IconCalendar, IconDesktop）
- 后端：Wails Go 绑定 `window.go.main.App.CheckActivation()`

## Tasks

- [x] 定义 `ActivationInfo` 接口
- [x] 实现 `wails()` 辅助函数获取 Go 绑定
- [x] 实现 `AboutPanel` 组件，挂载时加载激活信息
- [x] 实现加载态和错误处理（Toast 提示）
- [x] 渲染标题区域（应用图标 + 名称 + 版本号）
- [x] 渲染激活信息卡片（激活码、机器码、激活时间 + 复制功能）
- [x] 渲染功能介绍卡片（6 项 FeatureItem）
- [x] 渲染使用提示卡片
- [x] 实现 `FeatureItem` 子组件
- [x] 日期格式化为中文本地化格式（zh-CN）
