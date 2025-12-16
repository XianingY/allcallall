# 🌐 APP_ENV 环境配置使用指南

## 概述

本文档说明 AllCallAll 移动端项目中的 APP_ENV 环境配置系统，该系统基于 `.env` 文件中的 `APP_ENV` 变量来自动检测和切换不同的环境配置。

## 🎯 特性

### 核心优势
- ✅ **直观明确** - APP_ENV=development/production 直接表达环境类型
- ✅ **多环境支持** - 支持 development/staging/production 三种环境
- ✅ **灵活配置** - 每个环境可以有不同的 API 地址和设置
- ✅ **详细日志** - 启动时显示完整的环境信息
- ✅ **类型安全** - 提供 IS_DEVELOPMENT、IS_PRODUCTION 等布尔值

## 🔧 环境类型

### 1. 开发环境 (development)
```bash
APP_ENV=development
```
- **API地址**: `http://192.168.31.217:8080`
- **WebSocket**: `ws://192.168.31.217:8080`
- **用途**: 本地开发调试
- **启动日志**: `🚀 开发模式`

### 2. 测试环境 (staging)
```bash
APP_ENV=staging
```
- **API地址**: `http://81.68.168.207:8080`
- **WebSocket**: `ws://81.68.168.207:8080`
- **用途**: 预发布测试
- **启动日志**: `🧪 测试模式`

### 3. 生产环境 (production)
```bash
APP_ENV=production
```
- **API地址**: `http://81.68.168.207`
- **WebSocket**: `ws://81.68.168.207`
- **用途**: 生产部署
- **启动日志**: `🏭 生产模式`

## 📁 配置文件

### .env 文件
```bash
# .env
APP_ENV=development

# 开发环境API地址
DEV_API_URL=192.168.31.217
DEV_API_PORT=8080

# 生产环境API地址
PROD_API_URL=81.68.168.207
PROD_API_PORT=8080

# 调试模式
DEBUG_MODE=false
```

### .env.example 文件
```bash
# .env.example
APP_ENV=development

# 开发环境API地址
DEV_API_URL=192.168.31.217
DEV_API_PORT=8080

# 生产环境API地址
PROD_API_URL=81.68.168.207
PROD_API_PORT=8080

# 调试模式
DEBUG_MODE=false
```

## 🚀 使用方法

### 设置环境

#### 方法1: 编辑 .env 文件
```bash
# 修改 .env 文件
APP_ENV=production

# 保存文件并重新启动应用
```

#### 方法2: 使用环境变量
```bash
# 在运行应用时设置环境变量
APP_ENV=production npm start
```

#### 方法3: 代码中设置
```typescript
// 在应用启动前设置
(global as any).__APP_ENV__ = 'production';
```

### 运行不同环境

#### 开发环境
```bash
# 确保 .env 文件中 APP_ENV=development
cd mobile
npm start

# 控制台输出:
# ==================================================
# 📋 环境检测结果
# ==================================================
# 环境类型: development
# 显示名称: 🚀 开发模式
# 描述: 使用本地开发环境配置
# API地址: http://192.168.31.217:8080
# WebSocket: ws://192.168.31.217:8080
# 设备信息: iPhone 15 (ios)
# ==================================================
```

#### 生产环境
```bash
# 方法1: 修改 .env 文件
APP_ENV=production
npm start

# 方法2: 环境变量
APP_ENV=production npm run android

# 控制台输出:
# ==================================================
# 📋 环境检测结果
# ==================================================
# 环境类型: production
# 显示名称: 🏭 生产模式
# 描述: 使用生产环境配置
# API地址: http://81.68.168.207
# WebSocket: ws://81.68.168.207
# 设备信息: iPhone 15 (ios)
# ==================================================
```

## 💻 在代码中使用

### 导入环境配置
```typescript
import {
  APP_ENVIRONMENT,
  IS_DEVELOPMENT,
  IS_STAGING,
  IS_PRODUCTION,
  API_BASE_URL,
  WS_URL
} from './src/config/index';
```

### 条件渲染
```typescript
// 根据环境渲染不同内容
function MyComponent() {
  if (IS_DEVELOPMENT) {
    return <DevelopmentPanel />;
  }

  if (IS_PRODUCTION) {
    return <ProductionView />;
  }

  return <DefaultView />;
}
```

### API 请求
```typescript
// 自动使用当前环境的 API 地址
fetch(`${API_BASE_URL}/users`)
  .then(response => response.json())
  .then(data => console.log(data));
```

### WebSocket 连接
```typescript
// 自动使用当前环境的 WebSocket 地址
const ws = new WebSocket(WS_URL);
```

### 调试日志
```typescript
// 只在开发环境输出调试日志
if (IS_DEVELOPMENT) {
  console.log('🔍 调试信息:', someData);
}
```

## 🔄 环境切换

### 快速切换脚本

#### 创建切换脚本
```bash
#!/bin/bash
# switch-env.sh

ENV=$1

if [ -z "$ENV" ]; then
  echo "使用方法: ./switch-env.sh <development|staging|production>"
  exit 1
fi

# 验证环境值
if [[ ! "$ENV" =~ ^(development|staging|production)$ ]]; then
  echo "错误: 无效的环境值 '$ENV'"
  echo "有效值: development, staging, production"
  exit 1
fi

# 更新 .env 文件
sed -i '' "s/APP_ENV=.*/APP_ENV=$ENV/" .env

echo "✅ 环境已切换到: $ENV"
echo "请重启应用以使更改生效"
```

#### 使用脚本
```bash
# 切换到开发环境
./switch-env.sh development

# 切换到生产环境
./switch-env.sh production

# 切换到测试环境
./switch-env.sh staging
```

## 📊 高级配置

### 自定义 API 地址

您可以在 `.env` 文件中自定义 API 地址：

```bash
# .env
APP_ENV=custom

# 自定义 API 地址
CUSTOM_API_URL=192.168.1.100
CUSTOM_API_PORT=9000
```

然后在代码中读取：

```typescript
const API_URL = process.env.CUSTOM_API_URL || 'http://192.168.1.100:9000';
```

### 多环境配置文件

#### .env.development
```bash
APP_ENV=development
API_URL=192.168.31.217:8080
DEBUG_MODE=true
```

#### .env.staging
```bash
APP_ENV=staging
API_URL=81.68.168.207:8080
DEBUG_MODE=false
```

#### .env.production
```bash
APP_ENV=production
API_URL=81.68.168.207
DEBUG_MODE=false
```

## 🐛 故障排除

### 环境未生效

**问题**: 修改 `.env` 文件后环境没有变化

**解决方案**:
1. 确保文件保存成功
2. 重启 Metro bundler: `npx expo start -c`
3. 重启应用
4. 检查环境变量是否正确设置

### 环境值无效

**问题**: 使用了不支持的环境值

**解决方案**:
1. 检查 `.env` 文件中的 `APP_ENV` 值
2. 确保值是小写: `development`/`staging`/`production`
3. 验证没有多余的空格或特殊字符

### API 连接失败

**问题**: 无法连接到 API 服务器

**解决方案**:
1. 验证当前环境配置
2. 检查网络连接
3. 确认 API 服务器地址正确
4. 查看控制台环境日志

## 📝 最佳实践

### 1. 环境文件管理
- ✅ 将 `.env` 文件加入 `.gitignore`
- ✅ 提交 `.env.example` 文件作为模板
- ✅ 定期更新 `.env.example` 保持同步

### 2. 环境切换
- ✅ 使用脚本自动化环境切换
- ✅ 在部署前验证环境配置
- ✅ 记录环境切换历史

### 3. 代码组织
- ✅ 使用导出的常量而非硬编码
- ✅ 在开发环境中启用详细日志
- ✅ 避免在生产环境中输出调试信息

### 4. 安全
- ✅ 不要在代码中硬编码敏感信息
- ✅ 使用环境变量管理配置
- ✅ 定期轮换 API 密钥和令牌

## 📚 相关文档

- [React Native Environment Variables](https://reactnative.dev/docs/environment-variables)
- [Expo Environment Variables](https://docs.expo.dev/guides/environment-variables/)
- [Metro Bundler](https://facebook.github.io/metro/)

## 📞 联系方式

如有问题或建议，请提交 Issue 或联系开发团队。

---

**创建日期**: 2024-12-11
**版本**: v1.0
**作者**: AllCallAll Development Team
