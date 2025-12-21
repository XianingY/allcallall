# Piper TTS 中文模型下载指南

## 🎯 什么是 Piper TTS？

Piper TTS 是一个轻量级、高质量的文本转语音库，特别适合移动端和嵌入式设备。

### 优势
- ✅ **体积小**: 仅 ~75MB
- ✅ **质量高**: 接近商业级 TTS
- ✅ **速度快**: 实时合成
- ✅ **多语言**: 支持中文等多种语言
- ✅ **开源免费**: 完全开源

## 📥 下载方法

### 方法1: 自动下载 (推荐)
```bash
cd scripts/translation
./download_piper_tts.sh
```

### 方法2: 手动下载
```bash
# 创建目录
mkdir -p assets/models/tts/piper_zh

# 下载模型文件
wget -O assets/models/tts/piper_zh/medium.onnx \
  "https://huggingface.co/rhasspy/piper-voices/resolve/main/zh_CN-xiaoxiao-medium/medium.onnx"

# 下载配置文件
wget -O assets/models/tts/piper_zh/medium.onnx.json \
  "https://huggingface.co/rhasspy/piper-voices/resolve/main/zh_CN-xiaoxiao-medium/medium.onnx.json"
```

### 方法3: 使用 huggingface-cli
```bash
# 安装 huggingface-hub
pip3 install huggingface_hub

# 下载模型
huggingface-cli download rhasspy/piper-voices zh_CN-xiaoxiao-medium \
  --local-dir assets/models/tts/piper_zh \
  --local-dir-use-symlinks False
```

## 📁 文件结构

下载完成后，目录结构如下：

```
assets/models/tts/piper_zh/
├── medium.onnx                  # 主要模型文件 (~70MB)
├── medium.onnx.json            # 模型配置 (~1KB)
└── zh_CN-xiaoxiao-medium.onnx.json  # 音色配置 (~1KB)
```

## 🔧 使用方法

### 在 Python 中使用
```python
import onnxruntime as ort

# 加载模型
session = ort.InferenceSession('assets/models/tts/piper_zh/medium.onnx')

# 准备输入
text = "你好，世界！"
# ... 文本预处理 ...

# 推理
outputs = session.run(None, {'text': processed_text})
audio_data = outputs[0]

# 保存音频
import wave
with wave.open('output.wav', 'wb') as wav_file:
    wav_file.setnchannels(1)  # 单声道
    wav_file.setsampwidth(2)  # 16位
    wav_file.setframerate(22050)  # 采样率
    wav_file.writeframes(audio_data.tobytes())
```

### 在 React Native 中使用
```javascript
// 使用 react-native-tts
import TTS from 'react-native-tts';

// 配置 Piper TTS 引擎
TTS.setDefaultVoice('zh_CN_xiaoxiao');
TTS.speak('你好，世界！');
```

## 🎨 其他 Piper TTS 模型

如果需要其他音色或语言，可以选择：

### 中文音色
- `zh_CN-xiaoxiao-medium` (推荐) - 女声，温和
- `zh_CN-yunyang-medium` - 男声，清晰
- `zh_CN-yunyang-medium` - 男声，稳重

### 其他语言
- `en_US-jenny-medium` - 英语女声
- `es_ES-sharvard-medium` - 西班牙语
- `de_DE-thorsten-medium` - 德语

下载地址: https://huggingface.co/rhasspy/piper-voices

## ⚠️ 注意事项

1. **网络问题**: 如果下载失败，请尝试使用镜像或代理
2. **文件完整性**: 下载完成后检查文件大小是否正常
3. **版权**: 仅用于学习和研究目的
4. **性能**: 在移动设备上可能需要优化

## 🔗 相关链接

- [Piper TTS GitHub](https://github.com/rhasspy/piper)
- [Hugging Face 模型库](https://huggingface.co/rhasspy/piper-voices)
- [ONNX Runtime](https://onnxruntime.ai/)
- [React Native TTS](https://github.com/ak1394/react-native-tts)

