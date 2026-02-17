#!/bin/bash
# deploy.sh - 编译并部署 kiro-go 到服务器
set -e

SERVER="${1:-root@117.72.183.248}"
REMOTE_DIR="/opt/kiro-proxy"

echo "🔨 编译 Linux amd64 二进制..."
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o /tmp/kiro-go-linux .

echo "📦 上传到 $SERVER..."
scp /tmp/kiro-go-linux "$SERVER:$REMOTE_DIR/kiro-go-new"

echo "🚀 部署并重启..."
ssh "$SERVER" "chmod +x $REMOTE_DIR/kiro-go-new && mv $REMOTE_DIR/kiro-go-new $REMOTE_DIR/kiro-go-latest && systemctl restart kiro-proxy && sleep 2 && systemctl status kiro-proxy --no-pager | head -15"

echo "✅ 部署完成！"
