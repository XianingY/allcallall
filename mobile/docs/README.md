# AllCallAll 移动端文档

移动端（React Native）的完整文档和指南。

## 📁 目录结构

### 🔧 设置和初始化 (`setup/`)

初始化和配置移动端环境。

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
│   ├── api/                      API 调用
│   ├── components/               UI 组件
│   ├── context/                  React Context
│   ├── screens/                  页面屏幕
│   ├── services/                 业务服务
│   └── config/                   配置文件
├── android/                      Android 特定代码
├── ios/                         iOS 特定代码
├── assets/                       静态资源
│   └── sounds/                   音频文件
├── docs/                        文档
│   ├── setup/                   设置文档
│   ├── features/                功能文档
│   ├── guides/                  使用指南
│   └── troubleshooting/         故障排除
├── scripts/                     脚本
│   ├── verify-alarm-setup.sh   验证 Alarm 设置
│   └── verify-app-env.sh       验证应用环境
├── package.json                依赖管理
├── app.json                    应用配置
├── metro.config.js            打包配置
└── tsconfig.json              TypeScript 配置
```

---

## 🚀 快速开始

### 安装依赖

```bash
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

## 🔗 相关链接

- [主项目文档](../../docs/README.md)
- [API 文档](../../docs/api/API_DOCUMENTATION.md)
- [部署指南](../../docs/deployment/DEPLOYMENT_GUIDE.md)

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

# 运行代码检查
npm run lint

# 运行类型检查
npm run type-check
```

---

## 🛠️ 故障排除

### 依赖冲突

```bash
# 清除 node_modules 重新安装
rm -rf node_modules package-lock.json
npm install
```

### 构建失败

```bash
# 清除 Gradle 缓存
cd android && ./gradlew clean

# 重新构建
./gradlew assembleDebug
```

### 环境检测失败

```bash
# 运行环境验证脚本
./scripts/verify-app-env.sh
./scripts/verify-alarm-setup.sh
```

---

## 🤝 贡献指南

1. 新增文档时，将其放在相应的 `docs/` 子目录
2. 新增脚本时，将其放在 `scripts/` 目录
3. 保持文档和代码同步更新

---

**最后更新**: 2025-12-16  
**版本**: v2.0
