# AllCallAll 模型下载指南

本指南说明如何下载并准备 AllCallAll 离线翻译功能所需的 AI 模型。

## 📦 模型列表

| 模型 | 用途 | 原始大小 | INT8量化后 | 压缩率 |
|------|------|---------|-----------|--------|
| Whisper-small | 语音识别 | 244MB | ~74MB | 70% ↓ |
| Opus-MT-en-zh | 文本翻译 | 300MB | ~150MB | 50% ↓ |
| VITS TTS | 语音合成 | 40MB | 40MB | - |
| **总计** | | **584MB** | **~264MB** | **55% ↓** |

## 🛠️ 安装依赖

### 1. Python 依赖包
```bash
pip3 install huggingface_hub onnxruntime transformers
```

### 2. 系统工具
```bash
# macOS (使用 Homebrew)
brew install wget

# Ubuntu/Debian
sudo apt-get install wget git
```

### 3. 量化工具
```bash
# 克隆 whisper.cpp 获取量化工具
git clone https://github.com/ggerganov/whisper.cpp
cd whisper.cpp
make
```

## 📥 模型下载

### 方法 1: 使用自动化脚本 (推荐)

```bash
# 赋予执行权限
chmod +x download_models.sh

# 运行下载脚本
./download_models.sh
```

该脚本会自动：
- 创建必要的目录
- 下载所有模型文件
- 执行 INT8 量化 (如果工具可用)
- 显示模型大小统计

### 方法 2: 手动下载

#### 1. Whisper-small 模型
```bash
# 下载原始模型
wget -O assets/models/whisper/ggml-small.bin \
  https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-small.bin

# INT8 量化 (需要 whisper.cpp)
./whisper.cpp/quantize \
  assets/models/whisper/ggml-small.bin \
  assets/models/whisper/ggml-small-q8.bin q8_0
```

#### 2. Opus-MT-en-zh 模型
```bash
# 下载模型
huggingface-cli download Helsinki-NLP/opus-mt-en-zh \
  --local-dir ./models/opus-mt-en-zh

# 转换并量化 ONNX
python3 quantize_opus.py
```

#### 3. VITS TTS 模型
```bash
# 下载模型
wget -O assets/models/tts/vits-zh-en.bin \
  https://huggingface.co/coqui/VITS/resolve/main/vits-zh-en.bin
```

## 📁 最终目录结构

```
assets/models/
├── whisper/
│   └── ggml-small-q8.bin      # ~74MB (INT8量化)
├── opus/
│   ├── opus-mt-en-zh.onnx     # ~300MB (原始)
│   └── opus-mt-en-zh-q8.onnx  # ~150MB (INT8量化)
└── tts/
    └── vits-zh-en.bin         # ~40MB
```

## 🔧 脚本说明

### download_models.sh
主下载脚本，包含：
- 依赖检查
- 目录创建
- 模型下载
- 自动量化
- 进度显示

### quantize_opus.py
Opus-MT 模型专用脚本：
- 加载 MarianMT 模型
- 导出 ONNX 格式
- 执行 INT8 量化
- 显示压缩统计

## ⚡ 性能优化

### INT8 量化优势
- **大小减少 55%**: 584MB → 264MB
- **内存使用减少**: 更低的 RAM 占用
- **推理速度提升**: 量化模型推理更快
- **准确率损失 <3%**: 几乎无性能损失

### 硬件要求
- **存储空间**: 至少 300MB (模型 + 缓存)
- **内存**: 建议 1GB+ (模型加载)
- **CPU**: 支持 ARM64 或 x86_64

## 🚀 集成到应用

模型下载完成后，需要集成到 React Native 应用：

1. **复制模型文件**
   ```bash
   cp -r assets/models/* mobile/assets/models/
   ```

2. **配置 Android**
   ```gradle
   // android/app/build.gradle
   android {
       sourceSets {
           main {
               assets.srcDirs = ['src/main/assets']
           }
       }
   }
   ```

3. **配置 iOS** (如果需要)
   - 将模型文件添加到 Xcode 项目
   - 确保 "Create folder references" 选项

## 🔍 验证模型

下载完成后，验证文件是否存在：

```bash
# 检查文件
ls -lh assets/models/whisper/
ls -lh assets/models/opus/
ls -lh assets/models/tts/

# 计算总大小
du -sh assets/models/
```

预期输出：
```
assets/models/whisper/ggml-small-q8.bin  ~74MB
assets/models/opus/opus-mt-en-zh-q8.onnx ~150MB
assets/models/tts/vits-zh-en.bin         ~40MB
总计                                    ~264MB
```

## ❗ 故障排除

### Git 克隆失败
如果无法克隆 whisper.cpp：
```bash
# 尝试使用镜像
git clone https://ghproxy.com/github.com/ggerganov/whisper.cpp

# 或手动下载
# 访问: https://github.com/ggerganov/whisper.cpp/releases
# 下载量化工具
```

### Hugging Face 下载慢
```bash
# 使用镜像
export HF_ENDPOINT=https://hf-mirror.com
huggingface-cli download Helsinki-NLP/opus-mt-en-zh
```

### Python 依赖安装失败
```bash
# 使用用户安装
pip3 install --user huggingface_hub onnxruntime transformers

# 或使用虚拟环境
python3 -m venv venv
source venv/bin/activate
pip install huggingface_hub onnxruntime transformers
```

## 📚 参考资源

- **Whisper.cpp**: https://github.com/ggerganov/whisper.cpp
- **Hugging Face 模型**: https://huggingface.co/Helsinki-NLP/opus-mt-en-zh
- **Coqui TTS**: https://github.com/coqui-ai/TTS
- **ONNX Runtime**: https://onnxruntime.ai/

## 📝 下一步

模型下载完成后，可以开始：

1. **Week 1**: 项目初始化 (IMPLEMENTATION_ROADMAP.md)
2. **Week 2**: Android JNI 开发
3. **Week 3**: UI 组件集成
4. **Week 4**: 性能优化
5. **Week 5**: 测试与部署

查看 `IMPLEMENTATION_ROADMAP.md` 获取详细实施计划。
