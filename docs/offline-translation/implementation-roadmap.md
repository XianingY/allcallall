# AllCallAll 离线本地翻译模型实施指令集

## 执行参数

### 目标系统
- Platform: React Native (Android only)
- Models: Whisper-small, Opus-MT-en-zh, VITS
- Quantization: INT8
- Total Size: ~264MB
- Performance Target: <500ms latency

### 硬性指标
- Speech Recognition Accuracy: >88%
- Translation Accuracy: >85%
- Model Load Time: 3-5 seconds
- Memory Footprint: +350MB peak

---

## 执行计划 (5 weeks)

```
WEEK 1: project_init
WEEK 2: jni_integration
WEEK 3: ui_integration
WEEK 4: performance_optimization
WEEK 5: testing_deployment
```

---

# WEEK 1: project_init

## 指令模板

### AI_PROMPT_001
```
Generate complete React Native TypeScript project structure for translation service with Android JNI integration.
Include: TranslationService.ts, WhisperModel.ts, OpusModel.ts, TTSModel.ts, VoiceCloningService.ts, AudioProcessor.ts, ModelDownloader.ts, Quantizer.ts
Components: TranslationOverlay.tsx, TranslationControl.tsx, VoiceCloneScreen.tsx, ModelDownloadScreen.tsx
Screens: TranslationScreen.tsx
Android: JNI interfaces, CMakeLists.txt, native C++ code structure
```

## 创建文件清单

### 目录结构 (执行以下命令)
```bash
mkdir -p mobile/src/services/translation/{models,voice,utils}
mkdir -p mobile/src/components/translation
mkdir -p mobile/src/screens
mkdir -p mobile/android/app/src/main/cpp/{whisper,opus,tts}
mkdir -p mobile/assets/models/{whisper,opus,tts}
```

### 依赖安装
```bash
npm install react-native-audio-recorder-player react-native-sound react-native-fs react-native-zip-archive
```

### Android 配置 (android/app/build.gradle)
```gradle
android {
    defaultConfig {
        ndk {
            abiFilters 'arm64-v8a', 'armeabi-v7a'
        }
    }
    externalNativeBuild {
        cmake {
            path "src/main/cpp/CMakeLists.txt"
        }
    }
}

dependencies {
    implementation 'com.facebook.react:react-native:+'
    implementation 'org.libsdl:SDL2:2.28.5'
}
```

## 代码模板

### TranslationService.ts
```typescript
// mobile/src/services/translation/TranslationService.ts
import { NativeModules } from 'react-native';
const { TranslationModule } = NativeModules;

export interface TranslationConfig {
  whisperModel?: 'tiny' | 'base' | 'small';
  opusModel?: string;
  ttsModel?: string;
  targetLanguage: string;
  quantization?: 'fp32' | 'int8' | 'int4';
}

export interface TranslationResult {
  originalText: string;
  translatedText: string;
  confidence: number;
  processingTime: number;
  audioUrl?: string;
}

class TranslationService {
  private isInitialized = false;
  private config: TranslationConfig | null = null;

  async initialize(config: TranslationConfig): Promise<void> {
    if (this.isInitialized) return;
    this.config = config;
    await this.checkAndDownloadModels(config);
    await TranslationModule.initialize(
      await this.getModelPath('whisper'),
      await this.getModelPath('opus'),
      await this.getModelPath('tts'),
      config.quantization || 'int8'
    );
    this.isInitialized = true;
  }

  private async checkAndDownloadModels(config: TranslationConfig): Promise<void> {
    const models = [
      { name: 'whisper', path: this.getModelPath('whisper') },
      { name: 'opus', path: this.getModelPath('opus') },
      { name: 'tts', path: this.getModelPath('tts') }
    ];
    for (const model of models) {
      const exists = await RNFS.exists(model.path);
      if (!exists) await this.downloadModel(model.name);
    }
  }

  private async downloadModel(modelName: string): Promise<void> {
    const downloader = new ModelDownloader();
    await downloader.download(modelName);
  }

  async translateAudio(
    audioData: Float32Array,
    targetLanguage: string
  ): Promise<TranslationResult> {
    if (!this.isInitialized) {
      throw new Error('Translation service not initialized');
    }
    const startTime = Date.now();
    return new Promise((resolve, reject) => {
      TranslationModule.translateAudio(
        this.floatArrayToBase64(audioData),
        targetLanguage,
        (result: any) => {
          resolve({
            originalText: result.originalText,
            translatedText: result.translatedText,
            confidence: result.confidence,
            processingTime: Date.now() - startTime,
            audioUrl: result.audioUrl
          });
        },
        (error: string) => {
          reject(new Error(error));
        }
      );
    });
  }

  private floatArrayToBase64(array: Float32Array): string {
    // Implementation required
  }

  private async getModelPath(modelName: string): Promise<string> {
    // Implementation required
  }
}

export default new TranslationService();
```

## Week 1 交付物
- 完整项目结构
- TranslationService.ts
- ModelDownloader.ts
- Native Module 接口定义

---

# WEEK 2: jni_integration

## AI_PROMPT_002
```
Generate complete Android JNI implementation for translation service.
Include: TranslationModule.java, native C++ interfaces, CMakeLists.txt
Models: whisper.cpp, opus.cpp, tts.cpp, translation-lib.cpp
Features: model loading, audio processing, inference pipeline, error handling
```

### JNI 接口 (TranslationModule.java)
```java
// android/app/src/main/java/com/allcallall/TranslationModule.java
package com.allcallall;

import androidx.annotation.NonNull;
import com.facebook.react.bridge.ReactApplicationContext;
import com.facebook.react.bridge.ReactContextBaseJavaModule;
import com.facebook.react.bridge.ReactMethod;
import com.facebook.react.bridge.Promise;
import com.facebook.react.bridge.WritableMap;
import com.facebook.react.bridge.WritableNativeMap;

public class TranslationModule extends ReactContextBaseJavaModule {
    private final ReactApplicationContext reactContext;

    public TranslationModule(ReactApplicationContext reactContext) {
        super(reactContext);
        this.reactContext = reactContext;
    }

    @NonNull
    @Override
    public String getName() {
        return "TranslationModule";
    }

    @ReactMethod
    public void initialize(
        String whisperPath,
        String opusPath,
        String ttsPath,
        String quantization,
        Promise promise
    ) {
        try {
            System.loadLibrary("translation-lib");
            nativeInitialize(
                whisperPath,
                opusPath,
                ttsPath,
                quantization
            );
            promise.resolve(true);
        } catch (Exception e) {
            promise.reject("INIT_ERROR", e.getMessage());
        }
    }

    @ReactMethod
    public void translateAudio(
        String audioDataBase64,
        String targetLanguage,
        Promise promise
    ) {
        try {
            String result = nativeTranslateAudio(
                audioDataBase64,
                targetLanguage
            );
            WritableMap response = new WritableNativeMap();
            response.putString("translatedText", result);
            response.putDouble("confidence", 0.9);
            promise.resolve(response);
        } catch (Exception e) {
            promise.reject("TRANSLATE_ERROR", e.getMessage());
        }
    }

    private native void nativeInitialize(
        String whisperPath,
        String opusPath,
        String ttsPath,
        String quantization
    );

    private native String nativeTranslateAudio(
        String audioDataBase64,
        String targetLanguage
    );
}
```

### C++ 推理引擎 (translation-lib.cpp)
```cpp
// android/app/src/main/cpp/translation-lib.cpp
#include <jni.h>
#include <string>
#include <vector>
#include <android/log.h>
#include "whisper.h"
#include "opus.h"
#include "tts.h"

#define LOG_TAG "TranslationLib"
#define LOGI(...) __android_log_print(ANDROID_LOG_INFO, LOG_TAG, __VA_ARGS__)
#define LOGE(...) __android_log_print(ANDROID_LOG_ERROR, LOG_TAG, __VA_ARGS__)

static whisper_context* g_whisper_ctx = nullptr;
static opus_context* g_opus_ctx = nullptr;
static tts_context* g_tts_ctx = nullptr;

extern "C" JNIEXPORT void JNICALL
Java_com_allcallall_TranslationModule_nativeInitialize(
    JNIEnv *env,
    jobject thiz,
    jstring whisperPath,
    jstring opusPath,
    jstring ttsPath,
    jstring quantization
) {
    const char* whisper_model = env->GetStringUTFChars(whisperPath, 0);
    const char* opus_model = env->GetStringUTFChars(opusPath, 0);
    const char* tts_model = env->GetStringUTFChars(ttsPath, 0);
    const char* quant_type = env->GetStringUTFChars(quantization, 0);

    LOGI("Initializing models...");
    g_whisper_ctx = whisper_init_from_file(whisper_model);
    g_opus_ctx = opus_init_from_file(opus_model);
    g_tts_ctx = tts_init_from_file(tts_model);

    env->ReleaseStringUTFChars(whisperPath, whisper_model);
    env->ReleaseStringUTFChars(opusPath, opus_model);
    env->ReleaseStringUTFChars(ttsPath, tts_model);
    env->ReleaseStringUTFChars(quantization, quant_type);
}

extern "C" JNIEXPORT jstring JNICALL
Java_com_allcallall_TranslationModule_nativeTranslateAudio(
    JNIEnv *env,
    jobject thiz,
    jstring audioDataBase64,
    jstring targetLanguage
) {
    if (!g_whisper_ctx || !g_opus_ctx) {
        LOGE("Models not initialized");
        return env->NewStringUTF("Error: Models not initialized");
    }

    const char* audio_b64 = env->GetStringUTFChars(audioDataBase64, 0);
    const char* target_lang = env->GetStringUTFChars(targetLanguage, 0);

    std::vector<float> audio_data = base64_decode_to_float(audio_b64);
    std::string recognized_text = whisper_transcribe(
        g_whisper_ctx,
        audio_data.data(),
        audio_data.size()
    );

    if (recognized_text.empty()) {
        env->ReleaseStringUTFChars(audioDataBase64, audio_b64);
        env->ReleaseStringUTFChars(targetLanguage, target_lang);
        return env->NewStringUTF("");
    }

    std::string translated_text = opus_translate(
        g_opus_ctx,
        recognized_text,
        target_lang
    );

    env->ReleaseStringUTFChars(audioDataBase64, audio_b64);
    env->ReleaseStringUTFChars(targetLanguage, target_lang);

    return env->NewStringUTF(translated_text.c_str());
}

std::vector<float> base64_decode_to_float(const char* input) {
    // Implementation required
}
```

### CMakeLists.txt
```cmake
# android/app/src/main/cpp/CMakeLists.txt
cmake_minimum_required(VERSION 3.18.1)

project("translation-lib")

add_library(
    translation-lib
    SHARED
    translation-lib.cpp
    whisper/whisper.cpp
    opus/opus.cpp
    tts/tts.cpp
)

find_library(
    log-lib
    log
)

target_link_libraries(
    translation-lib
    ${log-lib}
)
```

## Week 2 交付物
- JNI 接口完整实现
- C++ 推理引擎框架
- CMake 构建配置
- 模型加载与推理流程

---

# WEEK 3: ui_integration

## AI_PROMPT_003
```
Generate React Native UI components for real-time translation.
Include: TranslationOverlay.tsx (subtitle display), TranslationControl.tsx (language picker), VoiceCloneScreen.tsx
Features: animated subtitles, language switching, voice cloning interface
Integration: connect to TranslationService, handle WebRTC audio streams
```

### TranslationOverlay.tsx
```typescript
// mobile/src/components/translation/TranslationOverlay.tsx
import React, { useState, useEffect } from 'react';
import {
  View,
  Text,
  StyleSheet,
  Animated,
  Dimensions
} from 'react-native';

interface SubtitleItem {
  id: string;
  original: string;
  translated: string;
  timestamp: number;
}

interface TranslationOverlayProps {
  subtitles: SubtitleItem[];
  isVisible: boolean;
  language: string;
  onClear: () => void;
}

const { width, height } = Dimensions.get('window');

const TranslationOverlay: React.FC<TranslationOverlayProps> = ({
  subtitles,
  isVisible,
  language,
  onClear
}) => {
  const [fadeAnim] = useState(new Animated.Value(0));

  useEffect(() => {
    if (isVisible && subtitles.length > 0) {
      Animated.sequence([
        Animated.timing(fadeAnim, {
          toValue: 1,
          duration: 300,
          useNativeDriver: true
        }),
        Animated.delay(3000),
        Animated.timing(fadeAnim, {
          toValue: 0,
          duration: 500,
          useNativeDriver: true
        })
      ]).start();
    }
  }, [subtitles, isVisible]);

  if (!isVisible || subtitles.length === 0) {
    return null;
  }

  const latestSubtitle = subtitles[subtitles.length - 1];

  return (
    <View style={styles.container}>
      <Animated.View
        style={[
          styles.subtitleContainer,
          { opacity: fadeAnim }
        ]}
      >
        {latestSubtitle.original && (
          <Text style={styles.originalText}>
            {latestSubtitle.original}
          </Text>
        )}
        <Text style={styles.translatedText}>
          {latestSubtitle.translated}
        </Text>
        <Text style={styles.languageTag}>
          {language === 'zh' ? '中文' : 'English'}
        </Text>
      </Animated.View>
    </View>
  );
};

const styles = StyleSheet.create({
  container: {
    position: 'absolute',
    bottom: 120,
    left: 0,
    right: 0,
    alignItems: 'center',
    zIndex: 1000
  },
  subtitleContainer: {
    backgroundColor: 'rgba(0, 0, 0, 0.8)',
    borderRadius: 12,
    paddingHorizontal: 16,
    paddingVertical: 12,
    maxWidth: width * 0.9,
    alignItems: 'center'
  },
  originalText: {
    color: '#ccc',
    fontSize: 14,
    marginBottom: 4,
    textAlign: 'center'
  },
  translatedText: {
    color: '#fff',
    fontSize: 18,
    fontWeight: 'bold',
    textAlign: 'center'
  },
  languageTag: {
    color: '#3b82f6',
    fontSize: 12,
    marginTop: 4
  }
});

export default TranslationOverlay;
```

### TranslationControl.tsx
```typescript
// mobile/src/components/translation/TranslationControl.tsx
import React, { useState } from 'react';
import {
  View,
  Text,
  StyleSheet,
  Switch,
  TouchableOpacity,
  Modal,
  ScrollView
} from 'react-native';

interface TranslationControlProps {
  isEnabled: boolean;
  onToggle: (enabled: boolean) => void;
  targetLanguage: string;
  onLanguageChange: (language: string) => void;
  onVoiceClone: () => void;
}

const SUPPORTED_LANGUAGES = [
  { code: 'zh', name: '中文', nativeName: '中文' },
  { code: 'en', name: 'English', nativeName: 'English' },
  { code: 'ja', name: 'Japanese', nativeName: '日本語' },
  { code: 'ko', name: 'Korean', nativeName: '한국어' }
];

const TranslationControl: React.FC<TranslationControlProps> = ({
  isEnabled,
  onToggle,
  targetLanguage,
  onLanguageChange,
  onVoiceClone
}) => {
  const [showLanguagePicker, setShowLanguagePicker] = useState(false);
  const currentLanguage = SUPPORTED_LANGUAGES.find(
    lang => lang.code === targetLanguage
  );

  return (
    <View style={styles.container}>
      <View style={styles.controlBar}>
        <View style={styles.toggleContainer}>
          <Text style={styles.label}>翻译</Text>
          <Switch
            value={isEnabled}
            onValueChange={onToggle}
            trackColor={{ false: '#767577', true: '#81b0ff' }}
            thumbColor={isEnabled ? '#f5dd4b' : '#f4f3f4'}
          />
        </View>

        {isEnabled && (
          <>
            <TouchableOpacity
              style={styles.languageButton}
              onPress={() => setShowLanguagePicker(true)}
            >
              <Text style={styles.languageButtonText}>
                {currentLanguage?.nativeName}
              </Text>
            </TouchableOpacity>

            <TouchableOpacity
              style={styles.cloneButton}
              onPress={onVoiceClone}
            >
              <Text style={styles.cloneButtonText}>🎤</Text>
            </TouchableOpacity>
          </>
        )}
      </View>

      <Modal
        visible={showLanguagePicker}
        transparent={true}
        animationType="fade"
        onRequestClose={() => setShowLanguagePicker(false)}
      >
        <TouchableOpacity
          style={styles.modalOverlay}
          activeOpacity={1}
          onPress={() => setShowLanguagePicker(false)}
        >
          <View style={styles.languagePicker}>
            <Text style={styles.pickerTitle}>选择目标语言</Text>
            <ScrollView>
              {SUPPORTED_LANGUAGES.map(lang => (
                <TouchableOpacity
                  key={lang.code}
                  style={[
                    styles.languageOption,
                    targetLanguage === lang.code && styles.selectedOption
                  ]}
                  onPress={() => {
                    onLanguageChange(lang.code);
                    setShowLanguagePicker(false);
                  }}
                >
                  <Text
                    style={[
                      styles.languageOptionText,
                      targetLanguage === lang.code && styles.selectedOptionText
                    ]}
                  >
                    {lang.nativeName} ({lang.name})
                  </Text>
                </TouchableOpacity>
              ))}
            </ScrollView>
          </View>
        </TouchableOpacity>
      </Modal>
    </View>
  );
};

const styles = StyleSheet.create({
  container: {
    position: 'absolute',
    bottom: 20,
    left: 20,
    right: 20,
    zIndex: 1000
  },
  controlBar: {
    flexDirection: 'row',
    alignItems: 'center',
    backgroundColor: 'rgba(0, 0, 0, 0.7)',
    borderRadius: 25,
    paddingHorizontal: 16,
    paddingVertical: 12
  },
  toggleContainer: {
    flexDirection: 'row',
    alignItems: 'center',
    flex: 1
  },
  label: {
    color: '#fff',
    fontSize: 16,
    marginRight: 12
  },
  languageButton: {
    backgroundColor: '#3b82f6',
    borderRadius: 20,
    paddingHorizontal: 16,
    paddingVertical: 8,
    marginLeft: 12
  },
  languageButtonText: {
    color: '#fff',
    fontSize: 14,
    fontWeight: '600'
  },
  cloneButton: {
    marginLeft: 8
  },
  cloneButtonText: {
    fontSize: 24
  },
  modalOverlay: {
    flex: 1,
    backgroundColor: 'rgba(0, 0, 0, 0.5)',
    justifyContent: 'center',
    alignItems: 'center'
  },
  languagePicker: {
    backgroundColor: '#fff',
    borderRadius: 12,
    padding: 20,
    width: '80%',
    maxHeight: '60%'
  },
  pickerTitle: {
    fontSize: 18,
    fontWeight: 'bold',
    marginBottom: 16,
    textAlign: 'center'
  },
  languageOption: {
    paddingVertical: 12,
    borderBottomWidth: 1,
    borderBottomColor: '#f0f0f0'
  },
  selectedOption: {
    backgroundColor: '#e3f2fd'
  },
  languageOptionText: {
    fontSize: 16,
    textAlign: 'center'
  },
  selectedOptionText: {
    color: '#3b82f6',
    fontWeight: 'bold'
  }
});

export default TranslationControl;
```

### VoiceCloningService.ts
```typescript
// mobile/src/services/translation/voice/VoiceCloningService.ts
class VoiceCloningService {
  async cloneVoice(referenceAudio: string): Promise<string> {
    // Implementation required
  }

  async synthesize(text: string, voiceId: string): Promise<string> {
    // Implementation required
  }
}

export default new VoiceCloningService();
```

## Week 3 交付物
- TranslationOverlay.tsx (字幕显示)
- TranslationControl.tsx (控制面板)
- VoiceCloningService.ts (音色克隆)
- UI 集成到现有通话系统

---

# WEEK 4: performance_optimization

## AI_PROMPT_004
```
Optimize translation service for performance.
Tasks: implement parallel processing, memory management, caching, model preloading
Target: <500ms latency, <350MB memory usage
Techniques: async/await optimization, worker threads, memory pooling
```

### 性能优化实现

#### ParallelProcessor.ts
```typescript
// mobile/src/services/translation/utils/ParallelProcessor.ts
class ParallelProcessor {
  async processAudioStream(
    audioStream: MediaStream,
    onSubtitle: (subtitle: SubtitleItem) => void
  ): Promise<void> {
    // Implementation required
  }
}
```

#### PerformanceMonitor.ts
```typescript
// mobile/src/services/translation/utils/PerformanceMonitor.ts
class PerformanceMonitor {
  private metrics = {
    translationCount: 0,
    totalTranslationTime: 0,
    averageConfidence: 0,
    errorCount: 0,
    memoryUsage: []
  };

  recordTranslation(result: TranslationResult): void {
    this.metrics.translationCount++;
    this.metrics.totalTranslationTime += result.processingTime;
  }

  recordError(error: Error): void {
    this.metrics.errorCount++;
  }

  getMetrics() {
    return {
      ...this.metrics,
      averageTranslationTime: this.metrics.totalTranslationTime /
        this.metrics.translationCount,
      errorRate: this.metrics.errorCount / this.metrics.translationCount
    };
  }
}
```

### 集成到 SignalingContext
```typescript
// mobile/src/context/SignalingContext.tsx (增强版)
import TranslationService from '../services/translation/TranslationService';
import VoiceCloningService from '../services/translation/voice/VoiceCloningService';

const [translationEnabled, setTranslationEnabled] = useState(false);
const [translationLanguage, setTranslationLanguage] = useState('zh');
const [subtitles, setSubtitles] = useState<SubtitleItem[]>([]);

const startRealTimeTranslation = useCallback(async () => {
  if (!localStream) return;

  const audioTrack = localStream.getAudioTracks()[0];
  if (!audioTrack) return;

  const audioStream = new MediaStream([audioTrack]);

  TranslationService.initialize({
    whisperModel: 'small',
    opusModel: 'opus-mt-en-zh',
    ttsModel: 'vits-zh-en',
    targetLanguage: translationLanguage,
    quantization: 'int8'
  });

  const processor = new ParallelProcessor();
  await processor.processAudioStream(
    audioStream,
    (subtitle) => {
      setSubtitles(prev => [...prev, subtitle]);
    }
  );
}, [localStream, translationLanguage]);

const startCall = useCallback(async (email: string) => {
  try {
    const stream = await videoService.getLocalStream(
      true,
      videoEnabled,
      cameraFacing,
      'medium'
    );

    setLocalStream(stream);

    if (translationEnabled) {
      await startRealTimeTranslation();
    }

    // ... existing logic ...

  } catch (error) {
    // ... error handling ...
  }
}, [videoEnabled, cameraFacing, translationEnabled, startRealTimeTranslation]);
```

## Week 4 交付物
- ParallelProcessor 并行处理
- PerformanceMonitor 性能监控
- 内存管理优化
- 与 SignalingContext 完整集成

---

# WEEK 5: testing_deployment

## AI_PROMPT_005
```
Generate comprehensive test suite for translation service.
Tasks: unit tests, integration tests, E2E tests, performance benchmarks
Coverage: >80% code coverage, <500ms latency, >88% accuracy
Tools: Jest, React Native Testing Library, Detox
```

### 测试套件

#### TranslationService.test.ts
```typescript
// mobile/__tests__/TranslationService.test.ts
import TranslationService from '../src/services/translation/TranslationService';

describe('TranslationService', () => {
  beforeAll(async () => {
    await TranslationService.initialize({
      whisperModel: 'small',
      opusModel: 'opus-mt-en-zh',
      ttsModel: 'vits-zh-en',
      targetLanguage: 'zh',
      quantization: 'int8'
    });
  });

  test('should translate audio to text', async () => {
    const mockAudioData = new Float32Array(16000);
    const result = await TranslationService.translateAudio(
      mockAudioData,
      'zh'
    );

    expect(result.translatedText).toBeDefined();
    expect(result.confidence).toBeGreaterThan(0.5);
    expect(result.processingTime).toBeLessThan(1000);
  });
});
```

#### E2E 测试
```typescript
// mobile/__tests__/e2e/TranslationE2E.test.ts
describe('Translation E2E Tests', () => {
  test('complete translation workflow', async () => {
    await TranslationService.initialize(config);
    const mockAudio = generateMockAudio('Hello world');
    const result = await TranslationService.translateAudio(
      mockAudio,
      'zh'
    );

    expect(result.translatedText).toBe('你好世界');
    expect(result.confidence).toBeGreaterThan(0.7);
    expect(result.processingTime).toBeLessThan(500);
  });

  test('performance benchmarks', async () => {
    const startTime = Date.now();
    await TranslationService.initialize(config);
    const initTime = Date.now() - startTime;
    expect(initTime).toBeLessThan(5000);

    const audio = new Float32Array(16000);
    const result = await TranslationService.translateAudio(audio, 'zh');

    const processTime = Date.now() - startTime;
    expect(processTime).toBeLessThan(500);
  });
});
```

## 上线检查清单

### 功能测试
- [ ] 语音识别准确率 >88%
- [ ] 翻译准确率 >85%
- [ ] 延迟 <500ms (P95)
- [ ] 内存使用 <350MB
- [ ] 模型加载时间 <5秒

### 错误处理
- [ ] 网络异常处理
- [ ] 权限被拒绝处理
- [ ] 模型加载失败处理
- [ ] 音频设备异常处理

### 集成测试
- [ ] 与现有视频通话系统兼容
- [ ] WebRTC 音频流正常处理
- [ ] UI 响应正常
- [ ] 语言切换正常

## Week 5 交付物
- 完整测试套件 (单元测试 + E2E 测试)
- 性能基准测试报告
- 上线部署包
- 用户文档

---

## 模型文件准备

### 下载与量化命令
```bash
# 创建模型目录
mkdir -p assets/models/{whisper,opus,tts}

# Whisper 模型
wget -O assets/models/whisper/ggml-small.bin \
  https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-small.bin
./quantize ggml-small.bin ggml-small-q8.bin q8_0

# Opus-MT 模型
python convert_to_onnx.py --model opus-mt-en-zh --output assets/models/opus/opus-mt-en-zh.onnx
onnxruntime_quantization quantize \
  --input assets/models/opus/opus-mt-en-zh.onnx \
  --output assets/models/opus/opus-mt-en-zh-q8.onnx \
  --quant_type int8

# TTS 模型
wget -O assets/models/tts/vits-zh-en.bin \
  https://huggingface.co/coqui/VITS/resolve/main/vits-zh-en.bin
```

## 总结

### 执行参数
- **开发周期**: 5 周
- **AI 辅助**: 每个阶段提供具体指令模板
- **技术栈**: React Native + Android JNI + C++
- **模型**: INT8 量化 (~264MB)
- **性能**: <500ms 延迟, >88% 准确率

### 立即执行
使用提供的 AI_PROMPT_001 到 AI_PROMPT_005 配合 AI 助手快速生成代码并实施。
