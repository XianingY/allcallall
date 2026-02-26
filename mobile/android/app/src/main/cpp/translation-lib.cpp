// android/app/src/main/cpp/translation-lib.cpp
#include <jni.h>
#include <string>
#include <vector>
#include <android/log.h>
#include <sstream>

// Include model wrappers
#include "whisper/whisper_service.h"
#include "onnx/onnx_wrapper.h"
#include "tts/piper_wrapper.h"

#define LOG_TAG "TranslationLib"
#define LOGI(...) __android_log_print(ANDROID_LOG_INFO, LOG_TAG, __VA_ARGS__)
#define LOGE(...) __android_log_print(ANDROID_LOG_ERROR, LOG_TAG, __VA_ARGS__)

// Global model contexts
static whisper_wrapper_context* g_whisper_ctx = nullptr;
static OnnxModel* g_opus_en_zh = nullptr;
static OnnxModel* g_opus_zh_en = nullptr;
static PiperTTS* g_tts_zh = nullptr;
static PiperTTS* g_tts_en = nullptr;

static bool g_models_ready = false;

// Helper function declarations
std::vector<float> base64_decode_to_float(const char* input);
std::string build_result_json(const std::string& original, const std::string& translation, 
                              const std::vector<int16_t>& audio);
std::string audio_to_base64(const std::vector<int16_t>& audio);
std::string escape_json_string(const std::string& s);

extern "C" JNIEXPORT void JNICALL
Java_com_allcallall_TranslationModule_nativeInitialize(
    JNIEnv *env,
    jobject /* thiz */,
    jstring whisperPath,
    jstring opusPath,
    jstring ttsPath,
    jstring quantization
) {
    const char* whisper_model = env->GetStringUTFChars(whisperPath, 0);
    const char* opus_dir = env->GetStringUTFChars(opusPath, 0);
    const char* tts_dir = env->GetStringUTFChars(ttsPath, 0);
    const char* quant_type = env->GetStringUTFChars(quantization, 0);

    LOGI("Initializing translation models...");
    LOGI("Whisper: %s", whisper_model);
    LOGI("Opus: %s", opus_dir);
    LOGI("TTS: %s", tts_dir);
    LOGI("Quantization: %s", quant_type);

    // Load Whisper model
    g_whisper_ctx = whisper_wrapper_init(whisper_model);
    if (!g_whisper_ctx) {
        LOGE("Failed to load Whisper model");
    }

    // Load translation models (Opus-MT)
    // JS side copies assets into:
    //   <opus_dir>/en-zh/opus-mt-en-zh-q8.onnx (+ source.spm/target.spm)
    //   <opus_dir>/zh-en/opus-mt-zh-en-q8.onnx (+ source.spm/target.spm)
    const std::string opus_path = std::string(opus_dir);
    g_opus_en_zh = new OnnxModel(opus_path + "/en-zh/opus-mt-en-zh-q8.onnx");
    g_opus_zh_en = new OnnxModel(opus_path + "/zh-en/opus-mt-zh-en-q8.onnx");

    // Load TTS models (Piper)
    // JS side copies assets into:
    //   <tts_dir>/zh/zh_CN-huayan-medium.onnx (+ .json)
    //   <tts_dir>/en/en_US-amy-medium.onnx (+ .json)
    const std::string tts_path = std::string(tts_dir);
    g_tts_zh = new PiperTTS(tts_path + "/zh/zh_CN-huayan-medium.onnx");
    g_tts_en = new PiperTTS(tts_path + "/en/en_US-amy-medium.onnx");

    env->ReleaseStringUTFChars(whisperPath, whisper_model);
    env->ReleaseStringUTFChars(opusPath, opus_dir);
    env->ReleaseStringUTFChars(ttsPath, tts_dir);
    env->ReleaseStringUTFChars(quantization, quant_type);

    g_models_ready = (g_whisper_ctx != nullptr);
    g_models_ready = g_models_ready && (g_opus_en_zh != nullptr) && (g_opus_zh_en != nullptr);
    g_models_ready = g_models_ready && (g_tts_zh != nullptr) && (g_tts_en != nullptr);

    LOGI("Translation models initialized (ready=%s)", g_models_ready ? "true" : "false");
}

extern "C" JNIEXPORT jstring JNICALL
Java_com_allcallall_TranslationModule_nativeTranslateAudio(
    JNIEnv *env,
    jobject /* thiz */,
    jstring audioDataBase64,
    jstring targetLanguage
) {
    if (!g_models_ready || !g_whisper_ctx) {
        LOGE("Translation models not initialized");
        return env->NewStringUTF("{\"error\":\"Translation models not initialized\"}");
    }

    const char* audio_b64 = env->GetStringUTFChars(audioDataBase64, 0);
    const char* target_lang = env->GetStringUTFChars(targetLanguage, 0);

    LOGI("Translating audio to: %s", target_lang);

    // 1. Decode base64 audio data
    std::vector<float> audio_data = base64_decode_to_float(audio_b64);
    LOGI("Decoded %zu audio samples", audio_data.size());

    // 2. Speech recognition with Whisper
    std::string recognized = whisper_wrapper_transcribe(g_whisper_ctx, audio_data);
    LOGI("Recognized: %s", recognized.c_str());

    // 3. Detect source language
    std::string detected_lang = whisper_wrapper_get_language(g_whisper_ctx);
    bool source_is_english = (detected_lang == "en") || is_english_text(recognized);
    LOGI("Source language detected as: %s (whisper: %s)", 
         source_is_english ? "English" : "Chinese", detected_lang.c_str());

    // 4. Translate based on source and target language
    std::string translation;
    std::string target_str(target_lang);
    
    if (source_is_english && target_str == "zh") {
        // English to Chinese
        if (g_opus_en_zh && g_opus_en_zh->isReady()) {
            translation = g_opus_en_zh->translate(recognized);
        } else {
            translation = recognized;
        }
    } else if (!source_is_english && target_str == "en") {
        // Chinese to English
        if (g_opus_zh_en && g_opus_zh_en->isReady()) {
            translation = g_opus_zh_en->translate(recognized);
        } else {
            translation = recognized;
        }
    } else {
        // No translation needed or same language
        translation = recognized;
    }
    LOGI("Translation: %s", translation.c_str());

    // 5. TTS synthesis
    // Temporarily disabled for call subtitles path because some Android devices
    // crash inside espeak_TextToPhonemes. We only need subtitle text here.
    std::vector<int16_t> audio_out;
    const bool kEnableTtsSynthesis = false;
    if (kEnableTtsSynthesis) {
        if (target_str == "zh" && g_tts_zh && g_tts_zh->isReady()) {
            audio_out = g_tts_zh->synthesize(translation);
        } else if (target_str == "en" && g_tts_en && g_tts_en->isReady()) {
            audio_out = g_tts_en->synthesize(translation);
        }
    }
    LOGI("Synthesized %zu audio samples", audio_out.size());

    // 6. Build and return JSON result
    std::string result = build_result_json(recognized, translation, audio_out);

    env->ReleaseStringUTFChars(audioDataBase64, audio_b64);
    env->ReleaseStringUTFChars(targetLanguage, target_lang);

    return env->NewStringUTF(result.c_str());
}

extern "C" JNIEXPORT void JNICALL
Java_com_allcallall_TranslationModule_nativeCleanup(
    JNIEnv * /* env */,
    jobject /* thiz */
) {
    LOGI("Cleaning up translation models...");

    if (g_whisper_ctx) {
        whisper_wrapper_free(g_whisper_ctx);
        g_whisper_ctx = nullptr;
    }

    delete g_opus_en_zh;
    delete g_opus_zh_en;
    delete g_tts_zh;
    delete g_tts_en;
    
    g_opus_en_zh = nullptr;
    g_opus_zh_en = nullptr;
    g_tts_zh = nullptr;
    g_tts_en = nullptr;

    LOGI("Translation models cleaned up");
}

// Helper function: Escape JSON string
std::string escape_json_string(const std::string& s) {
    std::string result;
    result.reserve(s.size() * 2);
    for (char c : s) {
        switch (c) {
            case '"':  result += "\\\""; break;
            case '\\': result += "\\\\"; break;
            case '\b': result += "\\b";  break;
            case '\f': result += "\\f";  break;
            case '\n': result += "\\n";  break;
            case '\r': result += "\\r";  break;
            case '\t': result += "\\t";  break;
            default:   result += c;      break;
        }
    }
    return result;
}

// Helper function: Decode base64 to float array
std::vector<float> base64_decode_to_float(const char* input) {
    // Base64 decoding table
    static const uint8_t d[] = {
        0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
        0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
        0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 62, 0, 0, 0, 63,
        52, 53, 54, 55, 56, 57, 58, 59, 60, 61, 0, 0, 0, 0, 0, 0,
        0, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14,
        15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 0, 0, 0, 0, 0,
        0, 26, 27, 28, 29, 30, 31, 32, 33, 34, 35, 36, 37, 38, 39, 40,
        41, 42, 43, 44, 45, 46, 47, 48, 49, 50, 51, 0, 0, 0, 0, 0
    };
    
    size_t len = strlen(input);
    if (len == 0) return {};
    
    // Calculate output size
    size_t out_len = (len / 4) * 3;
    if (input[len - 1] == '=') out_len--;
    if (len >= 2 && input[len - 2] == '=') out_len--;
    
    std::vector<uint8_t> bytes(out_len);
    
    for (size_t i = 0, j = 0; i < len;) {
        uint32_t a = input[i] == '=' ? 0 : d[(int)input[i]]; i++;
        uint32_t b = input[i] == '=' ? 0 : d[(int)input[i]]; i++;
        uint32_t c = input[i] == '=' ? 0 : d[(int)input[i]]; i++;
        uint32_t e = input[i] == '=' ? 0 : d[(int)input[i]]; i++;
        
        uint32_t triple = (a << 18) + (b << 12) + (c << 6) + e;
        
        if (j < out_len) bytes[j++] = (triple >> 16) & 0xFF;
        if (j < out_len) bytes[j++] = (triple >> 8) & 0xFF;
        if (j < out_len) bytes[j++] = triple & 0xFF;
    }
    
    // Convert bytes to floats (assuming little-endian float32)
    size_t num_floats = bytes.size() / sizeof(float);
    std::vector<float> result(num_floats);
    memcpy(result.data(), bytes.data(), num_floats * sizeof(float));
    
    return result;
}

// Helper function: Build JSON result
std::string build_result_json(const std::string& original, const std::string& translation,
                              const std::vector<int16_t>& audio) {
    std::stringstream ss;
    ss << "{";
    ss << "\"originalText\":\"" << escape_json_string(original) << "\",";
    ss << "\"translatedText\":\"" << escape_json_string(translation) << "\",";
    ss << "\"confidence\":" << 0.95 << ",";
    ss << "\"audioBase64\":\"" << audio_to_base64(audio) << "\"";
    ss << "}";
    return ss.str();
}

// Helper function: Convert audio samples to base64
std::string audio_to_base64(const std::vector<int16_t>& audio) {
    static const char encode_table[] = 
        "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";
    
    const uint8_t* data = reinterpret_cast<const uint8_t*>(audio.data());
    size_t len = audio.size() * sizeof(int16_t);
    
    std::string result;
    result.reserve(((len + 2) / 3) * 4);
    
    for (size_t i = 0; i < len; i += 3) {
        uint32_t triple = (data[i] << 16);
        if (i + 1 < len) triple |= (data[i + 1] << 8);
        if (i + 2 < len) triple |= data[i + 2];
        
        result += encode_table[(triple >> 18) & 0x3F];
        result += encode_table[(triple >> 12) & 0x3F];
        result += (i + 1 < len) ? encode_table[(triple >> 6) & 0x3F] : '=';
        result += (i + 2 < len) ? encode_table[triple & 0x3F] : '=';
    }
    
    return result;
}
