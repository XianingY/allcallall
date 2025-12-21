# Opus Translation Models

## 概述
本目录包含 Opus 机器翻译模型，支持中英文双向翻译。

## 目录结构

### en-zh/
**English → Chinese Translation Model**
- 英文到中文翻译
- 包含原始模型和量化版本
- 文件大小: ~424MB (原始) / ~108MB (量化)

### zh-en/
**Chinese → English Translation Model**
- 中文到英文翻译
- 原始 PyTorch 模型
- 文件大小: ~300MB

## 量化优化

量化版本使用 INT8 格式，具有以下优势:
- 文件大小减少 70%+
- 推理速度提升 2-3x
- 精度损失 < 5%

## 使用建议

### 开发阶段
使用原始模型 (pytorch_model.bin) 进行开发和测试。

### 生产部署
使用量化模型 (*-q8.onnx) 以获得更好的性能。

## 文件说明

| 文件 | 用途 |
|------|------|
| *.onnx | ONNX 格式模型 |
| *-q8.onnx | 量化版本 |
| *.bin | PyTorch 原始模型 |
| *.spm | SentencePiece 词表 |
| *.json | 配置文件 |

## 下一步

1. **量化 zh-en 模型** (如果需要)
   ```bash
   python3 quantize_opus.py
   # 修改模型ID为 Helsinki-NLP/opus-mt-zh-en
   ```

2. **集成到应用**
   - React Native JNI 接口
   - ONNX Runtime 推理
   - 模型缓存管理

## 参考
- [Opus 项目](https://github.com/Helsinki-NLP/Opus-MT)
- [Transformers MarianMT](https://huggingface.co/docs/transformers/model_doc/marian)
