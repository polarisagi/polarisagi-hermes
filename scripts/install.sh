#!/bin/bash
set -e

REPO="mrlaoliai/polaris-gateway"
BIN_NAME="polaris-gateway"
INSTALL_DIR="$HOME/.polaris-gateway/bin"
PLIST_PATH="$HOME/Library/LaunchAgents/com.polaris.gateway.plist"

echo "🌌 正在安装/更新 Polaris Gateway (用户态模式)..."

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

# 停止旧服务
if [ "$OS" = "linux" ] && command -v systemctl >/dev/null; then
    if systemctl --user is-active --quiet polaris-gateway 2>/dev/null; then
        echo "🛑 正在停止运行中的 Linux 用户级服务..."
        systemctl --user stop polaris-gateway || true
    fi
    # 清理遗留的全局系统服务（如果之前用 sudo 安装过）
    if systemctl is-active --quiet polaris-gateway 2>/dev/null || [ -f "/etc/systemd/system/polaris-gateway.service" ]; then
        echo "🧹 发现旧的全局 Systemd 服务，尝试停止并清理 (可能需要 sudo 密码)..."
        sudo systemctl stop polaris-gateway 2>/dev/null || true
        sudo systemctl disable polaris-gateway 2>/dev/null || true
        sudo rm -f /etc/systemd/system/polaris-gateway.service
        sudo systemctl daemon-reload
    fi
elif [ "$OS" = "darwin" ]; then
    if launchctl list | grep -q "com.polaris.gateway"; then
        echo "🛑 正在停止运行中的 macOS 服务..."
        launchctl unload "$PLIST_PATH" 2>/dev/null || true
    fi
    pkill -f "${BIN_NAME}" 2>/dev/null || true
fi

# 安装文件
mkdir -p "${INSTALL_DIR}"
mv -f /tmp/${BIN_NAME} "${INSTALL_DIR}/${BIN_NAME}"

echo "✅ 二进制文件已安装至: ${INSTALL_DIR}/${BIN_NAME}"

# 配置服务
if [ "$OS" = "linux" ] && command -v systemctl >/dev/null; then
    echo "⚙️ 正在配置 Linux 用户级 Systemd 后台服务..."
    
    SYSTEMD_USER_DIR="$HOME/.config/systemd/user"
    mkdir -p "$SYSTEMD_USER_DIR"
    
    cat <<EOF > "$SYSTEMD_USER_DIR/polaris-gateway.service"
[Unit]
Description=Polaris AI Gateway (User Service)
After=network.target

[Service]
ExecStart=${INSTALL_DIR}/${BIN_NAME}
Restart=always
WorkingDirectory=${HOME}

[Install]
WantedBy=default.target
EOF

    # 启用 lingering 使得用户退出登录后服务仍继续运行（可选，部分系统可能需要 root 权限，不强求）
    loginctl enable-linger $USER 2>/dev/null || true

    systemctl --user daemon-reload
    systemctl --user enable polaris-gateway
    systemctl --user restart polaris-gateway
    echo "✅ Systemd 服务已启动。可通过 systemctl --user status polaris-gateway 查看状态。"

elif [ "$OS" = "darwin" ]; then
    echo "⚙️ 正在配置 macOS launchd 后台服务..."
    
    mkdir -p "$HOME/Library/LaunchAgents"

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
    <string>${HOME}</string>
</dict>
</plist>
EOF
    launchctl load "$PLIST_PATH"
    echo "✅ macOS 服务已配置。将随当前用户登录自动启动并在后台运行。"
fi

echo "🎉 安装完成！请打开浏览器访问 http://127.0.0.1:28888/dashboard 进入控制台。"
echo "💡 提示：如果需要在命令行直接使用 polaris-gateway，请将 ${INSTALL_DIR} 加入环境变量。"
