# 📱 AllCallAll 移动端文档

移动端（React Native）的完整开发文档和指南。

## 📁 文档目录

### 🔧 设置和初始化 (`setup/`)

初始化和配置移动端开发环境。

- **[AUDIO_FILES_SETUP.md](./setup/AUDIO_FILES_SETUP.md)** - 音频文件配置
  - 音频资源的设置和验证
  - 铃声和背景音乐配置

- **[APP_ENV_USAGE.md](./setup/APP_ENV_USAGE.md)** - 应用环境变量使用
  - 开发/测试/生产环境配置
  - 环境检测机制

- **[AUTO_ENV_DETECTION.md](./setup/AUTO_ENV_DETECTION.md)** - 自动环境检测
  - 应用启动时的自动环境检测
  - 配置文件的自动选择

### 🎨 功能特性 (`features/`)

移动端特定功能的实现和优化。

- **[ALARM_ENHANCEMENTS_SUMMARY.md](./features/ALARM_ENHANCEMENTS_SUMMARY.md)** - Alarm 功能增强
  - 来电铃声和震动
  - 通话状态音效

- **[MP3_FORMAT_UPDATE.md](./features/MP3_FORMAT_UPDATE.md)** - MP3 格式更新
  - 音频文件格式支持
  - 兼容性和性能优化

### 📖 使用指南 (`guides/`)

实现细节和使用说明。

- **[IMPLEMENTATION_STATUS.md](./guides/IMPLEMENTATION_STATUS.md)** - 实现状态
  - 功能完成情况
  - 已知问题

- **[MODIFICATION_SUMMARY.md](./guides/MODIFICATION_SUMMARY.md)** - 修改总结
  - 最近的代码修改
  - 变更日志

### 🔍 故障排除 (`troubleshooting/`)

常见问题和解决方案。

（待补充）

---

## 📂 项目结构

```
mobile/
├── src/                          源代码
│   ├── api/                      - API 调用
│   ├── components/               - UI 组件
│   ├── context/                  - React Context
│   ├── screens/                  - 页面屏幕
│   ├── services/                 - 业务服务
│   └── config/                   - 配置文件
├── android/                      - Android 特定代码
├── ios/                         - iOS 特定代码
├── assets/                       - 静态资源
│   └── sounds/                   - 音频文件
├── scripts/                     - 移动端脚本
│   ├── verify-alarm-setup.sh
│   ├── verify-app-env.sh
│   └── README.md
├── docs/                        - 移动端文档（本文档）
├── package.json                - 依赖管理
├── app.json                    - 应用配置
├── metro.config.js            - 打包配置
└── tsconfig.json              - TypeScript 配置
```

---

## 🚀 快速开始

### 安装依赖

```bash
cd mobile
npm install
```

### 运行开发服务器

```bash
npm start
```

### 构建 APK

```bash
cd android
./gradlew assembleDebug
```

### 在真机上运行

```bash
npm run android
# 或
adb install -r android/app/build/outputs/apk/debug/app-debug.apk
```

---

## 📋 关键配置文件

- **`app.json`** - 应用元数据和配置
- **`eas.json`** - Expo 构建配置
- **`metro.config.js`** - Metro 打包器配置
- **`tsconfig.json`** - TypeScript 配置
- **`package.json`** - 项目依赖和脚本

---

## 📝 常用命令

```bash
# 启动开发服务器
npm start

# 清除缓存后启动
npm start -- --clear

# 运行在 Android 设备上
npm run android

# 构建 APK（Debug）
cd android && ./gradlew assembleDebug

# 构建 APK（Release）
cd android && ./gradlew assembleRelease
```

---

## 🔗 相关链接

- [主项目文档](../README.md)
- [API 文档](../api/API_DOCUMENTATION.md)
- [部署指南](../deployment/DEPLOYMENT_GUIDE.md)
- [移动端脚本](../../mobile/scripts/README.md)

---

**最后更新**: 2025-12-16  
**版本**: v2.0
