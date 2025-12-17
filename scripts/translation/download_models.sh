#!/bin/bash

# AllCallAll 模型下载脚本
# 下载并量化用于离线翻译的AI模型

set -e

echo "=== AllCallAll 模型下载与量化脚本 ==="
echo ""

# 创建目录
echo "创建模型目录..."
mkdir -p assets/models/{whisper,opus,tts}
mkdir -p temp_downloads

# 检查工具是否存在
check_tool() {
    if ! command -v $1 &> /dev/null; then
        echo "错误: $1 未安装"
        return 1
    fi
    return 0
}

# 下载函数
download_with_wget() {
    local url=$1
    local output=$2
    local description=$3

    echo "下载 $description..."
    if wget -c "$url" -O "$output"; then
        echo "✓ $description 下载完成"
    else
        echo "✗ $description 下载失败"
        return 1
    fi
}

download_with_hf() {
    local model_id=$1
    local output_dir=$2
    local description=$3

    echo "下载 $description..."
    if huggingface-cli download "$model_id" --local-dir "$output_dir"; then
        echo "✓ $description 下载完成"
    else
        echo "✗ $description 下载失败"
        return 1
    fi
}

# 检查依赖
echo "检查依赖工具..."
check_tool wget || { echo "请安装 wget: brew install wget"; exit 1; }
check_tool huggingface-cli || { echo "请安装 huggingface-cli: pip3 install huggingface_hub"; exit 1; }
check_tool python3 || { echo "请安装 Python3"; exit 1; }

# 1. Whisper 模型
echo ""
echo "=== 步骤 1: 下载 Whisper-small 模型 ==="
download_with_wget \
    "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-small.bin" \
    "temp_downloads/ggml-small.bin" \
    "Whisper-small 原始模型"

if [ ! -f "./whisper.cpp/quantize" ]; then
    echo "下载 whisper.cpp 量化工具..."
    if [ ! -d "whisper.cpp" ]; then
        git clone https://github.com/ggerganov/whisper.cpp || {
            echo "Git 克隆失败，尝试下载预编译版本..."
            # 如果git失败，用户需要手动下载
            echo "请手动从 https://github.com/ggerganov/whisper.cpp 下载并编译"
        }
    fi

    if [ -d "whisper.cpp" ]; then
        cd whisper.cpp
        make
        cd ..
    fi
fi

if [ -f "./whisper.cpp/quantize" ]; then
    echo "量化 Whisper 模型为 INT8..."
    ./whisper.cpp/quantize temp_downloads/ggml-small.bin assets/models/whisper/ggml-small-q8.bin q8_0
    echo "✓ Whisper INT8 量化完成"
    rm temp_downloads/ggml-small.bin
else
    echo "⚠ 量化工具不可用，跳过 Whisper 量化"
    cp temp_downloads/ggml-small.bin assets/models/whisper/ggml-small.bin
fi

# 2. Opus-MT 模型
echo ""
echo "=== 步骤 2: 下载 Opus-MT-en-zh 模型 ==="
download_with_hf \
    "Helsinki-NLP/opus-mt-en-zh" \
    "temp_downloads/opus-mt-en-zh" \
    "Opus-MT-en-zh 原始模型"

echo "转换 Opus-MT 到 ONNX..."
python3 -c "
import sys
try:
    from transformers import MarianMTModel, MarianTokenizer
    from transformers.onnx import export
    from pathlib import Path

    model_id = 'Helsinki-NLP/opus-mt-en-zh'
    model = MarianMTModel.from_pretrained(model_id)
    tokenizer = MarianTokenizer.from_pretrained(model_id)

    # 导出 ONNX
    onnx_path = Path('assets/models/opus/opus-mt-en-zh.onnx')
    onnx_path.parent.mkdir(parents=True, exist_ok=True)

    # 创建示例输入
    dummy_text = ['Hello world']
    inputs = tokenizer(dummy_text, return_tensors='pt')

    # 导出
    onnx_inputs = export(
        preprocessor=tokenizer,
        model=model,
        output=onnx_path,
        opset=13
    )
    print('ONNX export successful')
except ImportError as e:
    print(f'ImportError: {e}')
    print('跳过 ONNX 转换，使用原始模型')
    sys.exit(1)
except Exception as e:
    print(f'Error: {e}')
    print('跳过 ONNX 转换，使用原始模型')
    sys.exit(1)
" 2>/dev/null || {
    echo "ONNX 转换失败，使用原始模型文件"
    cp -r temp_downloads/opus-mt-en-zh/* assets/models/opus/ 2>/dev/null || true
}

# 3. VITS TTS 模型
echo ""
echo "=== 步骤 3: 下载 VITS TTS 模型 ==="
download_with_wget \
    "https://huggingface.co/coqui/VITS/resolve/main/vits-zh-en.bin" \
    "assets/models/tts/vits-zh-en.bin" \
    "VITS TTS 模型"

# 清理临时文件
echo ""
echo "清理临时文件..."
rm -rf temp_downloads

echo ""
echo "=== 模型下载完成! ==="
echo ""
echo "模型文件位置:"
echo "  Whisper:   assets/models/whisper/"
echo "  Opus-MT:   assets/models/opus/"
echo "  VITS TTS:  assets/models/tts/"
echo ""
echo "总模型大小:"

du -sh assets/models/whisper/ 2>/dev/null || echo "  Whisper:   N/A"
du -sh assets/models/opus/ 2>/dev/null || echo "  Opus-MT:   N/A"
du -sh assets/models/tts/ 2>/dev/null || echo "  VITS TTS:  N/A"

du -sh assets/models/ 2>/dev/null || echo "  总计:      N/A"

echo ""
echo "下一步: 将这些模型文件集成到 React Native 应用中"
