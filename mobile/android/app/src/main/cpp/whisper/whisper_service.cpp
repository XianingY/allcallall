// mobile/android/app/src/main/cpp/whisper/whisper_service.cpp
#include "whisper_service.h"  // 我们自己的 wrapper 头文件
#include <android/log.h>
#include <cstring>
#include <cctype>
#include <algorithm>

#define LOG_TAG "WhisperWrapper"
#define LOGI(...) __android_log_print(ANDROID_LOG_INFO, LOG_TAG, __VA_ARGS__)
#define LOGE(...) __android_log_print(ANDROID_LOG_ERROR, LOG_TAG, __VA_ARGS__)

#ifdef WHISPER_AVAILABLE
// =============================================
// 完整 whisper.cpp 实现
// =============================================
// 包含原始 whisper.cpp 库的头文件 (从 whisper.cpp/include)
extern "C" {
    #include <whisper.h>
}

struct whisper_wrapper_context {
    struct whisper_context* ctx;
    std::string last_language;
    bool is_initialized;
};

whisper_wrapper_context* whisper_wrapper_init(const char* path) {
    LOGI("Loading Whisper model from: %s", path);
    
    auto* wrapper = new whisper_wrapper_context();
    wrapper->ctx = nullptr;
    wrapper->last_language = "auto";
    wrapper->is_initialized = false;
    
    // 使用默认参数初始化
    struct whisper_context_params cparams = whisper_context_default_params();
    cparams.use_gpu = false;  // Android 上暂时禁用 GPU
    cparams.flash_attn = false;
    
    wrapper->ctx = whisper_init_from_file_with_params(path, cparams);
    
    if (!wrapper->ctx) {
        LOGE("Failed to load Whisper model from: %s", path);
        delete wrapper;
        return nullptr;
    }
    
    wrapper->is_initialized = true;
    LOGI("Whisper model loaded successfully, multilingual: %d", 
         whisper_is_multilingual(wrapper->ctx));
    
    return wrapper;
}

std::string whisper_wrapper_transcribe(whisper_wrapper_context* ctx, const std::vector<float>& audio) {
    if (!ctx || !ctx->ctx || !ctx->is_initialized) {
        LOGE("Whisper context not initialized");
        return "";
    }
    
    LOGI("Transcribing audio with %zu samples (%.2f seconds)", 
         audio.size(), (float)audio.size() / WHISPER_SAMPLE_RATE);
    
    // 设置 whisper 参数
    struct whisper_full_params params = whisper_full_default_params(WHISPER_SAMPLING_GREEDY);
    params.language = "auto";  // 自动检测语言
    params.n_threads = 4;
    params.translate = false;  // 只转录，不翻译
    params.print_progress = false;
    params.print_timestamps = false;
    params.print_realtime = false;
    params.print_special = false;
    params.single_segment = false;
    params.no_timestamps = true;
    
    // 运行转录
    if (whisper_full(ctx->ctx, params, audio.data(), audio.size()) != 0) {
        LOGE("Whisper transcription failed");
        return "";
    }
    
    // 获取检测到的语言
    int lang_id = whisper_full_lang_id(ctx->ctx);
    if (lang_id >= 0) {
        const char* lang_str = whisper_lang_str(lang_id);
        if (lang_str) {
            ctx->last_language = lang_str;
            LOGI("Detected language: %s (id: %d)", lang_str, lang_id);
        }
    }
    
    // 收集所有分段的文本
    std::string result;
    const int n_segments = whisper_full_n_segments(ctx->ctx);
    LOGI("Got %d segments", n_segments);
    
    for (int i = 0; i < n_segments; ++i) {
        const char* text = whisper_full_get_segment_text(ctx->ctx, i);
        if (text) {
            result += text;
        }
    }
    
    // 去除前后空白
    size_t start = result.find_first_not_of(" \t\n\r");
    if (start == std::string::npos) {
        return "";
    }
    size_t end = result.find_last_not_of(" \t\n\r");
    result = result.substr(start, end - start + 1);
    
    LOGI("Transcription result (%zu chars): %s", result.length(), result.c_str());
    return result;
}

std::string whisper_wrapper_get_language(whisper_wrapper_context* ctx) {
    if (!ctx) return "en";
    return ctx->last_language;
}

void whisper_wrapper_free(whisper_wrapper_context* ctx) {
    if (ctx) {
        LOGI("Freeing Whisper context");
        if (ctx->ctx) {
            whisper_free(ctx->ctx);
        }
        delete ctx;
    }
}

#else
// =============================================
// 占位符实现（当 whisper.cpp 不可用时）
// =============================================

struct whisper_wrapper_context {
    std::string last_language;
    bool is_initialized;
};

whisper_wrapper_context* whisper_wrapper_init(const char* path) {
    LOGI("Loading Whisper model from: %s (PLACEHOLDER - whisper.cpp not integrated)", path);
    
    auto* ctx = new whisper_wrapper_context();
    ctx->last_language = "en";
    ctx->is_initialized = true;
    
    LOGI("Whisper placeholder initialized");
    return ctx;
}

std::string whisper_wrapper_transcribe(whisper_wrapper_context* ctx, const std::vector<float>& audio) {
    if (!ctx || !ctx->is_initialized) {
        LOGE("Whisper context not initialized");
        return "";
    }
    
    LOGI("Transcribing audio with %zu samples (placeholder)", audio.size());
    
    // 占位符响应
    return "Hello, this is a placeholder transcription. The actual whisper.cpp integration is not available.";
}

std::string whisper_wrapper_get_language(whisper_wrapper_context* ctx) {
    if (!ctx) return "en";
    return ctx->last_language;
}

void whisper_wrapper_free(whisper_wrapper_context* ctx) {
    if (ctx) {
        LOGI("Freeing Whisper context");
        delete ctx;
    }
}

#endif

// =============================================
// 通用辅助函数
// =============================================

bool is_english_text(const std::string& text) {
    if (text.empty()) return true;
    
    int ascii_count = 0;
    int non_ascii_count = 0;
    
    for (unsigned char c : text) {
        if (c < 128) {
            if (std::isalpha(c)) {
                ascii_count++;
            }
        } else {
            non_ascii_count++;
        }
    }
    
    // 如果超过50%的字母字符是 ASCII，则认为是英文
    int total = ascii_count + non_ascii_count;
    if (total == 0) return true;
    
    return (float)ascii_count / total > 0.5f;
}
