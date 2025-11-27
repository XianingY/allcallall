#!/bin/bash

# AllCallAll - 自定义开发客户端调试脚本
# Expo Development Client Debug Script

set -e

echo "╔════════════════════════════════════════════════════════╗"
echo "║     AllCallAll - 自定义开发客户端调试启动脚本          ║"
echo "║     Expo Development Client Debug Setup Script         ║"
echo "╚════════════════════════════════════════════════════════╝"
echo ""

# 颜色定义
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# 检查设备连接
echo -e "${YELLOW}[1/5] 检查Android设备连接...${NC}"
ADB_PATH="/Users/byzantium/Library/Android/sdk/platform-tools/adb"
DEVICE_ID=$($ADB_PATH devices | grep "device$" | awk '{print $1}' | head -1)

if [ -z "$DEVICE_ID" ]; then
    echo -e "${RED}❌ 未找到连接的Android设备${NC}"
    echo "请确保设备已连接并启用USB调试"
    exit 1
fi

echo -e "${GREEN}✅ 设备已连接: $DEVICE_ID${NC}"
echo ""

# 清理旧缓存（保护虚拟入口点文件）
echo -e "${YELLOW}[2/5] 清理Expo和Metro缓存...${NC}"
# ⚠️ 关键: 保留 .expo/.virtual-metro-entry.js 文件（Metro必需）
# 首先保存虚拟入口点文件
if [ -f ".expo/.virtual-metro-entry.js" ]; then
    cp .expo/.virtual-metro-entry.js /tmp/.virtual-metro-entry.js.bak
fi
# 清理缓存
rm -rf .expo node_modules/.cache /tmp/metro-* 2>/dev/null || true
# 恢复虚拟入口点文件
if [ -f "/tmp/.virtual-metro-entry.js.bak" ]; then
    mkdir -p .expo
    cp /tmp/.virtual-metro-entry.js.bak .expo/.virtual-metro-entry.js
    rm -f /tmp/.virtual-metro-entry.js.bak
else
    # 如果备份不存在，重新创建虚拟入口点文件
    echo "📝 重建虚拟入口点文件..."
    mkdir -p .expo
    cat > .expo/.virtual-metro-entry.js << 'ENTRY_EOF'
import { registerRootComponent } from 'expo';
import App from '../App';

registerRootComponent(App);
ENTRY_EOF
fi
echo -e "${GREEN}✅ 缓存已清理（虚拟入口点文件已保护）${NC}"
echo ""

# 配置ADB反向转发
echo -e "${YELLOW}[3/5] 配置ADB反向转发...${NC}"
$ADB_PATH -s $DEVICE_ID reverse --remove-all 2>/dev/null || true
$ADB_PATH -s $DEVICE_ID reverse tcp:8080 tcp:8080
$ADB_PATH -s $DEVICE_ID reverse tcp:8081 tcp:8081
echo -e "${GREEN}✅ ADB反向转发已配置 (8080, 8081)${NC}"
echo ""

# 清除设备应用数据
echo -e "${YELLOW}[4/5] 清除设备上的应用数据...${NC}"
$ADB_PATH -s $DEVICE_ID shell pm clear com.allcallall.mobile 2>/dev/null || true
echo -e "${GREEN}✅ 应用数据已清除${NC}"
echo ""

# 启动Metro开发服务器（自定义开发客户端模式）
echo -e "${YELLOW}[5/5] 启动Metro开发服务器 (开发客户端模式)...${NC}"
echo -e "${YELLOW}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""

npx expo start --dev-client

