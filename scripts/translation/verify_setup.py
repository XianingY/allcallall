#!/usr/bin/env python3
"""
AllCallAll 环境验证脚本
验证模型下载环境是否正确配置
"""

import sys
import os
from pathlib import Path

def check_python_packages():
    """检查 Python 包是否安装"""
    print("=== 检查 Python 依赖包 ===")
    packages = {
        'huggingface_hub': 'Hugging Face CLI',
        'onnxruntime': 'ONNX Runtime',
        'transformers': 'Transformers'
    }

    missing = []
    for package, name in packages.items():
        try:
            __import__(package)
            print(f"✓ {name} ({package})")
        except ImportError:
            print(f"✗ {name} ({package}) - 未安装")
            missing.append(package)

    return len(missing) == 0

def check_directories():
    """检查目录结构"""
    print("\n=== 检查目录结构 ===")

    dirs = [
        'assets/models/whisper',
        'assets/models/opus',
        'assets/models/tts'
    ]

    all_exist = True
    for dir_path in dirs:
        full_path = Path(dir_path)
        if full_path.exists():
            print(f"✓ {dir_path}")
        else:
            print(f"✗ {dir_path} - 不存在")
            all_exist = False

    return all_exist

def check_model_files():
    """检查模型文件"""
    print("\n=== 检查模型文件 ===")

    models = [
        ('assets/models/whisper/ggml-small.bin', 'Whisper-small', 244),
        ('assets/models/opus/config.json', 'Opus-MT', 300),
        ('assets/models/tts/vits-zh-en.bin', 'VITS TTS', 40)
    ]

    found = 0
    for file_path, name, size_mb in models:
        full_path = Path(file_path)
        if full_path.exists():
            size = full_path.stat().st_size / (1024 * 1024)
            print(f"✓ {name}: {file_path} ({size:.1f}MB)")
            found += 1
        else:
            print(f"✗ {name}: {file_path} - 未找到")

    return found

def check_tools():
    """检查系统工具"""
    print("\n=== 检查系统工具 ===")

    tools = {
        'wget': 'Wget 下载工具',
        'git': 'Git 版本控制',
        'python3': 'Python 3'
    }

    import shutil
    all_available = True
    for tool, name in tools.items():
        if shutil.which(tool):
            print(f"✓ {name} ({tool})")
        else:
            print(f"✗ {name} ({tool}) - 未找到")
            all_available = False

    return all_available

def main():
    """主函数"""
    print("AllCallAll 模型环境验证")
    print("=" * 50)

    # 检查 Python 包
    packages_ok = check_python_packages()

    # 检查目录
    dirs_ok = check_directories()

    # 检查工具
    tools_ok = check_tools()

    # 检查模型文件
    models_found = check_model_files()

    # 总结
    print("\n" + "=" * 50)
    print("验证结果:")
    print(f"  Python 依赖: {'✓' if packages_ok else '✗'}")
    print(f"  目录结构: {'✓' if dirs_ok else '✗'}")
    print(f"  系统工具: {'✓' if tools_ok else '✗'}")
    print(f"  模型文件: {models_found}/3")

    if packages_ok and dirs_ok and tools_ok:
        print("\n✓ 环境配置正确！可以开始下载模型或继续开发。")
        if models_found < 3:
            print("\n⚠ 建议: 请下载缺失的模型文件 (查看 MANUAL_DOWNLOAD_INSTRUCTIONS.md)")
    else:
        print("\n✗ 环境配置不完整，请检查上述问题。")

    # 显示下一步建议
    print("\n下一步:")
    if models_found < 3:
        print("1. 手动下载模型文件 (查看 MANUAL_DOWNLOAD_INSTRUCTIONS.md)")
        print("2. 或等待网络恢复后运行 ./download_models.sh")
    else:
        print("1. 开始 Week 1 开发 (查看 IMPLEMENTATION_ROADMAP.md)")
        print("2. 使用 AI_PROMPT_001 生成项目结构")

if __name__ == "__main__":
    main()
