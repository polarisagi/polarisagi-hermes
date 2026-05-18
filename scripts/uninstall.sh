#!/bin/bash

BIN_NAME="polaris-gateway"
INSTALL_DIR="$HOME/.polaris-gateway/bin"

echo "🗑️ 正在卸载 Polaris Gateway (用户态模式)..."

OS=$(uname -s | tr '[:upper:]' '[:lower:]')

# Stop and remove services
if [ "$OS" = "linux" ] && command -v systemctl >/dev/null; then
    if systemctl --user is-active --quiet polaris-gateway 2>/dev/null; then
        echo "⚙️ 停止用户级 systemd 服务..."
        systemctl --user stop polaris-gateway
    fi
    if systemctl --user is-enabled --quiet polaris-gateway 2>/dev/null; then
        echo "⚙️ 禁用用户级 systemd 服务..."
        systemctl --user disable polaris-gateway
    fi
    
    SYSTEMD_USER_DIR="$HOME/.config/systemd/user"
    if [ -f "$SYSTEMD_USER_DIR/polaris-gateway.service" ]; then
        echo "⚙️ 删除用户级 systemd 配置文件..."
        rm "$SYSTEMD_USER_DIR/polaris-gateway.service"
        systemctl --user daemon-reload
    fi
elif [ "$OS" = "darwin" ]; then
    PLIST_PATH="$HOME/Library/LaunchAgents/com.polaris.gateway.plist"
    if [ -f "$PLIST_PATH" ]; then
        echo "⚙️ 卸载 macOS launchd 服务..."
        launchctl unload "$PLIST_PATH" 2>/dev/null
        rm "$PLIST_PATH"
    fi
fi

# Remove binary
if [ -f "${INSTALL_DIR}/${BIN_NAME}" ]; then
    echo "🗑️ 删除二进制文件: ${INSTALL_DIR}/${BIN_NAME}"
    rm "${INSTALL_DIR}/${BIN_NAME}"
fi

echo ""
echo "⚠️  注意: 您的数据库和配置数据仍保留在 ~/.polaris-gateway 目录中。"
echo "如果您想彻底清理所有数据（这会删除所有配置和账单记录），请手动执行："
echo "rm -rf ~/.polaris-gateway"
echo ""
echo "✅ 卸载完成！"
