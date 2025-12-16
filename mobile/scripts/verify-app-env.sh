#!/bin/bash

# AllCallAll APP_ENV 环境检测验证脚本
# APP_ENV Environment Detection Verification Script

echo "=================================================="
echo "🔍 APP_ENV 环境检测验证"
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

# 检查 .env.example 文件是否存在
echo "📁 检查模板文件..."
if [ -f ".env.example" ]; then
  echo "✅ .env.example 文件存在"
else
  echo "❌ .env.example 文件不存在"
fi
echo ""

# 验证 APP_ENV 值
echo "🔍 验证 APP_ENV 配置..."
if [ -f ".env" ]; then
  APP_ENV_VALUE=$(grep "APP_ENV=" .env | cut -d '=' -f2)
  echo "当前 APP_ENV 值: $APP_ENV_VALUE"
  echo ""

  case "$APP_ENV_VALUE" in
    development)
      echo "✅ 有效的开发环境配置"
      echo "📡 API 地址: http://192.168.31.217:8080"
      echo "🔌 WebSocket: ws://192.168.31.217:8080"
      ;;
    staging)
      echo "✅ 有效的测试环境配置"
      echo "📡 API 地址: http://81.68.168.207:8080"
      echo "🔌 WebSocket: ws://81.68.168.207:8080"
      ;;
    production)
      echo "✅ 有效的生产环境配置"
      echo "📡 API 地址: http://81.68.168.207"
      echo "🔌 WebSocket: ws://81.68.168.207"
      ;;
    *)
      echo "❌ 无效的 APP_ENV 值: $APP_ENV_VALUE"
      echo "有效值: development, staging, production"
      ;;
  esac
else
  echo "⚠️  未找到 .env 文件，使用默认值 (development)"
fi

echo ""
echo "=================================================="
echo "🚀 运行应用测试"
echo "=================================================="
echo ""
echo "启动命令:"
echo "  开发环境: npm start"
echo "  生产环境: APP_ENV=production npm start"
echo ""
echo "或编辑 .env 文件修改 APP_ENV 值"
echo ""

# 检查 index.ts 文件是否存在
echo "📁 检查配置文件..."
if [ -f "src/config/index.ts" ]; then
  echo "✅ src/config/index.ts 存在"
  echo ""
  echo "📄 检查关键代码..."
  if grep -q "APP_ENV" "src/config/index.ts"; then
    echo "✅ 检测到 APP_ENV 变量使用"
  else
    echo "❌ 未检测到 APP_ENV 变量使用"
  fi

  if grep -q "getAppEnv" "src/config/index.ts"; then
    echo "✅ 检测到 getAppEnv 函数"
  else
    echo "❌ 未检测到 getAppEnv 函数"
  fi
else
  echo "❌ src/config/index.ts 不存在"
fi

echo ""
echo "=================================================="
echo "📋 使用建议"
echo "=================================================="
echo ""
echo "1. 开发环境:"
echo "   设置 .env 文件: APP_ENV=development"
echo "   运行: npm start"
echo ""
echo "2. 生产环境:"
echo "   设置 .env 文件: APP_ENV=production"
echo "   运行: npm run android"
echo ""
echo "3. 快速切换:"
echo "   编辑 .env 文件，修改 APP_ENV 值"
echo "   重启应用使更改生效"
echo ""
echo "4. 查看文档:"
echo "   cat APP_ENV_USAGE.md"
echo ""
echo "=================================================="
echo "✅ 验证完成"
echo "=================================================="
