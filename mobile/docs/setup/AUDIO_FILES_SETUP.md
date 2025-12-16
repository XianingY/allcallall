# 音频文件设置指南

## 📋 概述

本指南将帮助您为 AllCallAll 应用添加真实的音频文件，以替换当前的合成音调。

## 🎵 音频文件要求

### 推荐格式
- **格式**: MP3 (推荐) 或 WAV
- **采样率**: 44100 Hz 或 48000 Hz
- **声道**: 单声道 (Mono)
- **比特率**: 64-128 kbps (MP3)
- **文件大小**: 建议小于 500KB

> **提示**: MP3格式具有更好的压缩比，文件更小，推荐使用。

### 音频类型和时长

| 音频类型 | 文件名 | 建议时长 | 用途 |
|----------|--------|----------|------|
| 来电铃声 | incoming_call.mp3 | 3-5 秒 | 接到来电时播放 |
| 回铃音 | ringback.mp3 | 2-3 秒 | 拨打电话时播放（对方手机响铃提示） |

## 📁 添加音频文件

### 步骤 1: 创建音频目录

在 `mobile/src/` 目录下创建以下目录结构：

```bash
cd mobile/src
mkdir -p assets/sounds
```

### 步骤 2: 添加音频文件

将您的音频文件放置在 `assets/sounds/` 目录下：

```
mobile/src/
└── assets/
    └── sounds/
        ├── incoming_call.mp3
        └── ringback.mp3
```

### 步骤 3: 检查文件引用

确保 `AudioServiceExpo.ts` 中的文件路径正确：

```typescript
private readonly audioFiles: AudioFile[] = [
  {
    type: "incoming_call",
    source: require("../../assets/sounds/incoming_call.mp3"),
    name: "incoming_call.mp3"
  },
  {
    type: "ringback",
    source: require("../../assets/sounds/ringback.mp3"),
    name: "ringback.mp3"
  }
];
```

## 🎨 自定义音频文件

### 录制自己的音频

您可以使用以下方法录制音频：

1. **使用 Audacity** (免费)
   - 下载: https://www.audacityteam.org/
   - 录制后导出为 WAV 格式

2. **使用手机录音应用**
   - 录制后传输到电脑
   - 转换为 WAV 格式

3. **在线生成**
   - 使用在线音频生成工具
   - 生成提示音或铃声

### 音频文件制作建议

#### 来电铃声 (incoming_call.wav)
- **内容**: 清晰、连续的铃声
- **音量**: 中等（避免过大）
- **建议**: 使用经典的电话铃声或自定义音乐片段

#### 回铃音 (ringback.wav)
- **内容**: 间歇性提示音
- **音量**: 中等
- **建议**: 表示对方正在振铃的音乐或提示音

## 🔧 测试音频文件

### 1. 检查文件存在性

在开发环境中，您可以使用以下方法检查音频文件：

```typescript
// 在 AudioServiceExpo.ts 中添加
console.log("Audio files check:", this.checkAudioFiles());
```

### 2. 验证音频加载

在控制台中查看加载日志：

```
[AudioService] ✓ Loaded: incoming_call.mp3
[AudioService] ✓ Loaded: ringback.mp3
```

### 3. 测试播放

进行实际通话测试，验证音频是否正确播放。

## 📱 平台特定说明

### Android

1. **文件放置**: 直接放置在 `assets/sounds/` 目录
2. **引用方式**: 使用 `require()` 引用
3. **权限**: 确保应用有音频播放权限

### iOS

1. **文件放置**: 放置在 `assets/sounds/` 目录
2. **Xcode 配置**: 可能需要在 Xcode 中添加文件引用
3. **权限**: 在 `Info.plist` 中添加音频相关权限

## ⚠️ 注意事项

### 文件大小限制
- 单个文件建议小于 500KB
- 总大小建议小于 2MB

### 兼容性
- 使用标准音频格式确保兼容性
- 避免使用 DRM 保护的音频文件

### 版权
- 确保您有音频文件的使用权
- 建议使用原创或开源音频

## 🔄 替换音频文件

### 更新现有文件

1. 替换 `assets/sounds/` 目录下的对应文件
2. 保持文件名不变
3. 重启应用以重新加载

### 添加新音频

1. 在 `assets/sounds/` 下添加新文件
2. 更新 `AudioServiceExpo.ts` 中的 `audioFiles` 数组
3. 重新编译应用

## 📊 调试和故障排除

### 常见问题

1. **文件未加载**
   - 检查文件路径是否正确
   - 确认文件格式受支持
   - 查看控制台错误日志

2. **音频不播放**
   - 检查设备音量设置
   - 确认应用音频权限
   - 验证音频文件未损坏

3. **应用崩溃**
   - 检查音频文件大小
   - 确认文件格式正确
   - 查看崩溃日志

### 调试工具

```typescript
// 在 AudioServiceExpo.ts 中查看状态
const status = await AudioService.getStatus('incoming_call');
console.log("Audio status:", status);

// 检查音频文件
const fileCheck = AudioService.checkAudioFiles();
console.log("Files check:", fileCheck);
```

## 🎯 性能优化

### 预加载
音频文件会在应用启动时自动预加载，确保播放流畅。

### 内存管理
- 音频文件加载到内存中
- 应用退出时自动清理
- 避免频繁加载/卸载

### 缓存策略
- 音频文件缓存以提高播放速度
- 循环播放时无需重复加载

## 📝 更新日志

- **v1.0.0**: 初始版本，支持基本音频播放
- **v1.1.0**: 添加音频预加载和缓存
- **v1.2.0**: 支持自定义音频文件

## 💡 提示和技巧

1. **使用 Audacity 优化音频**
   - 降低采样率以减小文件大小
   - 调整音量确保一致性
   - 添加淡入淡出效果

2. **测试不同设备**
   - 在多台设备上测试音频效果
   - 调整音量以适应不同设备

3. **考虑用户体验**
   - 避免过于刺耳的音频
   - 确保在嘈杂环境中也能听到
   - 考虑静音模式下的替代方案

## 📞 支持

如有问题，请参考：
- [移动端音频实现文档](./MOBILE_AUDIO_IMPLEMENTATION.md)
- [移动端音频设置指南](./MOBILE_AUDIO_SETUP.md)
- 项目 GitHub Issues
