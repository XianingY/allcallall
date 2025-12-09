#!/bin/bash

# AllCallAll - 快速无线调试配对脚本
# Quick Wireless Debugging Pairing Script

ADB_PATH="/Users/byzantium/Library/Android/sdk/platform-tools/adb"

echo "╔══════════════════════════════════════════════════════════════╗"
echo "║        AllCallAll - 无线调试配对工具                         ║"
echo "║        Wireless Debugging Pairing Tool                       ║"
echo "╚══════════════════════════════════════════════════════════════╝"
echo ""

# 检查 ADB
if ! command -v "$ADB_PATH" &> /dev/null; then
    echo "❌ ADB 命令未找到: $ADB_PATH"
    exit 1
fi

echo "📋 请输入 Android 设备显示的配对信息"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

# 输入设备 IP 和端口
read -p "🔗 设备 IP 和端口（例如: 192.168.31.12:33449）: " DEVICE_ADDRESS

if [ -z "$DEVICE_ADDRESS" ]; then
    echo "❌ IP 和端口不能为空"
    exit 1
fi

# 输入配对码
read -p "🔐 配对码（6位数字，例如: 083678）: " PAIRING_CODE

if [ -z "$PAIRING_CODE" ] || [ ${#PAIRING_CODE} -ne 6 ]; then
    echo "❌ 配对码必须是 6 位数字"
    exit 1
fi

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "正在配对..."
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

# 执行配对
PAIR_OUTPUT=$("$ADB_PATH" pair "$DEVICE_ADDRESS" "$PAIRING_CODE" 2>&1)
PAIR_RESULT=$?

echo "$PAIR_OUTPUT"
echo ""

if [ $PAIR_RESULT -eq 0 ]; then
    echo "✅ 配对成功！"
    echo ""
    
    # 等待设备准备好
    sleep 2
    
    # 提取 IP（去掉端口）
    DEVICE_IP=$(echo "$DEVICE_ADDRESS" | cut -d':' -f1)
    
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "正在连接到设备..."
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
    
    # 尝试连接到设备
    CONNECT_OUTPUT=$("$ADB_PATH" connect "$DEVICE_IP:5555" 2>&1)
    CONNECT_RESULT=$?
    
    echo "$CONNECT_OUTPUT"
    echo ""
    
    if [ $CONNECT_RESULT -eq 0 ]; then
        # 等待连接建立
        sleep 1
        
        echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
        echo "✅ 连接成功！验证设备列表..."
        echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
        echo ""
        
        "$ADB_PATH" devices
        echo ""
        
        # 检查连接状态
        DEVICE_STATUS=$("$ADB_PATH" devices | grep "$DEVICE_IP" | awk '{print $2}')
        
        if [ "$DEVICE_STATUS" = "device" ]; then
            echo "✅ 无线调试已正常连接！"
            echo ""
            echo "设备信息:"
            echo "  • IP 地址: $DEVICE_IP"
            echo "  • 端口: 5555"
            echo "  • 状态: device (✅ 已连接)"
            echo ""
            echo "现在可以:"
            echo "  1. 拔掉 USB 线"
            echo "  2. 设备通过 WiFi 继续连接"
            echo "  3. 启动开发服务器: npm run start:dev-client:lan"
            echo ""
        elif [ "$DEVICE_STATUS" = "unauthorized" ]; then
            echo "⚠️  设备状态: unauthorized（未授权）"
            echo ""
            echo "请在 Android 设备上:"
            echo "  1. 查看屏幕上的通知或弹窗"
            echo "  2. 点击 '允许' 或 '信任此电脑'"
            echo "  3. 然后运行: adb devices"
            echo ""
        else
            echo "❌ 连接状态: $DEVICE_STATUS"
            echo "请检查 WiFi 连接和防火墙设置"
        fi
    else
        echo "❌ 连接失败"
        echo ""
        echo "可能的原因:"
        echo "  • 设备和电脑不在同一 WiFi 网络"
        echo "  • 防火墙阻止了连接"
        echo "  • 设备进入了省电模式"
        echo ""
        echo "排查步骤:"
        echo "  1. 检查网络: ping $DEVICE_IP"
        echo "  2. 检查防火墙"
        echo "  3. 重新启用无线调试"
        echo "  4. 重新运行此脚本"
    fi
else
    echo "❌ 配对失败"
    echo ""
    echo "可能的原因:"
    echo "  • 配对码已过期（有效期约 10 分钟）"
    echo "  • 配对码格式错误"
    echo "  • 设备 IP/端口不正确"
    echo ""
    echo "解决方案:"
    echo "  1. 在 Android 设备上重新生成配对码"
    echo "  2. 立即运行此脚本"
    echo "  3. 输入新的配对码和 IP"
fi

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "💡 常用命令"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "查看连接: adb devices"
echo "拔掉 USB 后: adb connect 192.168.31.12:5555"
echo "断开连接: adb disconnect 192.168.31.12:5555"
echo "启动 WiFi 开发服务器: npm run start:dev-client:lan"
echo ""
