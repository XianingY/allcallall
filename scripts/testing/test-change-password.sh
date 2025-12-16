#!/bin/bash

# 密码修改功能测试脚本
# Change Password Feature Test Script

set -e

# 配置
API_HOST="${API_HOST:-http://localhost:8080}"
API_URL="$API_HOST/api/v1"

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo "======================================"
echo "🔐 密码修改功能 API 测试"
echo "======================================"
echo ""

# 1. 注册新用户
echo -e "${YELLOW}1. 注册测试用户...${NC}"
TIMESTAMP=$(date +%s)
TEST_EMAIL="test.changepass.$TIMESTAMP@example.com"
TEST_PASSWORD="TestPass123"

REGISTER_RESPONSE=$(curl -s -X POST "$API_URL/auth/register" \
  -H "Content-Type: application/json" \
  -d "{
    \"email\": \"$TEST_EMAIL\",
    \"password\": \"$TEST_PASSWORD\",
    \"display_name\": \"Test User\"
  }")

echo "Response: $REGISTER_RESPONSE"
TOKEN=$(echo $REGISTER_RESPONSE | grep -o '"token":"[^"]*' | cut -d'"' -f4)

if [ -z "$TOKEN" ]; then
  echo -e "${RED}❌ 注册失败，无法获取 token${NC}"
  exit 1
fi

echo -e "${GREEN}✓ 注册成功${NC}"
echo "  用户: $TEST_EMAIL"
echo "  Token: ${TOKEN:0:20}..."
echo ""

# 2. 测试有效的密码修改
echo -e "${YELLOW}2. 测试有效的密码修改...${NC}"
NEW_PASSWORD="NewPass456"

CHANGE_RESPONSE=$(curl -s -X POST "$API_URL/users/change-password" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "{
    \"old_password\": \"$TEST_PASSWORD\",
    \"new_password\": \"$NEW_PASSWORD\",
    \"confirm_password\": \"$NEW_PASSWORD\"
  }")

echo "Response: $CHANGE_RESPONSE"

if echo "$CHANGE_RESPONSE" | grep -q "password changed successfully"; then
  echo -e "${GREEN}✓ 密码修改成功${NC}"
else
  echo -e "${RED}❌ 密码修改失败${NC}"
  exit 1
fi
echo ""

# 3. 测试错误的旧密码
echo -e "${YELLOW}3. 测试错误的旧密码...${NC}"
ERROR_RESPONSE=$(curl -s -X POST "$API_URL/users/change-password" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "{
    \"old_password\": \"WrongPassword123\",
    \"new_password\": \"AnotherPass789\",
    \"confirm_password\": \"AnotherPass789\"
  }")

echo "Response: $ERROR_RESPONSE"

if echo "$ERROR_RESPONSE" | grep -q "invalid old password"; then
  echo -e "${GREEN}✓ 正确地拒绝了错误的旧密码${NC}"
else
  echo -e "${RED}❌ 没有正确处理错误的旧密码${NC}"
fi
echo ""

# 4. 测试密码强度不足 (缺少数字)
echo -e "${YELLOW}4. 测试密码强度不足 (缺少数字)...${NC}"
WEAK_RESPONSE=$(curl -s -X POST "$API_URL/users/change-password" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "{
    \"old_password\": \"$NEW_PASSWORD\",
    \"new_password\": \"OnlyLetters\",
    \"confirm_password\": \"OnlyLetters\"
  }")

echo "Response: $WEAK_RESPONSE"

if echo "$WEAK_RESPONSE" | grep -q "letters and numbers"; then
  echo -e "${GREEN}✓ 正确地拒绝了强度不足的密码${NC}"
else
  echo -e "${RED}❌ 没有正确验证密码强度${NC}"
fi
echo ""

# 5. 测试密码过短
echo -e "${YELLOW}5. 测试密码过短...${NC}"
SHORT_RESPONSE=$(curl -s -X POST "$API_URL/users/change-password" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "{
    \"old_password\": \"$NEW_PASSWORD\",
    \"new_password\": \"Pass1\",
    \"confirm_password\": \"Pass1\"
  }")

echo "Response: $SHORT_RESPONSE"

if echo "$SHORT_RESPONSE" | grep -q "at least 8 characters"; then
  echo -e "${GREEN}✓ 正确地拒绝了过短的密码${NC}"
else
  echo -e "${RED}❌ 没有正确验证密码长度${NC}"
fi
echo ""

# 6. 测试特殊字符
echo -e "${YELLOW}6. 测试包含特殊字符的密码...${NC}"
SPECIAL_RESPONSE=$(curl -s -X POST "$API_URL/users/change-password" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "{
    \"old_password\": \"$NEW_PASSWORD\",
    \"new_password\": \"Pass@123\",
    \"confirm_password\": \"Pass@123\"
  }")

echo "Response: $SPECIAL_RESPONSE"

if echo "$SPECIAL_RESPONSE" | grep -q "special characters"; then
  echo -e "${GREEN}✓ 正确地拒绝了包含特殊字符的密码${NC}"
else
  echo -e "${RED}❌ 没有正确处理特殊字符验证${NC}"
fi
echo ""

# 7. 测试密码不匹配
echo -e "${YELLOW}7. 测试密码不匹配...${NC}"
MISMATCH_RESPONSE=$(curl -s -X POST "$API_URL/users/change-password" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "{
    \"old_password\": \"$NEW_PASSWORD\",
    \"new_password\": \"NewPass111\",
    \"confirm_password\": \"NewPass222\"
  }")

echo "Response: $MISMATCH_RESPONSE"

if echo "$MISMATCH_RESPONSE" | grep -q "do not match"; then
  echo -e "${GREEN}✓ 正确地检测到密码不匹配${NC}"
else
  echo -e "${RED}❌ 没有正确检测密码不匹配${NC}"
fi
echo ""

echo "======================================"
echo -e "${GREEN}✓ 所有测试完成!${NC}"
echo "======================================"
