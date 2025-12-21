#ifndef WHISPER_SERVICE_H
#define WHISPER_SERVICE_H

#include <string>
#include <vector>

// 前向声明 - 避免依赖 whisper.cpp 头文件
struct whisper_wrapper_context;

// 初始化 Whisper 模型
// path: 模型文件路径 (.bin 或 .ggml)
// 返回: 上下文指针，失败时返回 nullptr
whisper_wrapper_context* whisper_wrapper_init(const char* path);

// 转录音频
// ctx: Whisper 上下文
// audio: 16kHz float32 PCM 音频数据
// 返回: 转录的文本
std::string whisper_wrapper_transcribe(whisper_wrapper_context* ctx, const std::vector<float>& audio);

// 获取检测到的语言代码
// ctx: Whisper 上下文
// 返回: 语言代码 (如 "en", "zh")
std::string whisper_wrapper_get_language(whisper_wrapper_context* ctx);

// 释放上下文
void whisper_wrapper_free(whisper_wrapper_context* ctx);

// 辅助函数：检测文本是否主要为英文
bool is_english_text(const std::string& text);

#endif // WHISPER_SERVICE_H
