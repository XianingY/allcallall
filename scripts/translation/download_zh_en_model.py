#!/usr/bin/env python3
"""
下载 Opus 中→英翻译模型
用于实现双向翻译功能
"""

import os
from pathlib import Path
from huggingface_hub import snapshot_download

def download_zh_en_model():
    """下载中文→英文翻译模型"""
    print("=== 下载 Opus 中→英翻译模型 ===")
    print("")

    # 创建目录
    output_dir = Path("assets/models/opus/zh-en")
    output_dir.mkdir(parents=True, exist_ok=True)

    model_id = "Helsinki-NLP/opus-mt-zh-en"

    print(f"模型: {model_id}")
    print(f"目标目录: {output_dir}")
    print("")

    try:
        print("正在下载模型...")
        print("这可能需要几分钟时间，请耐心等待...")
        print("")

        # 下载模型
        local_dir = snapshot_download(
            repo_id=model_id,
            local_dir=str(output_dir),
            local_dir_use_symlinks=False,
            resume_download=True
        )

        print("")
        print("✅ 模型下载完成!")
        print("")

        # 显示文件信息
        files = list(output_dir.glob("*"))
        total_size = sum(f.stat().st_size for f in files if f.is_file())
        total_size_mb = total_size / (1024 * 1024)

        print(f"下载了 {len(files)} 个文件")
        print(f"总大小: {total_size_mb:.1f} MB")
        print("")
        print("主要文件:")
        for file in sorted(files):
            if file.is_file():
                size_mb = file.stat().st_size / (1024 * 1024)
                print(f"  - {file.name} ({size_mb:.1f} MB)")

        print("")
        print("下一步:")
        print("1. 运行量化脚本: python3 quantize_opus.py")
        print("2. 修改 quantize_opus.py 中的模型ID为: Helsinki-NLP/opus-mt-zh-en")
        print("3. 测试双向翻译功能")

        return True

    except Exception as e:
        print(f"❌ 下载失败: {e}")
        print("")
        print("可能的原因:")
        print("  - 网络连接问题")
        print("  - 权限不足")
        print("  - 磁盘空间不足")
        print("")
        print("解决方案:")
        print("  - 检查网络连接")
        print("  - 确保有足够的磁盘空间 (>500MB)")
        print("  - 尝试手动下载文件")

        return False

if __name__ == "__main__":
    success = download_zh_en_model()
    exit(0 if success else 1)
