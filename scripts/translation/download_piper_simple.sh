#!/bin/bash

# Piper TTS 简单下载脚本
# 使用 wget 直接下载模型文件

set -e

echo "============================================================"
echo "🎙️ Piper TTS 简单下载器 (wget 方法)"
echo "============================================================"
echo ""

# 创建目录
echo "创建目录..."
mkdir -p assets/models/tts/zh/zh_CN-xiaoxiao-medium
mkdir -p assets/models/tts/en/en_US-jenny-medium
echo "  ✅ 目录创建完成"
echo ""

# 检查 wget
if ! command -v wget &> /dev/null; then
    echo "❌ 未安装 wget"
    echo "请先安装 wget"
    exit 1
fi

echo "✅ wget 已安装"
echo ""

# 下载函数
download_with_wget() {
    local url=$1
    local output=$2
    local description=$3

    echo "下载 $description..."
    echo "  链接: $url"
    echo "  目标: $output"

    if wget -c "$url" -O "$output"; then
        if [ -f "$output" ]; then
            size=$(du -h "$output" | cut -f1)
            echo "  ✅ 下载完成 ($size)"
        else
            echo "  ⚠️ 下载完成但文件不存在"
        fi
    else
        echo "  ❌ 下载失败"
        return 1
    fi
    echo ""
}

# 下载中文模型
echo "=== 下载中文 TTS 模型 ==="
echo ""

download_with_wget \
    "https://huggingface.co/rhasspy/piper-voices/resolve/main/zh_CN-xiaoxiao-medium/medium.onnx" \
    "assets/models/tts/zh/zh_CN-xiaoxiao-medium/medium.onnx" \
    "中文模型 (ONNX)"

download_with_wget \
    "https://huggingface.co/rhasspy/piper-voices/resolve/main/zh_CN-xiaoxiao-medium/medium.onnx.json" \
    "assets/models/tts/zh/zh_CN-xiaoxiao-medium/medium.onnx.json" \
    "中文模型配置"

# 下载英文模型
echo "=== 下载英文 TTS 模型 ==="
echo ""

download_with_wget \
    "https://huggingface.co/rhasspy/piper-voices/resolve/main/en_US-jenny-medium/medium.onnx" \
    "assets/models/tts/en/en_US-jenny-medium/medium.onnx" \
    "英文模型 (ONNX)"

download_with_wget \
    "https://huggingface.co/rhasspy/piper-voices/resolve/main/en_US-jenny-medium/medium.onnx.json" \
    "assets/models/tts/en/en_US-jenny-medium/medium.onnx.json" \
    "英文模型配置"

# 验证下载
echo "=== 验证下载 ==="
echo ""

check_file() {
    local file=$1
    local name=$2

    if [ -f "$file" ]; then
        size=$(du -h "$file" | cut -f1)
        echo "  ✅ $name: $size"
        return 0
    else
        echo "  ❌ $name: 文件不存在"
        return 1
    fi
}

zh_count=0
en_count=0

echo "中文模型:"
check_file "assets/models/tts/zh/zh_CN-xiaoxiao-medium/medium.onnx" "ONNX 模型" && zh_count=$((zh_count + 1))
check_file "assets/models/tts/zh/zh_CN-xiaoxiao-medium/medium.onnx.json" "配置文件" && zh_count=$((zh_count + 1))

echo ""
echo "英文模型:"
check_file "assets/models/tts/en/en_US-jenny-medium/medium.onnx" "ONNX 模型" && en_count=$((en_count + 1))
check_file "assets/models/tts/en/en_US-jenny-medium/medium.onnx.json" "配置文件" && en_count=$((en_count + 1))

echo ""
echo "============================================================"
echo "📊 下载总结"
echo "============================================================"
echo ""

total_files=$((zh_count + en_count))
expected_files=4

if [ $total_files -eq $expected_files ]; then
    echo "✅ 所有文件下载完成！"
    echo ""

    # 计算总大小
    zh_size=$(du -sh assets/models/tts/zh/zh_CN-xiaoxiao-medium 2>/dev/null | cut -f1)
    en_size=$(du -sh assets/models/tts/en/en_US-jenny-medium 2>/dev/null | cut -f1)

    echo "文件大小:"
    echo "  中文模型: $zh_size"
    echo "  英文模型: $en_size"
    echo ""

    echo "下一步:"
    echo "  1. 测试模型: python3 -c \"import onnxruntime as ort; ort.InferenceSession('assets/models/tts/zh/zh_CN-xiaoxiao-medium/medium.onnx'); print('✅ 中文模型加载成功')\""
    echo "  2. 测试英文: python3 -c \"import onnxruntime as ort; ort.InferenceSession('assets/models/tts/en/en_US-jenny-medium/medium.onnx'); print('✅ 英文模型加载成功')\""
    echo ""

    echo "你的完整翻译系统:"
    echo "  ✅ ASR: Whisper-tiny (42MB)"
    echo "  ✅ 翻译: Opus 双模型 (200MB)"
    echo "  ✅ TTS: Piper 双语 (~150MB)"
    echo "  📦 总计: ~400MB"

else
    echo "⚠️ 部分文件下载失败"
    echo "成功: $total_files/$expected_files 个文件"
    echo ""
    echo "请检查网络连接后重试"
fi

echo ""
