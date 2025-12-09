#!/bin/bash

# AllCallAll Android 真机调试启动脚本
# Android Real Device Debug Setup Script

set -e

# 配置
ADB_PATH="/Users/byzantium/Library/Android/sdk/platform-tools/adb"
BACKEND_PORT=8080
METRO_PORT=8081
MOBILE_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )/.." && pwd )/mobile"

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}╔════════════════════════════════════════════════════════════════╗${NC}"
echo -e "${BLUE}║        🚀 AllCallAll Android 真机调试启动脚本                ║${NC}"
echo -e "${BLUE}╚════════════════════════════════════════════════════════════════╝${NC}"
echo ""

# ============================================================================
# 步骤 1: 检查 ADB 是否可用
# ============================================================================
echo -e "${YELLOW}[1/5] 检查 ADB 环境...${NC}"
if [ ! -f "$ADB_PATH" ]; then
  echo -e "${RED}❌ ADB 不存在于: $ADB_PATH${NC}"
  echo "请检查 Android SDK 安装路径"
  exit 1
fi
echo -e "${GREEN}✓ ADB 路径正确: $ADB_PATH${NC}"
echo ""

# ============================================================================
# 步骤 2: 检查设备连接
# ============================================================================
echo -e "${YELLOW}[2/5] 检查 Android 设备连接...${NC}"
DEVICES=$("$ADB_PATH" devices | grep -v "List of devices" | grep -v "^$")

if [ -z "$DEVICES" ]; then
  echo -e "${RED}❌ 未检测到任何 Android 设备${NC}"
  echo -e "${YELLOW}请确保:${NC}"
  echo "  1. Android 设备已通过 USB 连接"
  echo "  2. 设备已启用 USB 调试模式"
  echo "  3. 已授予计算机访问权限（在设备上允许）"
  echo ""
  echo -e "${YELLOW}已连接的设备列表:${NC}"
  "$ADB_PATH" devices
  exit 1
fi

DEVICE_ID=$(echo "$DEVICES" | head -1 | awk '{print $1}')
DEVICE_STATE=$(echo "$DEVICES" | head -1 | awk '{print $2}')

if [ "$DEVICE_STATE" != "device" ]; then
  echo -e "${RED}❌ 设备未就绪 (状态: $DEVICE_STATE)${NC}"
  echo "请在设备上授予 USB 调试权限"
  "$ADB_PATH" devices
  exit 1
fi

echo -e "${GREEN}✓ 设备已连接: $DEVICE_ID${NC}"
echo ""

# ============================================================================
# 步骤 3: 检查端口占用
# ============================================================================
echo -e "${YELLOW}[3/5] 检查端口占用情况...${NC}"

# 检查 8080 端口
if lsof -i :$BACKEND_PORT >/dev/null 2>&1; then
  echo -e "${RED}⚠️  端口 $BACKEND_PORT 已被占用${NC}"
  echo "请停止占用该端口的进程或修改后端端口"
  lsof -i :$BACKEND_PORT
else
  echo -e "${GREEN}✓ 端口 $BACKEND_PORT 可用${NC}"
fi

# 检查 8081 端口
if lsof -i :$METRO_PORT >/dev/null 2>&1; then
  echo -e "${YELLOW}⚠️  端口 $METRO_PORT 已被占用${NC}"
  echo "将由 Metro 自动重新分配"
else
  echo -e "${GREEN}✓ 端口 $METRO_PORT 可用${NC}"
fi
echo ""

# ============================================================================
# 步骤 4: 配置 ADB 反向转发
# ============================================================================
echo -e "${YELLOW}[4/5] 配置 ADB 反向端口转发...${NC}"

echo "设置: tcp:$BACKEND_PORT -> tcp:$BACKEND_PORT (后端 API)"
"$ADB_PATH" -s "$DEVICE_ID" reverse tcp:$BACKEND_PORT tcp:$BACKEND_PORT
echo -e "${GREEN}✓ 后端 API 转发配置成功${NC}"

echo "设置: tcp:$METRO_PORT -> tcp:$METRO_PORT (Metro 开发服务器)"
"$ADB_PATH" -s "$DEVICE_ID" reverse tcp:$METRO_PORT tcp:$METRO_PORT
echo -e "${GREEN}✓ Metro 开发服务器转发配置成功${NC}"

# 验证转发配置
echo -e "${YELLOW}验证转发配置...${NC}"
REVERSE_LIST=$("$ADB_PATH" -s "$DEVICE_ID" reverse --list)
echo -e "${GREEN}当前转发规则:${NC}"
echo "$REVERSE_LIST"
echo ""

# ============================================================================
# 步骤 5: 启动 Expo Development Client
# ============================================================================
echo -e "${YELLOW}[5/5] 启动 Expo Development Client...${NC}"
echo ""
echo -e "${BLUE}进入移动端目录...${NC}"
cd "$MOBILE_DIR"

echo -e "${BLUE}启动 Metro 开发服务器和 Expo CLI...${NC}"
echo -e "${YELLOW}该服务将运行在前台，请勿关闭此终端窗口。${NC}"
echo ""
echo -e "${BLUE}启动命令: npx expo start --dev-client${NC}"
echo ""

npx expo start --dev-client

# ============================================================================
# 清理（如果脚本被中断）
# ============================================================================
trap "echo -e \"\n${YELLOW}清理资源...${NC}\"; exit 0" SIGINT SIGTERM

