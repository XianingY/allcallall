# 🔧 自动 __DEV__ 环境检测功能

## 📋 概述

本文档说明 AllCallAll 移动端项目中的自动 `__DEV__` 环境检测功能，该功能可以自动识别开发环境与生产环境，无需手动修改配置文件。

## 🎯 功能特性

### 核心优势
- ✅ **零手动配置** - 无需在开发/生产环境间手动切换
- ✅ **自动检测** - 基于 Metro bundler 的 `__DEV__` 全局变量
- ✅ **实时日志** - 启动时显示当前环境信息
- ✅ **类型安全** - 使用 TypeScript 确保正确性

## 🔍 实现原理

### 检测机制
```typescript
const __DEV__ = global.__DEV__ ?? false;
```

- **开发模式**: Metro bundler 自动设置 `global.__DEV__ = true`
- **生产模式**: Metro bundler 自动设置 `global.__DEV__ = false`
- **回退机制**: 如果未定义，默认使用 `false` (生产模式)

### 环境配置

| 环境 | 配置来源 | API端点 | WebSocket |
|------|---------|---------|-----------|
| **开发** | `DEV_API` | `http://192.168.31.217:8080` | `ws://192.168.31.217:8080` |
| **生产** | `PROD_API_IP` | `http://47.109.183.99` | `ws://47.109.183.99` |

## 🚀 使用方法

### 开发环境 (Development)

#### 方式1: Expo 开发服务器
```bash
cd mobile
npm start
# 或
npx expo start
```

**特点**:
- 自动设置 `__DEV__ = true`
- 使用本地API配置
- 支持热重载
- 调试信息丰富

**启动日志**:
```
🚀 开发模式: 使用本地API配置
📡 API: http://192.168.31.217:8080
🔌 WebSocket: ws://192.168.31.217:8080
```

#### 方式2: 开发构建 (Development Build)
```bash
cd mobile
npm run android
# 或
npm run ios
```

**特点**:
- 安装到设备/模拟器的开发版本
- 自动设置 `__DEV__ = true`
- 支持所有 Expo 开发功能
- 调试体验最佳

### 生产环境 (Production)

#### 方式1: EAS 构建
```bash
cd mobile
eas build --profile production --platform android
eas build --profile production --platform ios
```

**特点**:
- 自动设置 `__DEV__ = false`
- 使用生产API配置
- 代码混淆和优化
- 发布到应用商店

**启动日志**:
```
🏭 生产模式: 使用云服务器API配置
📡 API: http://47.109.183.99
🔌 WebSocket: ws://47.109.183.99
```

#### 方式2: 开发预览 (Development Preview)
```bash
eas build --profile preview --platform android
```

**特点**:
- 介于开发与生产之间
- 可以设置 `__DEV__ = false`
- 适合QA测试

## 📊 环境配置详解

### 开发环境配置 (`DEV_API`)
```typescript
const DEV_API = {
  HTTP: "http://192.168.31.217:8080",  // 本地开发服务器
  WS: "ws://192.168.31.217:8080"        // WebSocket 信令服务器
};
```

**适用场景**:
- 本地开发调试
- 功能测试
- 联调测试

### 生产环境配置 (`PROD_API_IP`)
```typescript
const PROD_API_IP = {
  HTTP: "http://47.109.183.99",  // 云服务器
  WS: "ws://47.109.183.99"        // WebSocket 信令服务器
};
```

**适用场景**:
- 生产部署
- 公开测试
- 应用发布

### 可选配置 (`PROD_API`)
```typescript
const PROD_API = {
  HTTP: "https://allcall.cn",   // 域名配置
  WS: "wss://allcall.cn"        // 安全 WebSocket
};
```

**适用场景**:
- 域名替代IP
- HTTPS/WSS 安全连接
- 生产环境推荐

## 🛠️ 开发流程

### 1. 日常开发
```bash
# 1. 启动开发服务器
cd mobile
npm start

# 2. 在模拟器/真机中预览
# 按 'i' 打开 iOS 模拟器
# 按 'a' 打开 Android 模拟器
# 或扫描二维码在真机中预览

# 3. 查看控制台日志
# 应用启动时会显示环境信息
```

### 2. 测试生产构建
```bash
# 构建生产版本
eas build --profile production --platform android

# 安装到设备测试
eas build --profile production --platform android --profile preview

# 查看应用启动日志
# 确认显示 "生产模式" 信息
```

### 3. 调试环境切换
```bash
# 检查当前环境配置
# 在应用中打开开发者菜单
# 查看 Metro 控制台输出
# 或添加调试代码:
console.log('__DEV__:', global.__DEV__);
```

## 🔍 调试指南

### 验证环境配置

#### 方法1: 控制台检查
```typescript
// 在任意组件中添加
console.log('当前环境:', global.__DEV__ ? '开发模式' : '生产模式');
console.log('API地址:', API_BASE_URL);
console.log('WebSocket地址:', WS_URL);
```

#### 方法2: 开发菜单
```typescript
// 在 App.tsx 或任意组件中添加
if (__DEV__) {
  console.log('🔍 开发模式调试信息:');
  console.log('- API配置:', DEV_API);
  console.log('- 设备信息:', Device.modelName);
}
```

#### 方法3: 网络请求检查
```typescript
// 发送测试请求检查
fetch(API_BASE_URL + '/ping')
  .then(response => response.json())
  .then(data => console.log('后端响应:', data));
```

### 常见问题

#### Q: 开发模式下无法连接到后端
**A**: 检查以下几点:
1. 确保后端服务器运行在 `http://192.168.31.217:8080`
2. 确保设备与开发机在同一网络
3. 如果是真机调试，考虑使用ADB反向代理
4. 检查防火墙设置

#### Q: 生产模式下API请求失败
**A**: 检查以下几点:
1. 确认生产服务器 `47.109.183.99` 正常运行
2. 检查SSL证书配置（如果使用HTTPS）
3. 验证CORS设置
4. 查看网络连接

#### Q: 环境检测不准确
**A**: 检查以下几点:
1. 清理Metro缓存: `npx expo start -c`
2. 重新安装应用
3. 检查是否有其他代码修改了 `__DEV__` 值
4. 确认使用的是正确的构建命令

## 📝 代码示例

### 完整配置示例
```typescript
import { Platform } from "react-native";
import * as Device from "expo-device";

// 开发环境配置
const DEV_API = {
  HTTP: "http://192.168.31.217:8080",
  WS: "ws://192.168.31.217:8080"
};

// 生产环境配置
const PROD_API_IP = {
  HTTP: "http://47.109.183.99",
  WS: "ws://47.109.183.99"
};

// 自动检测环境
const __DEV__ = global.__DEV__ ?? false;

// 环境检测日志
if (__DEV__) {
  console.log("🚀 开发模式启动");
} else {
  console.log("🏭 生产模式启动");
}

// 选择配置
const API_CONFIG = __DEV__ ? DEV_API : PROD_API_IP;

// 导出配置
export const API_BASE_URL = `${API_CONFIG.HTTP}/api/v1`;
export const WS_URL = `${API_CONFIG.WS}/api/v1/ws`;
```

### 条件渲染示例
```typescript
// 在组件中使用环境变量
function MyComponent() {
  if (__DEV__) {
    // 仅在开发模式下显示
    return <DebugPanel />;
  }

  // 生产模式渲染
  return <ProductionView />;
}
```

### 调试信息示例
```typescript
// 在应用启动时显示环境信息
const showEnvironmentInfo = () => {
  console.log('='.repeat(50));
  console.log('📱 AllCallAll 环境信息');
  console.log('='.repeat(50));
  console.log('🔧 环境模式:', __DEV__ ? '开发模式' : '生产模式');
  console.log('📡 API地址:', API_BASE_URL);
  console.log('🔌 WebSocket:', WS_URL);
  console.log('📱 设备:', Device.modelName);
  console.log('💻 平台:', Platform.OS);
  console.log('='.repeat(50));
};

// 在应用启动时调用
showEnvironmentInfo();
```

## ✅ 检查清单

### 开发环境验证
- [ ] 运行 `npm start` 成功
- [ ] 控制台显示 "🚀 开发模式"
- [ ] API请求到 `192.168.31.217`
- [ ] WebSocket连接到 `ws://192.168.31.217`
- [ ] 热重载正常工作

### 生产环境验证
- [ ] 运行 `eas build --profile production` 成功
- [ ] 控制台显示 "🏭 生产模式"
- [ ] API请求到 `47.109.183.99`
- [ ] WebSocket连接到 `ws://47.109.183.99`
- [ ] 代码已混淆

## 🎯 最佳实践

1. **开发时**: 始终使用 `npm start` 或开发构建，确保使用开发环境配置
2. **测试时**: 使用 `eas build --profile preview` 进行预发布测试
3. **发布时**: 使用 `eas build --profile production` 构建生产版本
4. **调试时**: 关注控制台输出的环境信息，确保配置正确
5. **CI/CD**: 在流水线中明确区分开发和生产构建命令

## 📚 相关文档

- [Expo 文档 - Environment Variables](https://docs.expo.dev/guides/environment-variables/)
- [React Native - Debugging](https://reactnative.dev/docs/debugging)
- [Metro Bundler](https://facebook.github.io/metro/)
- [EAS Build](https://docs.expo.dev/build/introduction/)

## 📞 联系方式

如有问题或建议，请提交 Issue 或联系开发团队。

---

**创建日期**: 2024-12-11
**分支**: `feature/auto_DEV_detect`
**版本**: v1.0