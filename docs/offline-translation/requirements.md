# 离线翻译功能代码实施计划

## 目标
让移动应用能够离线实时翻译语音（英↔中双向）

## 现状
✅ 模型文件已准备完毕
- Whisper: `ggml-small-q8.bin`
- Opus 英→中: `opus-mt-en-zh-q8.onnx` (108MB)
- Opus 中→英: `opus-mt-z.onnx`h-en-q8 (108MB)
- TTS 中文: `zh_CN-huayan-medium.onnx`
- TTS 英文: `en_US-amy-medium.onnx`

❌ 缺少实际的代码实现

---

## 第一步：修改 Android C++ 代码

### 文件1: `mobile/android/app/src/main/cpp/CMakeLists.txt`

**目标**: 告诉 Android 如何编译 C++ 代码

**需要添加**:
```cmake
# 在第11-19行替换为:
add_library(
    translation-lib
    SHARED
    translation-lib.cpp
    whisper/whisper.cpp
    whisper/ggml/ggml.c
    onnx/onnx_wrapper.cpp
    tts/piper_wrapper.cpp
)

# 在第33-41行添加:
target_include_directories(
    translation-lib
    PRIVATE
    ${CMAKE_CURRENT_SOURCE_DIR}
    ${CMAKE_CURRENT_SOURCE_DIR}/whisper
    ${CMAKE_CURRENT_SOURCE_DIR}/onnx
    ${CMAKE_CURRENT_SOURCE_DIR}/tts
)
```

### 文件2: `mobile/android/app/src/main/cpp/translation-lib.cpp`

**目标**: 实现真正的翻译功能

**需要重写第11-128行**:

```cpp
#include <jni.h>
#include <whisper.h>          // Whisper 头文件
#include <onnx_wrapper.h>     // ONNX 封装
#include <piper_wrapper.h>    // TTS 封装
#include <vector>
#include <string>

static struct whisper_context* g_whisper_ctx = nullptr;
static OnnxModel* g_opus_en_zh = nullptr;
static OnnxModel* g_opus_zh_en = nullptr;
static OnnxModel* g_tts_zh = nullptr;
static OnnxModel* g_tts_en = nullptr;

extern "C" JNIEXPORT void JNICALL
Java_com_allcallall_TranslationModule_nativeInitialize(
    JNIEnv *env,
    jobject thiz,
    jstring whisperPath,
    jstring opusPath,
    jstring ttsPath,
    jstring quantization
) {
    // 加载 whisper 模型
    g_whisper_ctx = whisper_init_from_file(env->GetStringUTFChars(whisperPath, 0));

    // 加载翻译模型
    g_opus_en_zh = new OnnxModel(std::string(env->GetStringUTFChars(opusPath, 0)) + "/opus-mt-en-zh-q8.onnx");
    g_opus_zh_en = new OnnxModel(std::string(env->GetStringUTFChars(opusPath, 0)) + "/opus-mt-zh-en-q8.onnx");

    // 加载 TTS 模型
    g_tts_zh = new OnnxModel(std::string(env->GetStringUTFChars(ttsPath, 0)) + "/zh_CN-huayan-medium.onnx");
    g_tts_en = new OnnxModel(std::string(env->GetStringUTFChars(ttsPath, 0)) + "/en_US-amy-medium.onnx");
}

extern "C" JNIEXPORT jstring JNICALL
Java_com_allcallall_TranslationModule_nativeTranslateAudio(
    JNIEnv *env,
    jobject thiz,
    jstring audioDataBase64,
    jstring targetLanguage
) {
    // 1. 解码音频数据
    std::vector<float> audio = base64_decode_to_float(env->GetStringUTFChars(audioDataBase64, 0));

    // 2. 语音识别 (Whisper)
    std::string recognized = whisper_transcribe(g_whisper_ctx, audio);

    // 3. 自动检测语言
    bool is_english = is_english_text(recognized);

    // 4. 选择翻译方向
    std::string translation;
    if (is_english && std::string(targetLanguage) == "zh") {
        translation = g_opus_en_zh->translate(recognized);
    } else if (!is_english && std::string(targetLanguage) == "en") {
        translation = g_opus_zh_en->translate(recognized);
    } else {
        translation = recognized;  // 无需翻译
    }

    // 5. TTS 合成
    std::vector<int16_t> audio_out;
    if (std::string(targetLanguage) == "zh") {
        audio_out = g_tts_zh->synthesize(translation);
    } else {
        audio_out = g_tts_en->synthesize(translation);
    }

    // 6. 返回结果 (包含翻译文本和音频)
    return env->NewStringUTF(build_result_json(recognized, translation, audio_out).c_str());
}
```

### 文件3: 创建辅助文件

**新建**: `mobile/android/app/src/main/cpp/whisper/whisper.cpp`

```cpp
#include "whisper.h"
#include <string>

struct whisper_context* whisper_init_from_file(const char* path) {
    // TODO: 使用 whisper.cpp 加载模型
    // return whisper_init_from_file(path);
    return nullptr;  // 占位符
}

std::string whisper_transcribe(struct whisper_context* ctx, const std::vector<float>& audio) {
    // TODO: 使用 whisper.cpp 进行语音识别
    // return whisper_full(ctx, audio.data(), audio.size());
    return "Hello";  // 占位符
}
```

**新建**: `mobile/android/app/src/main/cpp/onnx/onnx_wrapper.cpp`

```cpp
#include "onnx_wrapper.h"
#include <onnxruntime_cxx_api.h>

class OnnxModel {
    Ort::Session* session = nullptr;
    Ort::Env env;

public:
    OnnxModel(const std::string& model_path) : env(nullptr) {
        Ort::SessionOptions options;
        session = new Ort::Session(env, model_path.c_str(), options);
    }

    std::string translate(const std::string& text) {
        // TODO: 使用 ONNX Runtime 进行翻译
        // 1. Tokenize text
        // 2. Run inference
        // 3. Decode tokens
        return "你好";  // 占位符
    }
};
```

**新建**: `mobile/android/app/src/main/cpp/tts/piper_wrapper.cpp`

```cpp
#include "piper_wrapper.h"
#include <onnxruntime_cxx_api.h>

class OnnxModel {
    Ort::Session* session = nullptr;
    Ort::Env env;

public:
    OnnxModel(const std::string& model_path) : env(nullptr) {
        Ort::SessionOptions options;
        session = new Ort::Session(env, model_path.c_str(), options);
    }

    std::vector<int16_t> synthesize(const std::string& text) {
        // TODO: 使用 ONNX Runtime 进行 TTS 合成
        // 1. Tokenize text
        // 2. Run inference
        // 3. Convert to audio
        return std::vector<int16_t>(16000, 0);  // 占位符：1秒静音
    }
};
```

---

## 第二步：修改移动端代码

### 文件: `mobile/src/services/translation/TranslationService.ts`

**修改第123-137行**:

```typescript
private async getModelPath(modelName: string): Promise<string> {
    const modelFiles: { [key: string]: any } = {
      whisper: 'ggml-small-q8.bin',
      opus: {
        'en-zh': 'opus-mt-en-zh-q8.onnx',
        'zh-en': 'opus-mt-zh-en-q8.onnx'
      },
      tts: {
        'zh': 'zh_CN-huayan-medium.onnx',
        'en': 'en_US-amy-medium.onnx'
      }
    };

    if (typeof modelFiles[modelName] === 'string') {
      return `${RNFS.DocumentDirectoryPath}/models/${modelName}/${modelFiles[modelName]}`;
    } else {
      return `${RNFS.DocumentDirectoryPath}/models/${modelName}`;
    }
}
```

**修改第42-48行**:

```typescript
// 初始化 Native Module
if (TranslationModule) {
  await TranslationModule.initialize(
    await this.getModelPath('whisper'),
    await this.getModelPath('opus'),
    await this.getModelPath('tts'),
    config.quantization || 'int8'
  );
}
```

---

## 第三步：创建音频处理代码

### 文件: `mobile/src/services/translation/utils/AudioRecorder.ts`

**新建**:

```typescript
import AudioRecorderPlayer from 'react-native-audio-recorder-player';

class AudioRecorder {
  private recorder = new AudioRecorderPlayer();

  async startRecording(): Promise<void> {
    await this.recorder.startRecorder();
  }

  async stopRecording(): Promise<Float32Array> {
    const result = await this.recorder.stopRecorder();
    // TODO: 转换为 Float32Array
    return new Float32Array(0);
  }

  async recordChunk(durationMs: number): Promise<Float32Array> {
    // TODO: 录制指定时长的音频块
    return new Float32Array(0);
  }
}

export default new AudioRecorder();
```

---

## 第四步：更新 UI

### 文件: `mobile/src/components/translation/TranslationControl.tsx`

**添加显示**:

```typescript
<Text style={styles.directionText}>
  {isEnglish ? '英 → 中' : '中 → 英'}
</Text>

<Text style={styles.originalText}>
  原文: {originalText}
</Text>

<Text style={styles.translatedText}>
  译文: {translatedText}
</Text>

<TouchableOpacity onPress={playAudio}>
  <Text>🔊 播放翻译</Text>
</TouchableOpacity>
```

---

## 实施顺序

1. **先创建 C++ 框架** (第1天)
   - 修改 CMakeLists.txt
   - 创建基本 C++ 文件结构
   - 实现 JNI 桥接

2. **集成 whisper.cpp** (第2天)
   - 下载 whisper.cpp 源码
   - 实现 whisper 封装

3. **集成 ONNX Runtime** (第3天)
   - 添加 Android 依赖
   - 实现翻译和 TTS 封装

4. **完善移动端** (第4天)
   - 修复模型路径
   - 添加音频录制和播放

5. **测试和调试** (第5天)
   - 运行 Python 测试脚本


---

## 关键点

- 使用已有的 whisper.cpp 代码库
- 使用 ONNX Runtime 加载 .onnx 模型
- Android 有完整的 JNI 框架，iOS 需要从头实现
- 所有模型文件已准备好，只需要加载和使用

这个计划直接对应到具体的代码修改，你可以逐步实现每个文件。
