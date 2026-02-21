#!/bin/bash
# Kiro Launcher Linux 安装脚本
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
INSTALL_DIR="/usr/local/bin"
DESKTOP_DIR="/usr/share/applications"
ICON_DIR="/usr/share/icons/hicolor/256x256/apps"

echo "=== Kiro Launcher 安装程序 ==="
echo ""

# 检查是否以 root 运行
if [ "$(id -u)" -ne 0 ]; then
    echo "需要 root 权限安装，正在使用 sudo..."
    exec sudo "$0" "$@"
fi

# 检查运行时依赖
echo "[1/4] 检查依赖..."
missing=""
dpkg -l libgtk-3-0 &>/dev/null || missing="$missing libgtk-3-0"
dpkg -l libwebkit2gtk-4.0-37 &>/dev/null || missing="$missing libwebkit2gtk-4.0-37"

if [ -n "$missing" ]; then
    echo "  安装缺失的依赖:$missing"
    apt-get update -qq && apt-get install -y $missing
else
    echo "  依赖已满足 ✓"
fi

# 安装二进制
echo "[2/4] 安装二进制文件..."
install -m 755 "$SCRIPT_DIR/kiro-launcher" "$INSTALL_DIR/kiro-launcher"
echo "  → $INSTALL_DIR/kiro-launcher ✓"

# 安装图标
echo "[3/4] 安装图标..."
mkdir -p "$ICON_DIR"
if [ -f "$SCRIPT_DIR/appicon.png" ]; then
    cp "$SCRIPT_DIR/appicon.png" "$ICON_DIR/kiro-launcher.png"
    echo "  → $ICON_DIR/kiro-launcher.png ✓"
else
    echo "  ⚠ 未找到图标文件，跳过"
fi

# 安装桌面快捷方式
echo "[4/4] 安装桌面快捷方式..."
mkdir -p "$DESKTOP_DIR"
cp "$SCRIPT_DIR/kiro-launcher.desktop" "$DESKTOP_DIR/kiro-launcher.desktop"
update-desktop-database "$DESKTOP_DIR" 2>/dev/null || true
echo "  → $DESKTOP_DIR/kiro-launcher.desktop ✓"

echo ""
echo "=== 安装完成! ==="
echo "运行方式："
echo "  1. 从应用菜单启动 'Kiro Launcher'"
echo "  2. 或终端运行: kiro-launcher"
