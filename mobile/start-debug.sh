#!/bin/bash

# 快速启动开发调试的脚本
# 解决 localhost 问题的完整方案

echo "🚀 启动 AllCallAll 开发环境..."
echo ""

# 1. 配置环境变量，明确指定开发服务器地址
export REACT_NATIVE_PACKAGER_HOSTNAME=192.168.1.30
export RCT_METRO_PORT=8081
export EAS_BUILD_SKIP_CLEANUP=true

echo "✅ 环境变量设置："
echo "   REACT_NATIVE_PACKAGER_HOSTNAME=192.168.1.30"
echo "   RCT_METRO_PORT=8081"
echo ""

# 2. 设置 ADB 反向端口转发（仅当有USB连接时）
# 使用完整的ADB路径
ADB_PATH="/Users/byzantium/Library/Android/sdk/platform-tools/adb"
DEVICE_ID=$($ADB_PATH devices | grep -E "^\S+\s+device$" | awk '{print $1}' | head -n 1)

if [ -z "$DEVICE_ID" ]; then
  echo "⚠️  未检测到USB连接的Android设备"
  echo "   请连接USB线或运行: adb connect <IP>:5555"
  echo ""
else
  echo "📱 检测到设备: $DEVICE_ID"
  echo "   设置 ADB 反向端口转发..."
  $ADB_PATH -s "$DEVICE_ID" reverse tcp:8081 tcp:8081
  $ADB_PATH -s "$DEVICE_ID" reverse tcp:8080 tcp:8080
  echo "   ✅ 端口转发已配置"
  echo ""
fi

# 3. 启动 Metro 开发服务器
echo "🎯 启动 Metro Bundler..."
echo "   Metro 将绑定到: http://192.168.1.30:8081"
echo ""

npm start
