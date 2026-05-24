#!/bin/bash

# AllCallAll Expo Public 环境检测验证脚本
# Expo Public environment verification script

echo "=================================================="
echo "🔍 EXPO_PUBLIC_* 环境检测验证"
echo "=================================================="
echo ""

# 检查 .env 文件是否存在
echo "📁 检查配置文件..."
if [ -f ".env" ]; then
  echo "✅ .env 文件存在"
  echo ""
  echo "📄 .env 文件内容:"
  cat .env
  echo ""
else
  echo "⚠️  .env 文件不存在，使用默认值"
  echo ""
fi

echo "🔍 检查 EXPO_PUBLIC 配置..."
HTTP_VALUE="${EXPO_PUBLIC_API_HTTP:-}"
WS_VALUE="${EXPO_PUBLIC_API_WS:-}"
FORCE_TLS_VALUE="${EXPO_PUBLIC_FORCE_TLS:-}"

if [ -f ".env" ]; then
  [ -z "$HTTP_VALUE" ] && HTTP_VALUE=$(grep "^EXPO_PUBLIC_API_HTTP=" .env | cut -d '=' -f2-)
  [ -z "$WS_VALUE" ] && WS_VALUE=$(grep "^EXPO_PUBLIC_API_WS=" .env | cut -d '=' -f2-)
  [ -z "$FORCE_TLS_VALUE" ] && FORCE_TLS_VALUE=$(grep "^EXPO_PUBLIC_FORCE_TLS=" .env | cut -d '=' -f2-)
fi

echo "EXPO_PUBLIC_API_HTTP: ${HTTP_VALUE:-<默认 http://127.0.0.1:8080>}"
echo "EXPO_PUBLIC_API_WS: ${WS_VALUE:-<默认 ws://127.0.0.1:8080>}"
echo "EXPO_PUBLIC_FORCE_TLS: ${FORCE_TLS_VALUE:-0}"

echo ""
echo "=================================================="
echo "🚀 运行应用测试"
echo "=================================================="
echo ""
echo "启动命令:"
echo "  本地默认: npm start"
echo "  指定接口: EXPO_PUBLIC_API_HTTP=http://10.0.2.2:8080 EXPO_PUBLIC_API_WS=ws://10.0.2.2:8080 npm start"
echo ""

# 检查 index.ts 文件是否存在
echo "📁 检查配置文件..."
if [ -f "src/config/index.ts" ]; then
  echo "✅ src/config/index.ts 存在"
  echo ""
  echo "📄 检查关键代码..."
  if grep -q "EXPO_PUBLIC_API_HTTP" "src/config/index.ts"; then
    echo "✅ 检测到 EXPO_PUBLIC_API_HTTP 变量使用"
  else
    echo "❌ 未检测到 EXPO_PUBLIC_API_HTTP 变量使用"
  fi

  if grep -q "DEFAULT_HTTP_HOST" "src/config/index.ts"; then
    echo "✅ 检测到统一默认配置"
  else
    echo "❌ 未检测到统一默认配置"
  fi
else
  echo "❌ src/config/index.ts 不存在"
fi

echo ""
echo "=================================================="
echo "📋 使用建议"
echo "=================================================="
echo ""
echo "1. 本地开发:"
echo "   直接运行: npm start"
echo "   默认连接: http://127.0.0.1:8080 / ws://127.0.0.1:8080"
echo ""
echo "2. 指定后端:"
echo "   设置 .env 或 shell:"
echo "   EXPO_PUBLIC_API_HTTP=http://<host>:8080"
echo "   EXPO_PUBLIC_API_WS=ws://<host>:8080"
echo ""
echo "3. 强制 TLS:"
echo "   EXPO_PUBLIC_FORCE_TLS=1"
echo ""
echo "4. 查看文档:"
echo "   cat ../docs/mobile/setup/app-env-usage.md"
echo ""
echo "=================================================="
echo "✅ 验证完成"
echo "=================================================="
