# AllCallAll 模型手动下载指南

由于网络限制，无法自动下载模型文件。请按照以下步骤手动下载：

## 📦 模型下载链接

### 1. Whisper-small 模型
- **下载链接**: https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-small.bin
- **文件名**: `ggml-small.bin`
- **大小**: ~244MB
- **保存到**: `assets/models/whisper/`

### 2. Opus-MT-en-zh 模型
- **下载链接**: https://huggingface.co/Helsinki-NLP/opus-mt-en-zh
- **文件**: 整个模型文件夹
- **大小**: ~300MB
- **保存到**: `assets/models/opus/`

### 3. VITS TTS 模型
- **下载链接**: https://huggingface.co/coqui/VITS/resolve/main/vits-zh-en.bin
- **文件名**: `vits-zh-en.bin`
- **大小**: ~40MB
- **保存到**: `assets/models/tts/`

## 🔧 手动下载步骤

### 方案一：浏览器直接下载
1. 打开浏览器，访问上述链接
2. 右键点击 → "另存为"
3. 选择对应的目录并保存

### 方案二：使用命令行下载（如果网络恢复）
```bash
# Whisper 模型
wget -O assets/models/whisper/ggml-small.bin \
  https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-small.bin

# Opus-MT 模型
hf download Helsinki-NLP/opus-mt-en-zh \
  --local-dir ./assets/models/opus/

# VITS TTS 模型
wget -O assets/models/tts/vits-zh-en.bin \
  https://huggingface.co/coqui/VITS/resolve/main/vits-zh-en.bin
```

### 方案三：使用镜像（如果可用）
```bash
# 设置 Hugging Face 镜像
export HF_ENDPOINT=https://hf-mirror.com

# 然后使用上面的命令
```

## 📁 最终目录结构

下载完成后，目录结构应如下：

```
assets/models/
├── whisper/
│   └── ggml-small.bin           # 244MB
├── opus/
│   ├── config.json
│   ├── model.safetensors
│   ├── tokenizer.json
│   └── vocab.txt
└── tts/
    └── vits-zh-en.bin           # 40MB
```

## 🔄 下一步：模型量化

### Whisper 量化
如果网络恢复或下载了 whisper.cpp：
```bash
git clone https://github.com/ggerganov/whisper.cpp
cd whisper.cpp
make
./quantize ../assets/models/whisper/ggml-small.bin \
  ../assets/models/whisper/ggml-small-q8.bin q8_0
```

### Opus-MT 量化
```bash
# 使用我们提供的脚本
python3 quantize_opus.py
```

## ⚠️ 注意事项

1. **文件完整性**: 下载后请检查文件大小是否正确
2. **目录结构**: 确保文件保存在正确的目录中
3. **权限**: 确保应用有权限读取这些文件

## 🚀 下载完成后

模型下载完成后，你可以：

1. **开始开发**: 按照 `implementation-roadmap.md` 进行 Week 1 开发
2. **运行测试**: 使用提供的测试脚本验证模型
3. **集成到应用**: 将模型文件复制到 React Native 项目

## 💡 替代方案

如果无法下载大文件，可以考虑：

1. **使用云存储**: 将模型文件上传到云盘，然后下载
2. **本地网络**: 使用其他网络环境下载
3. **分批下载**: 如果下载中断，使用 `wget -c` 断点续传

## 📞 获取帮助

如果遇到问题：
1. 检查网络连接
2. 尝试使用 VPN 或代理
3. 使用不同的下载工具（如 IDM、FDM 等）
4. 参考 `model-download-guide.md` 中的故障排除部分

---

**下载完成后，回到项目目录继续开发！** 🚀
