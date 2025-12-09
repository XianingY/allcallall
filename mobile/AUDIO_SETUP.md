# 音频提醒功能设置指南

## 概述

本功能为 AllCallAll 应用添加了电话呼叫时的声音提醒功能，包括：
- 来电铃声
- 拨出电话拨号音
- 回铃音

## 实现的功能

### 1. 来电提醒
- 当用户收到新的来电时，播放持续的铃声直到用户接听或拒绝
- 遵循系统静音设置

### 2. 拨出提醒
- 当用户拨出电话时，播放拨号音
- 对方接听时自动停止

### 3. 设置控制
- 可通过应用内的设置选项进行开启/关闭控制
- 设置页面路径：联系人页面 → 设置按钮

## 技术实现

### 依赖包
- `react-native-sound`: 音频播放库

### 新增文件
- `src/services/AudioService.ts` - 音频服务管理
- `src/context/SettingsContext.tsx` - 设置上下文
- `src/screens/SettingsScreen.tsx` - 设置页面

### 修改文件
- `src/context/SignalingContext.tsx` - 集成音频播放逻辑
- `src/navigation/AppNavigator.tsx` - 添加设置页面路由
- `src/screens/ContactsScreen.tsx` - 添加设置入口
- `App.tsx` - 添加 SettingsProvider
- `package.json` - 添加依赖

## 音频文件准备

### 文件要求
1. 格式：WAV（系统原生支持）
2. 位置：`android/app/src/main/res/raw/`
3. 文件名：
   - `incoming_call.wav` - 来电铃声
   - `outgoing_dial.wav` - 拨号音
   - `ringback.wav` - 回铃音

### 音频文件建议
1. **来电铃声** (`incoming_call.wav`)
   - 时长：建议 3-5 秒
   - 音量：中等（避免过大）
   - 格式：WAV PCM 16-bit

2. **拨号音** (`outgoing_dial.wav`)
   - 时长：建议 1-2 秒
   - 音调：连续的单音或双音
   - 格式：WAV PCM 16-bit

3. **回铃音** (`ringback.wav`)
   - 时长：建议 2-3 秒
   - 音调：间歇性提示音
   - 格式：WAV PCM 16-bit

### 如何添加音频文件

#### Android
1. 创建目录：`android/app/src/main/res/raw/`
2. 将音频文件放入该目录
3. 确保文件名正确：
   - `incoming_call.wav`
   - `outgoing_dial.wav`
   - `ringback.wav`

#### iOS
iOS 使用不同的音频加载方式，需要在代码中调整 AudioService 初始化逻辑。

## 使用说明

### 安装依赖
```bash
cd mobile
npm install
```

### 运行应用
```bash
# 启动 Metro
npm start

# 运行 Android
npm run android
```

### 音频文件放置
1. 准备 WAV 格式音频文件
2. 将文件放入 `android/app/src/main/res/raw/` 目录
3. 重新构建应用

### 测试功能
1. 进入设置页面
2. 开启"音频提醒"开关
3. 测试来电和拨出电话
4. 验证音频播放是否正常

## 注意事项

1. **音频文件大小**
   - 保持文件体积小（建议 < 100KB）
   - 压缩适中，不要过度压缩

2. **权限**
   - Android 需要 RECORD_AUDIO 权限
   - 应用已自动申请权限

3. **静音模式**
   - 遵循系统静音设置
   - 静音时不播放音频

4. **兼容性**
   - react-native-sound 在 iOS 上需要额外配置
   - 建议在 iOS 设备上单独测试

## 自定义音频

### 替换音频文件
1. 准备新的 WAV 文件
2. 替换 `android/app/src/main/res/raw/` 中的对应文件
3. 重新构建应用

### 调整音量
在 `AudioService.ts` 中可以调整音量：
```typescript
sound.setVolume(0.5); // 0.0 到 1.0
```

## 故障排除

### 音频不播放
1. 检查音频文件是否存在
2. 确认权限已授予
3. 检查设备是否静音

### 构建失败
1. 确保 react-native-sound 已安装
2. 运行 `npm install`
3. 重新构建：`npm run android`

## 未来改进

1. **音频持久化**：设置保存到本地存储
2. **更多音频类型**：添加通话结束音、通话忙音等
3. **音量控制**：独立的音量设置
4. **iOS 支持**：完整支持 iOS 音频播放
5. **音频选择**：允许用户选择不同的铃声

## 相关代码

- 音频服务：`src/services/AudioService.ts`
- 设置管理：`src/context/SettingsContext.tsx`
- 设置页面：`src/screens/SettingsScreen.tsx`
- 信令逻辑：`src/context/SignalingContext.tsx`

---

**注意**：当前实现为开发版本，需要在实际部署前添加音频文件并进行充分测试。
