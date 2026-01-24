#!/usr/bin/env python3
"""
测试双向翻译功能
使用优化后的目录结构
"""

import os
import sys
from pathlib import Path

# 添加虚拟环境路径
sys.path.insert(0, str(Path(".venv/lib/python3.9/site-packages")))

def test_bidirectional_translation():
    """测试双向翻译"""
    print("=== 测试双向翻译功能 ===")
    print("")

    try:
        from transformers import MarianMTModel, MarianTokenizer
    except ImportError:
        print("❌ 未安装 transformers")
        print("请先运行: source .venv/bin/activate && pip install transformers")
        return False

    # 路径 - 使用 mobile/assets/models/ 目录
    base_dir = Path("mobile/assets/models/opus")
    en_zh_dir = base_dir / "en-zh"
    zh_en_dir = base_dir / "zh-en"

    print("检查模型目录...")
    if not en_zh_dir.exists():
        print(f"❌ {en_zh_dir} 不存在")
        print("提示: 请确保模型文件已复制到 mobile/assets/models/")
        return False

    if not zh_en_dir.exists():
        print(f"❌ {zh_en_dir} 不存在")
        print("提示: 请确保模型文件已复制到 mobile/assets/models/")
        return False

    print(f"  ✅ {en_zh_dir}")
    print(f"  ✅ {zh_en_dir}")
    print("")

    # 测试文本
    test_cases = [
        {
            "text": "Hello, how are you?",
            "expected_dir": "en-zh",
            "description": "英 → 中"
        },
        {
            "text": "你好，你怎么样？",
            "expected_dir": "zh-en",
            "description": "中 → 英"
        }
    ]

    print("开始测试...")
    print("")

    for i, test_case in enumerate(test_cases, 1):
        text = test_case["text"]
        direction = test_case["expected_dir"]
        desc = test_case["description"]

        print(f"测试 {i}: {desc}")
        print(f"输入: {text}")

        # 选择模型目录
        model_dir = en_zh_dir if direction == "en-zh" else zh_en_dir
        model_name = f"Helsinki-NLP/opus-mt-{direction}"

        print(f"模型: {model_name}")
        print(f"目录: {model_dir}")

        try:
            # 加载模型
            print("加载模型...")
            model = MarianMTModel.from_pretrained(str(model_dir))
            tokenizer = MarianTokenizer.from_pretrained(str(model_dir))

            # 翻译
            print("翻译中...")
            inputs = tokenizer.prepare_seq2seq_batch([text], return_tensors="pt")
            translated = model.generate(**inputs)
            result = tokenizer.decode(translated[0], skip_special_tokens=True)

            print(f"输出: {result}")
            print("✅ 测试通过")
        except Exception as e:
            print(f"❌ 测试失败: {e}")

        print("-" * 50)
        print("")

    print("=== 测试完成 ===")
    return True

if __name__ == "__main__":
    # 检查虚拟环境
    venv_path = Path(".venv")
    if not venv_path.exists():
        print("⚠️ 未找到虚拟环境 .venv")
        print("请确保在正确的目录下运行此脚本")

    success = test_bidirectional_translation()
    exit(0 if success else 1)
