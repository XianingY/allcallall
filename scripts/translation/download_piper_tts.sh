#!/bin/bash

# Piper TTS 中文模型下载脚本
# 专门下载轻量级、高质量的中文 TTS 模型

set -e

echo "=== Piper TTS 中文模型下载器 ==="
echo ""
echo "Piper TTS 优势:"
echo "  ✅ 体积小: ~75MB"
echo "  ✅ 质量高: 接近商业级 TTS"
echo "  ✅ 多语言: 支持中文等多语言"
echo "  ✅ 适合移动端: 速度快，资源占用少"
echo ""

# 创建目录
mkdir -p assets/models/tts/piper_zh

# 检查依赖
check_tool() {
    if ! command -v wget &> /dev/null; then
        echo "错误: wget 未安装"
        return 1
    fi
    return 0
}

if ! check_tool; then
    echo "请先安装 wget"
    exit 1
fi

# 检查是否安装了 huggingface-cli
if command -v huggingface-cli &> /dev/null; then
    echo "检测到 huggingface-cli，使用它下载..."
    echo ""
    echo "正在下载 Piper TTS 中文模型..."
    huggingface-cli download rhasspy/piper-voices zh_CN-xiaoxiao-medium \
        --local-dir assets/models/tts/piper_zh \
        --local-dir-use-symlinks False

    if [ -f "assets/models/tts/piper_zh/medium.onnx" ]; then
        echo ""
        echo "✅ Piper TTS 中文模型下载完成!"
        echo ""
        echo "文件位置: assets/models/tts/piper_zh/"
        echo "文件大小: $(du -sh assets/models/tts/piper_zh/ | cut -f1)"
        echo ""
        echo "包含文件:"
        ls -lh assets/models/tts/piper_zh/ | grep -v total
    else
        echo "❌ 下载失败，尝试手动下载..."
        USE_WGET=1
    fi
else
    echo "未检测到 huggingface-cli，使用 wget 下载..."
    USE_WGET=1
fi

# 使用 wget 手动下载
if [ "$USE_WGET" = "1" ]; then
    echo ""
    echo "使用 wget 下载模型文件..."

    # 下载主要模型文件
    wget -c -O assets/models/tts/piper_zh/medium.onnx \
        "https://huggingface.co/rhasspy/piper-voices/resolve/main/zh_CN-xiaoxiao-medium/medium.onnx" || {
        echo "❌ 模型下载失败"
        echo "请检查网络连接或手动下载"
        exit 1
    }

    # 下载配置文件
    wget -c -O assets/models/tts/piper_zh/medium.onnx.json \
        "https://huggingface.co/rhasspy/piper-voices/resolve/main/zh_CN-xiaoxiao-medium/medium.onnx.json" || {
        echo "⚠️ 配置文件下载失败，但不影响使用"
    }

    # 下载音色文件
    wget -c -O assets/models/tts/piper_zh/zh_CN-xiaoxiao-medium.onnx.json \
        "https://huggingface.co/rhasspy/piper-voices/resolve/main/zh_CN-xiaoxiao-medium/zh_CN-xiaoxiao-medium.onnx.json" || {
        echo "⚠️ 音色配置文件下载失败"
    }

    echo ""
    echo "✅ Piper TTS 中文模型下载完成!"
fi

# 显示结果
echo ""
echo "=== 下载完成 ==="
echo ""
echo "📁 文件位置: assets/models/tts/piper_zh/"
echo "📊 文件大小: $(du -sh assets/models/tts/piper_zh/ 2>/dev/null | cut -f1 || echo 'N/A')"
echo ""
echo "包含文件:"
ls -lh assets/models/tts/piper_zh/ 2>/dev/null | grep -v total || echo "  (无文件)"

echo ""
echo "🎯 下一步:"
echo "  1. 将这些文件集成到 React Native 应用中"
echo "  2. 使用 ONNX Runtime 或 Piper 推理引擎"
echo "  3. 测试中文语音合成功能"
echo ""
echo "💡 参考资料:"
echo "  - Piper TTS 文档: https://github.com/rhasspy/piper"
echo "  - ONNX 推理: https://onnxruntime.ai/"
echo ""
