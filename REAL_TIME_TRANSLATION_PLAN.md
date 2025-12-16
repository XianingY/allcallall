# AllCallAll 实时翻译功能设计文档

## 🎯 功能概述

### 核心功能
实现**语音实时翻译**，在视频通话过程中实时将对方的语音翻译成目标语言，并显示在屏幕上或播放翻译后的语音。

### 目标效果
```
用户A（中文）  ←→  翻译引擎  ←→  用户B（英文）
   ↓                    ↓                    ↓
说："你好"        →  识别→翻译  →      "Hello"
听："Hello"       ←  翻译→合成  ←      说："Hello"
```

## 📊 功能需求

### 用户故事
- **作为用户**，我希望能够实时看到对方的语音翻译
- **作为用户**，我希望能够听到翻译后的语音
- **作为用户**，我希望能够切换翻译语言
- **作为用户**，我希望能够关闭/开启翻译功能

### 核心特性
1. **语音识别** - 实时将语音转为文字
2. **文本翻译** - 将识别出的文字翻译为目标语言
3. **字幕显示** - 实时显示翻译字幕
4. **语音合成** - 将翻译结果转换为语音播放
5. **语言检测** - 自动识别对方语言
6. **双向翻译** - 支持双向实时翻译

### 支持语言（初期）
- 中文（简体/繁体）
- 英语

## 🏗️ 技术架构

### 整体架构

```
┌─────────────┐         ┌─────────────┐
│   客户端A    │         │   客户端B    │
│             │         │             │
│  ┌────────┐ │         │  ┌────────┐ │
│  │ 语音   │ │         │  │ 语音   │ │
│  │ 采集   │ │         │  │ 采集   │ │
│  └────┬───┘ │         │  └────┬───┘ │
│       │     │         │       │     │
│  ┌────▼───┐ │         │  ┌────▼───┐ │
│  │ 语音   │ │         │  │ 语音   │ │
│  │ 识别   │ │         │  │ 识别   │ │
│  └────┬───┘ │         │  └────┬───┘ │
│       │     │         │       │     │
│  ┌────▼───┐ │         │  ┌────▼───┐ │
│  │ 文本   │ │         │  │ 文本   │ │
│  │ 翻译   │ │         │  │ 翻译   │ │
│  └────┬───┘ │         │  └────┬───┘ │
│       │     │         │       │     │
│  ┌────▼───┐ │         │  ┌────▼───┐ │
│  │ 语音   │ │         │  │ 语音   │ │
│  │ 合成   │ │         │  │ 合成   │ │
│  └────────┘ │         │  └────────┘ │
└─────────────┘         └─────────────┘
       ↕                       ↕
       └──────────  WebRTC  ─────────┘
                   视频通话
```

### 客户端架构

```
┌─────────────────────────────────────┐
│           客户端 (React Native)      │
├─────────────────────────────────────┤
│  UI 层                              │
│  ┌───────────┐ ┌─────────────────┐ │
│  │ 翻译字幕  │ │   控制面板      │ │
│  │ 显示组件  │ │   (语言选择等)  │ │
│  └───────────┘ └─────────────────┘ │
├─────────────────────────────────────┤
│  业务逻辑层                          │
│  ┌─────────────────────────────────┐ │
│  │      翻译服务 (TranslationService)  │ │
│  └─────────────────────────────────┘ │
├─────────────────────────────────────┤
│  服务集成层                          │
│  ┌────────┐ ┌────────┐ ┌──────────┐ │
│  │ 语音   │ │ 文本   │ │ 语音合成 │ │
│  │ 识别   │ │ 翻译   │ │  TTS    │ │
│  └────────┘ └────────┘ └──────────┘ │
└─────────────────────────────────────┘
```

## 🔧 技术实现方案

### 方案一：云服务集成（推荐）

#### 优势
- ✅ 开发速度快
- ✅ 准确率高
- ✅ 支持语言多
- ✅ 持续更新
- ❌ 成本较高
- ❌ 依赖网络
- ❌ 隐私考虑

#### 技术选型

**语音识别 (ASR)**
- Google Cloud Speech-to-Text
- 百度语音识别
- 阿里云语音识别
- Azure Speech Services

**文本翻译 (NMT)**
- Google Translate API
- DeepL API
- 百度翻译
- 腾讯翻译君
- Azure Translator

**语音合成 (TTS)**
- Google Cloud Text-to-Speech
- 百度合成
- 阿里云TTS
- Azure Speech

#### 成本分析
```
按每分钟翻译计算：
- 语音识别: $0.006/分钟
- 文本翻译: $20/百万字符 ≈ $0.02/分钟
- 语音合成: $0.016/分钟

总计: 约 $0.042/分钟/用户
≈ 2.5元/小时/用户
```

### 方案二：本地模型（长期规划）

#### 优势
- ✅ 隐私保护好
- ✅ 无网络依赖
- ✅ 可定制化
- ❌ 模型体积大
- ❌ 准确率较低
- ❌ 设备性能要求高
- ❌ 开发周期长

#### 技术选型

**离线语音识别**
- Whisper (OpenAI)
- Vosk
- SpeechRecognition

**离线翻译**
- Opus-MT
- M2M-100
- mBART

**离线TTS**
- eSpeak NG
- Festival
- Coqui TTS

#### 模型大小
```
- Whisper small: ~244MB
- Opus-MT en-zh: ~300MB
- TTS 模型: ~100MB
总计: ~650MB
```

### 混合方案（最佳实践）

```
┌─────────────────────────────────────┐
│            智能路由选择              │
│                                     │
│  ┌─────────┐  ┌─────────┐          │
│  │ 云服务  │  │ 本地模型│          │
│  │ 优先    │  │ 备用    │          │
│  └─────────┘  └─────────┘          │
│                                     │
│  网络好 → 云服务                    │
│  网络差 → 本地模型                  │
│  隐私敏感 → 本地模型                │
└─────────────────────────────────────┘
```

## 📱 客户端实现

### 1. 翻译服务设计

```typescript
// TranslationService.ts
class TranslationService {
  // 单例模式
  private static instance: TranslationService;
  private isEnabled: boolean = false;
  private currentLanguage: string = 'en';
  private speechRecognizer: SpeechRecognizer;
  private translator: TextTranslator;
  private ttsEngine: TextToSpeech;

  // 实时翻译流程
  async startRealTimeTranslation(
    audioStream: MediaStream,
    targetLanguage: string,
    onTranslation: (result: TranslationResult) => void
  ): Promise<void> {
    // 1. 从音频流中识别语音
    const recognizedText = await this.speechRecognizer.recognize(audioStream);

    // 2. 翻译文本
    const translatedText = await this.translator.translate(
      recognizedText,
      targetLanguage
    );

    // 3. 语音合成
    await this.ttsEngine.speak(translatedText, targetLanguage);

    // 4. 返回结果
    onTranslation({
      original: recognizedText,
      translated: translatedText,
      language: targetLanguage,
      timestamp: Date.now()
    });
  }

  // 语言检测
  async detectLanguage(text: string): Promise<string> {
    return await this.translator.detectLanguage(text);
  }
}
```

### 2. 翻译字幕组件

```typescript
// TranslationOverlay.tsx
const TranslationOverlay: React.FC = () => {
  const [subtitles, setSubtitles] = useState<SubtitleItem[]>([]);
  const [isVisible, setIsVisible] = useState(true);

  return (
    <View style={styles.container}>
      {subtitles.map((item, index) => (
        <Animated.View
          key={index}
          style={[
            styles.subtitle,
            { opacity: fadeAnim } // 淡入淡出动画
          ]}
        >
          <Text style={styles.originalText}>{item.original}</Text>
          <Text style={styles.translatedText}>{item.translated}</Text>
        </Animated.View>
      ))}
    </View>
  );
};
```

### 3. 翻译控制面板

```typescript
// TranslationControlPanel.tsx
const TranslationControlPanel: React.FC = () => {
  const [isEnabled, setIsEnabled] = useState(false);
  const [sourceLanguage, setSourceLanguage] = useState('auto');
  const [targetLanguage, setTargetLanguage] = useState('en');

  return (
    <View style={styles.panel}>
      <Switch
        value={isEnabled}
        onValueChange={toggleTranslation}
      />

      <Picker
        selectedValue={targetLanguage}
        onValueChange={setTargetLanguage}
      >
        <Picker.Item label="中文" value="zh" />
        <Picker.Item label="English" value="en" />
        <Picker.Item label="日本語" value="ja" />
        <Picker.Item label="한국어" value="ko" />
      </Picker>

      <Button title="清空字幕" onPress={clearSubtitles} />
    </View>
  );
};
```

### 4. 音频处理

```typescript
// AudioProcessor.ts
class AudioProcessor {
  private audioContext: AudioContext;
  private analyser: AnalyserNode;

  // 从 WebRTC 流中提取音频
  async processWebRTCAudio(
    stream: MediaStream
  ): Promise<AudioBuffer> {
    const audioTrack = stream.getAudioTracks()[0];
    const mediaStreamSource = this.audioContext.createMediaStreamSource(
      new MediaStream([audioTrack])
    );

    // 连接分析器
    mediaStreamSource.connect(this.analyser);

    // 获取音频数据
    const bufferLength = this.analyser.frequencyBinCount;
    const dataArray = new Uint8Array(bufferLength);
    this.analyser.getByteFrequencyData(dataArray);

    return dataArray;
  }

  // 音频降噪
  async denoise(audioData: Float32Array): Promise<Float32Array> {
    // 实现降噪算法或调用降噪库
    return this.noiseReduction.filter(audioData);
  }
}
```

## 🔌 后端集成

### 翻译 API 服务

```go
// backend/internal/handlers/translation_handler.go
func (h *TranslationHandler) TranslateText(c *gin.Context) {
    var req TranslationRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    // 调用翻译服务
    result, err := h.translationService.Translate(req.Text, req.TargetLang)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "original": req.Text,
        "translated": result.Text,
        "language": result.Language,
    })
}
```

### WebSocket 实时翻译

```go
// backend/internal/handlers/realtime_translation.go
func (h *RealtimeTranslationHandler) HandleWebSocket(c *gin.Context) {
    // 升级到 WebSocket
    conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
    if err != nil {
        return
    }
    defer conn.Close()

    for {
        // 接收音频数据
        messageType, p, err := conn.ReadMessage()
        if err != nil {
            break
        }

        // 实时翻译
        result, err := h.translateAudio(p)
        if err != nil {
            // 发送错误
            conn.WriteMessage(messageType, []byte(err.Error()))
            continue
        }

        // 发送翻译结果
        conn.WriteJSON(result)
    }
}
```

## 🎨 UI/UX 设计

### 字幕显示样式

```css
/* 字幕样式 */
.subtitle-container {
  position: absolute;
  bottom: 100px;
  left: 50%;
  transform: translateX(-50%);
  max-width: 80%;
  background: rgba(0, 0, 0, 0.7);
  border-radius: 10px;
  padding: 15px;
}

.original-text {
  color: #ccc;
  font-size: 14px;
  margin-bottom: 5px;
}

.translated-text {
  color: #fff;
  font-size: 18px;
  font-weight: bold;
}
```

### 交互流程

```
1. 通话界面
   ↓
2. 点击"翻译"按钮
   ↓
3. 选择目标语言
   ↓
4. 开始实时翻译
   ↓
5. 显示字幕和播放翻译语音
   ↓
6. 可随时关闭或切换语言
```

## 📊 性能优化

### 1. 延迟优化
```
目标延迟: < 500ms
分解:
- 语音识别: 200-300ms
- 文本翻译: 100-200ms
- 语音合成: 100-150ms
- 网络传输: 50ms
总计: 450-700ms
```

### 2. 优化策略

**并行处理**
```typescript
// 识别和翻译并行
async function processAudio(audioData: Float32Array) {
  const recognizerPromise = this.speechRecognizer.recognize(audioData);
  const translationPromise = this.translationCache.getLatest();

  const [recognizedText] = await Promise.all([
    recognizerPromise,
    translationPromise
  ]);

  return await this.translator.translate(recognizedText);
}
```

**缓存机制**
```typescript
class TranslationCache {
  private cache = new Map<string, string>();

  // 缓存常用短语
  async get(text: string): Promise<string | null> {
    const normalized = text.trim().toLowerCase();
    return this.cache.get(normalized) || null;
  }

  async set(text: string, translation: string): Promise<void> {
    const normalized = text.trim().toLowerCase();
    this.cache.set(normalized, translation);
  }
}
```

**断句优化**
```typescript
// 智能断句，避免翻译碎片化
function splitIntoPhrases(text: string): string[] {
  // 按标点符号和停顿分割
  const phrases = text.split(/[。！？.!?]\s*/);
  return phrases.filter(p => p.length > 3);
}
```

## 🔐 隐私与安全

### 数据保护

**敏感数据处理**
1. **最小化原则** - 只传输必要数据进行翻译
2. **加密传输** - 所有音频和文本数据加密传输
3. **不存储** - 翻译服务不存储用户语音数据
4. **匿名化** - 可选的匿名翻译模式

**合规要求**
- GDPR 合规
- 数据本地化（中国用户数据存储在中国）
- 用户同意机制
- 数据删除权

### 实现

```typescript
class PrivacyProtectedTranslation {
  // 端到端加密
  async encryptAudio(audioData: Float32Array): Promise<EncryptedData> {
    return await this.encrypt(audioData, this.sessionKey);
  }

  // 数据脱敏
  sanitizeText(text: string): string {
    // 移除敏感信息（电话号码、身份证等）
    return text.replace(/\d{11}/g, '***********');
  }

  // 匿名模式
  async translateAnonymously(
    text: string
  ): Promise<string> {
    // 不关联用户身份，仅处理文本
    return await this.anonymousTranslator.translate(text);
  }
}
```

## 💰 成本分析

### 云服务成本（月度预估）

```
假设: 1000 DAU，每用户每天使用翻译 30 分钟

语音识别:
1000 × 30 × 30 天 = 900,000 分钟
$0.006/分钟 = $5,400

文本翻译:
约 $1,800

语音合成:
约 $4,800

总计月度成本: $12,000
≈ 86,000元/月
```

### 成本优化策略

1. **分级服务**
   - 免费用户: 限制翻译时长
   - 付费用户: 无限制使用

2. **缓存优化**
   - 常用短语缓存命中率 > 60%
   - 节省约 40% 翻译成本

3. **本地模型**
   - 离线模式下使用本地模型
   - 节省云服务成本

4. **批量翻译**
   - 合并小段音频批量处理
   - 提高效率降低成本

## 🧪 测试策略

### 功能测试
- [ ] 语音识别准确率
- [ ] 翻译质量评估
- [ ] 语音合成自然度
- [ ] 延迟测试
- [ ] 多语言支持

### 性能测试
- [ ] 端到端延迟 < 500ms
- [ ] 并发翻译 100+ 用户
- [ ] 内存使用 < 100MB
- [ ] CPU 占用 < 30%

### 兼容性测试
- [ ] iOS 14+
- [ ] Android 10+
- [ ] 不同网络环境
- [ ] 不同设备性能

## 📅 实施计划

### Phase 1: 基础功能（4-6周）

**Week 1-2: 架构设计**
- [ ] 设计翻译服务架构
- [ ] 选择云服务提供商
- [ ] 搭建后端翻译 API

**Week 3-4: 语音识别**
- [ ] 集成语音识别 SDK
- [ ] 实现音频流处理
- [ ] 优化识别准确率

**Week 5-6: 文本翻译**
- [ ] 集成翻译 API
- [ ] 实现语言检测
- [ ] 优化翻译速度

### Phase 2: 语音合成（2-3周）

**Week 7-8: TTS 集成**
- [ ] 集成 TTS 服务
- [ ] 实现语音播放
- [ ] 优化语音自然度

**Week 9: UI 开发**
- [ ] 翻译字幕显示
- [ ] 控制面板开发
- [ ] 语言选择器

### Phase 3: 优化（3-4周）

**Week 10-12: 性能优化**
- [ ] 延迟优化
- [ ] 缓存机制
- [ ] 错误处理

**Week 13: 测试**
- [ ] 功能测试
- [ ] 性能测试
- [ ] 兼容性测试

### Phase 4: 上线（1周）

**Week 14: 发布**
- [ ] 灰度发布
- [ ] 监控数据
- [ ] 反馈收集

## 🎯 成功指标

### 技术指标
- **延迟**: < 500ms (P95)
- **准确率**: 语音识别 > 90%，翻译准确率 > 85%
- **稳定性**: 99.5% 可用性
- **并发**: 支持 100+ 用户同时翻译

### 用户指标
- **使用率**: 30% 用户使用翻译功能
- **满意度**: NPS > 50
- **留存**: 使用翻译功能用户留存率 +20%

## 🔮 未来扩展

### 短期扩展
1. **多人翻译** - 支持群组通话中的多人翻译
2. **离线模式** - 本地模型支持
3. **专业术语** - 行业术语翻译优化
4. **字幕导出** - 导出翻译字幕文件

### 长期规划
1. **AI 驱动优化** - 基于用户习惯的智能优化
2. **情感翻译** - 保留语音情感信息
3. **手势翻译** - 手语识别和翻译
4. **实时双向** - 真正的双向实时对话

## 📚 参考资源

### 技术文档
- [WebRTC Audio Processing](https://developer.mozilla.org/en-US/docs/Web/API/WebRTC_API)
- [Google Cloud Speech-to-Text](https://cloud.google.com/speech-to-text)
- [DeepL API](https://www.deepl.com/docs-api)

### 开源项目
- [OpenAI Whisper](https://github.com/openai/whisper)
- [Vosk Offline Speech Recognition](https://alphacephei.com/vosk/)
- [libretranslate](https://github.com/LibreTranslate/LibreTranslate)

---

**总结**: 实时翻译功能是一个复杂但极具价值的功能。建议采用云服务集成方案快速上线，然后逐步优化和增加离线支持。该功能将显著提升 AllCallAll 的竞争力和用户体验。
