#!/bin/bash

# AllCallAll - Android 无线调试配置脚本
# Android Wireless Debugging Setup Script

set -e

ADB_PATH="/Users/byzantium/Library/Android/sdk/platform-tools/adb"

echo "╔══════════════════════════════════════════════════════════════╗"
echo "║      AllCallAll - Android 无线调试配置向导                   ║"
echo "║      Android Wireless Debugging Setup Wizard                 ║"
echo "╚══════════════════════════════════════════════════════════════╝"
echo ""

# 检查是否有 USB 连接的设备
echo "[1/6] 检查 USB 连接的设备..."
USB_DEVICES=$("$ADB_PATH" devices | grep -v "List of devices" | grep -v "^$" | grep "device$" | wc -l)

if [ "$USB_DEVICES" -eq 0 ]; then
    echo "❌ 未检测到 USB 连接的设备"
    echo "请先通过 USB 连接设备，然后重新运行此脚本"
    exit 1
fi

DEVICE_ID=$("$ADB_PATH" devices | grep -v "List of devices" | grep -v "^$" | grep "device$" | awk '{print $1}' | head -1)
echo "✅ 检测到设备: $DEVICE_ID"
echo ""

# 检查 Android 版本
echo "[2/6] 检查 Android 版本..."
ANDROID_SDK=$("$ADB_PATH" -s "$DEVICE_ID" shell getprop ro.build.version.sdk)
ANDROID_VERSION=$("$ADB_PATH" -s "$DEVICE_ID" shell getprop ro.build.version.release)

echo "   设备 Android 版本: Android $ANDROID_VERSION (API $ANDROID_SDK)"

if [ "$ANDROID_SDK" -lt 30 ]; then
    echo "⚠️  设备 Android 版本低于 11，将使用传统 TCP/IP 模式"
    USE_LEGACY_MODE=true
else
    echo "✅ 设备支持 Android 11+ 无线调试功能"
    USE_LEGACY_MODE=false
fi
echo ""

# 获取设备 WiFi IP
echo "[3/6] 获取设备 WiFi IP 地址..."
DEVICE_IP=$("$ADB_PATH" -s "$DEVICE_ID" shell ip addr show wlan0 2>/dev/null | grep "inet " | awk '{print $2}' | cut -d/ -f1 | head -1)

if [ -z "$DEVICE_IP" ]; then
    echo "❌ 无法获取设备 WiFi IP 地址"
    echo "请确保设备已连接到 WiFi 网络（与电脑同一网络）"
    exit 1
fi

echo "✅ 设备 WiFi IP: $DEVICE_IP"
echo ""

if [ "$USE_LEGACY_MODE" = true ]; then
    # ===== Android 10 及以下：传统 TCP/IP 模式 =====
    echo "[4/6] 使用传统 TCP/IP 模式配置无线调试..."
    
    # 启用 TCP/IP 模式（端口 5555）
    "$ADB_PATH" -s "$DEVICE_ID" tcpip 5555
    echo "   已在设备上启用 TCP/IP 模式（端口 5555）"
    sleep 2
    
    echo ""
    echo "[5/6] 连接到设备..."
    "$ADB_PATH" connect "$DEVICE_IP:5555"
    sleep 1
    
    echo ""
    echo "[6/6] 验证连接..."
    WIRELESS_DEVICES=$("$ADB_PATH" devices | grep "$DEVICE_IP" | wc -l)
    
    if [ "$WIRELESS_DEVICES" -gt 0 ]; then
        echo "✅ 无线连接成功！"
        echo ""
        echo "现在可以拔掉 USB 线了"
        echo ""
        echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
        echo "✅ 无线调试配置完成！"
        echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
        echo ""
        echo "设备信息:"
        echo "  • WiFi IP: $DEVICE_IP:5555"
        echo "  • 模式: 传统 TCP/IP"
        echo ""
        echo "断开连接:"
        echo "  $ADB_PATH disconnect $DEVICE_IP:5555"
        echo ""
        echo "重新连接:"
        echo "  $ADB_PATH connect $DEVICE_IP:5555"
    else
        echo "❌ 无线连接失败"
        echo "请检查网络连接并重试"
        exit 1
    fi
else
    # ===== Android 11+：新的无线调试功能 =====
    echo "[4/6] 配置 Android 11+ 无线调试..."
    echo ""
    echo "请在设备上执行以下操作:"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "1. 打开 设置 → 开发者选项"
    echo "2. 找到并启用 无线调试 (Wireless debugging)"
    echo "3. 点击 使用配对码配对设备"
    echo "4. 记录显示的配对信息"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
    
    read -p "已在设备上启用无线调试？(y/n): " CONFIRM
    if [ "$CONFIRM" != "y" ] && [ "$CONFIRM" != "Y" ]; then
        echo "配置已取消"
        exit 0
    fi
    
    echo ""
    echo "请输入设备显示的配对信息:"
    read -p "  配对端口（例如: 12345）: " PAIRING_PORT
    read -p "  配对码（6位数字）: " PAIRING_CODE
    
    echo ""
    echo "[5/6] 配对设备..."
    
    # 使用配对码连接
    if "$ADB_PATH" pair "$DEVICE_IP:$PAIRING_PORT" "$PAIRING_CODE"; then
        echo "✅ 配对成功！"
        sleep 1
        
        # 获取无线调试端口（通常是 5555 或显示在无线调试页面）
        echo ""
        read -p "请输入设备的无线调试端口（通常显示在 IP 地址后，例如: 5555）: " WIRELESS_PORT
        WIRELESS_PORT=${WIRELESS_PORT:-5555}
        
        echo ""
        echo "[6/6] 连接到无线调试..."
        if "$ADB_PATH" connect "$DEVICE_IP:$WIRELESS_PORT"; then
            echo "✅ 无线连接成功！"
            echo ""
            echo "现在可以拔掉 USB 线了"
            echo ""
            echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
            echo "✅ 无线调试配置完成！"
            echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
            echo ""
            echo "设备信息:"
            echo "  • WiFi IP: $DEVICE_IP:$WIRELESS_PORT"
            echo "  • 模式: Android 11+ 无线调试"
            echo ""
            echo "断开连接:"
            echo "  $ADB_PATH disconnect $DEVICE_IP:$WIRELESS_PORT"
            echo ""
            echo "重新连接（已配对设备）:"
            echo "  $ADB_PATH connect $DEVICE_IP:$WIRELESS_PORT"
        else
            echo "❌ 连接失败"
            echo "请检查端口号是否正确"
            exit 1
        fi
    else
        echo "❌ 配对失败"
        echo "请检查配对码和端口是否正确"
        exit 1
    fi
fi

echo ""
echo "验证当前连接的设备:"
"$ADB_PATH" devices
echo ""

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "💡 使用提示"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "启动开发服务器（WiFi 模式）:"
echo "  cd /Users/byzantium/github/allcall/mobile"
echo "  npm run start:dev-client:lan"
echo ""
echo "或使用现有的调试脚本（会自动配置端口转发）:"
echo "  bash scripts/dev-client-debug.sh"
echo ""
echo "⚠️  注意事项:"
echo "  • 设备和电脑必须在同一 WiFi 网络"
echo "  • WiFi 模式下无需 ADB 反向转发"
echo "  • 需要使用 LAN IP 访问服务（192.168.31.217）"
echo "  • 重启设备后需要重新连接"
echo ""
