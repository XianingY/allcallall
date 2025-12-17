#!/usr/bin/env python3
"""
Opus-MT 模型 ONNX 转换与量化脚本
用于将 Helsinki-NLP/opus-mt-en-zh 模型转换为 INT8 量化的 ONNX 格式
"""

import sys
import os
from pathlib import Path

def convert_and_quantize_model():
    """转换并量化 Opus-MT 模型"""
    try:
        from transformers import MarianMTModel, MarianTokenizer
        from transformers.onnx import export
        import onnx
        from onnxruntime.quantization import quantize_dynamic, QuantType
    except ImportError as e:
        print(f"导入错误: {e}")
        print("请安装所需依赖:")
        print("  pip3 install transformers onnxruntime")
        return False

    model_id = 'Helsinki-NLP/opus-mt-en-zh'
    onnx_path = Path('assets/models/opus/opus-mt-en-zh.onnx')
    quantized_path = Path('assets/models/opus/opus-mt-en-zh-q8.onnx')

    print(f"加载模型: {model_id}")
    model = MarianMTModel.from_pretrained(model_id)
    tokenizer = MarianTokenizer.from_pretrained(model_id)

    print("导出 ONNX...")
    from transformers.onnx import export
    from onnx import load_model

    # Use the correct export API
    onnx_config = model.config

    # Export using torch.onnx with dynamo (newer API)
    import torch

    # Create dummy input
    dummy_input = tokenizer("Hello world", return_tensors="pt")

    # For encoder-decoder models like MarianMT, we need decoder_input_ids
    # This is typically the same as input_ids but shifted, starting with BOS token
    decoder_start_token_id = model.config.decoder_start_token_id
    batch_size = dummy_input['input_ids'].shape[0]
    decoder_input_ids = torch.full(
        (batch_size, 1),
        decoder_start_token_id,
        dtype=dummy_input['input_ids'].dtype
    )

    # Disable use_cache for ONNX export
    model.config.use_cache = False

    torch.onnx.export(
        model,
        (dummy_input['input_ids'], dummy_input['attention_mask'], decoder_input_ids),
        str(onnx_path),
        input_names=['input_ids', 'attention_mask', 'decoder_input_ids'],
        output_names=['logits'],
        dynamic_axes={
            'input_ids': {0: 'batch', 1: 'sequence'},
            'attention_mask': {0: 'batch', 1: 'sequence'},
            'decoder_input_ids': {0: 'batch', 1: 'sequence'},
            'logits': {0: 'batch', 1: 'sequence'}
        },
        opset_version=14,
        do_constant_folding=True,
        verbose=False
    )

    print(f"ONNX 模型已保存: {onnx_path}")

    print("执行 INT8 量化...")
    quantize_dynamic(
        model_input=str(onnx_path),
        model_output=str(quantized_path),
        weight_type=QuantType.QUInt8
    )

    print(f"量化模型已保存: {quantized_path}")

    # 显示文件大小
    original_size = onnx_path.stat().st_size / (1024 * 1024)
    quantized_size = quantized_path.stat().st_size / (1024 * 1024)
    reduction = (1 - quantized_size / original_size) * 100

    print(f"\n原始 ONNX 大小: {original_size:.1f} MB")
    print(f"量化后大小: {quantized_size:.1f} MB")
    print(f"压缩率: {reduction:.1f}%")

    return True

if __name__ == "__main__":
    print("=== Opus-MT ONNX 转换与量化 ===\n")

    # 创建输出目录
    os.makedirs('assets/models/opus', exist_ok=True)

    success = convert_and_quantize_model()

    if success:
        print("\n✓ 转换与量化成功!")
        sys.exit(0)
    else:
        print("\n✗ 转换失败!")
        sys.exit(1)
