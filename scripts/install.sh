#!/bin/bash
set -e

REPO="mrlaoliai/polaris-gateway"
BIN_NAME="polaris-gateway"
INSTALL_DIR="/usr/local/bin"

echo "🌌 正在安装/更新 Polaris Gateway..."

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

if [ "$ARCH" = "x86_64" ]; then
    ARCH="amd64"
elif [ "$ARCH" = "aarch64" ] || [ "$ARCH" = "arm64" ]; then
    ARCH="arm64"
else
    echo "❌ 不支持的架构: $ARCH"
    exit 1
fi

DL_URL="https://github.com/${REPO}/releases/latest/download/${BIN_NAME}-${OS}-${ARCH}"

echo "⬇️ 正在从 GitHub 下载最新版本: $DL_URL"
curl -sSL -f -o /tmp/${BIN_NAME} "$DL_URL" || {
    echo "❌ 下载失败。请检查网络或确认仓库是否已发布 Release。"
    exit 1
}

chmod +x /tmp/${BIN_NAME}

if [ "$OS" = "linux" ] && command -v systemctl >/dev/null; then
    if systemctl is-active --quiet polaris-gateway 2>/dev/null; then
        echo "🛑 正在停止运行中的 Linux 服务..."
        sudo systemctl stop polaris-gateway || true
    fi
elif [ "$OS" = "darwin" ]; then
    PLIST_PATH="$HOME/Library/LaunchAgents/com.polaris.gateway.plist"
    if launchctl list | grep -q "com.polaris.gateway"; then
        echo "🛑 正在停止运行中的 macOS 服务..."
        launchctl unload "$PLIST_PATH" 2>/dev/null || true
    fi
    pkill -f "${BIN_NAME}" 2>/dev/null || true
fi

sudo mkdir -p ${INSTALL_DIR}
sudo mv -f /tmp/${BIN_NAME} ${INSTALL_DIR}/${BIN_NAME}

echo "✅ 二进制文件已安装至: ${INSTALL_DIR}/${BIN_NAME}"

# Setup Service
if [ "$OS" = "linux" ] && command -v systemctl >/dev/null; then
    echo "⚙️ 正在配置 systemd 后台服务..."
    cat <<EOF | sudo tee /etc/systemd/system/polaris-gateway.service > /dev/null
[Unit]
Description=Polaris AI Gateway
After=network.target

[Service]
ExecStart=${INSTALL_DIR}/${BIN_NAME}
Restart=always
User=root
WorkingDirectory=/root

[Install]
WantedBy=multi-user.target
EOF
    sudo systemctl daemon-reload
    sudo systemctl enable polaris-gateway
    sudo systemctl restart polaris-gateway
    echo "✅ Systemd 服务已启动。可通过 systemctl status polaris-gateway 查看状态。"
elif [ "$OS" = "darwin" ]; then
    echo "⚙️ 正在配置 macOS launchd 后台服务..."
    PLIST_PATH="$HOME/Library/LaunchAgents/com.polaris.gateway.plist"
    
    # 卸载旧服务以支持无缝更新重启
    if launchctl list | grep -q "com.polaris.gateway"; then
        launchctl unload "$PLIST_PATH" 2>/dev/null || true
    fi

    cat <<EOF > "$PLIST_PATH"
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.polaris.gateway</string>
    <key>ProgramArguments</key>
    <array>
        <string>${INSTALL_DIR}/${BIN_NAME}</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>WorkingDirectory</key>
    <string>$HOME</string>
</dict>
</plist>
EOF
    launchctl load "$PLIST_PATH"
    echo "✅ macOS 服务已配置。将随当前用户登录自动启动并在后台运行。"
fi

echo "🎉 安装完成！请打开浏览器访问 http://127.0.0.1:28888/dashboard 进入控制台。"
