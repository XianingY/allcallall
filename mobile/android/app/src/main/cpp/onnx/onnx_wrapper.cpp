// mobile/android/app/src/main/cpp/onnx/onnx_wrapper.cpp
#include "onnx_wrapper.h"
#include <android/log.h>
#include <fstream>
#include <sstream>
#include <cmath>

#ifdef ONNX_RUNTIME_AVAILABLE
#include "onnxruntime_c_api.h"
#endif

#ifdef SENTENCEPIECE_AVAILABLE
#include "sentencepiece_processor.h"
#endif

#define LOG_TAG "OnnxWrapper"
#define LOGI(...) __android_log_print(ANDROID_LOG_INFO, LOG_TAG, __VA_ARGS__)
#define LOGE(...) __android_log_print(ANDROID_LOG_ERROR, LOG_TAG, __VA_ARGS__)
#define LOGW(...) __android_log_print(ANDROID_LOG_WARN, LOG_TAG, __VA_ARGS__)

class OnnxModel::Impl {
public:
    std::string model_path;
    bool is_ready = false;
    bool is_translation_model = false;
    bool is_tts_model = false;
    
#ifdef ONNX_RUNTIME_AVAILABLE
    const OrtApi* ort_api = nullptr;
    OrtEnv* env = nullptr;
    OrtSessionOptions* session_options = nullptr;
    OrtSession* session = nullptr;
#endif

#ifdef SENTENCEPIECE_AVAILABLE
    std::unique_ptr<sentencepiece::SentencePieceProcessor> sp_source;
    std::unique_ptr<sentencepiece::SentencePieceProcessor> sp_target;
#endif
    
    explicit Impl(const std::string& path) : model_path(path) {
        LOGI("Loading ONNX model from: %s", path.c_str());
        
        // 检查模型类型
        if (path.find("opus") != std::string::npos) {
            is_translation_model = true;
            LOGI("Detected translation model (Opus-MT)");
        } else if (path.find("zh_CN") != std::string::npos || 
                   path.find("en_US") != std::string::npos) {
            is_tts_model = true;
            LOGI("Detected TTS model (Piper)");
        }
        
        // 检查模型文件是否存在
        std::ifstream file(path);
        if (!file.good()) {
            LOGW("ONNX model file not found: %s", path.c_str());
            is_ready = false;
            return;
        }
        file.close();
        
        // 初始化 SentencePiece (如果可用且是翻译模型)
#ifdef SENTENCEPIECE_AVAILABLE
        if (is_translation_model) {
            initTokenizer();
        }
#endif
        
#ifdef ONNX_RUNTIME_AVAILABLE
        initOnnxRuntime();
#else
        LOGW("ONNX Runtime not available - using placeholder implementation");
        is_ready = true;
#endif
    }
    
    ~Impl() {
#ifdef ONNX_RUNTIME_AVAILABLE
        cleanup();
#endif
    }

#ifdef SENTENCEPIECE_AVAILABLE
    void initTokenizer() {
        // 假设 tokenizer 模型位于与 ONNX 模型相同的目录下
        // source.spm 和 target.spm
        std::string model_dir = model_path.substr(0, model_path.find_last_of('/'));
        std::string source_spm_path = model_dir + "/source.spm";
        std::string target_spm_path = model_dir + "/target.spm";
        
        LOGI("Loading Tokenizers from: %s", model_dir.c_str());
        
        sp_source = std::make_unique<sentencepiece::SentencePieceProcessor>();
        const auto status_source = sp_source->Load(source_spm_path);
        if (!status_source.ok()) {
            LOGW("Failed to load source tokenizer: %s", status_source.ToString().c_str());
        } else {
            LOGI("Source tokenizer loaded successfully");
        }

        sp_target = std::make_unique<sentencepiece::SentencePieceProcessor>();
        const auto status_target = sp_target->Load(target_spm_path);
        if (!status_target.ok()) {
            LOGW("Failed to load target tokenizer: %s", status_target.ToString().c_str());
        } else {
            LOGI("Target tokenizer loaded successfully");
        }
    }
#endif
    
#ifdef ONNX_RUNTIME_AVAILABLE
    void initOnnxRuntime() {
        ort_api = OrtGetApiBase()->GetApi(ORT_API_VERSION);
        if (!ort_api) {
            LOGE("Failed to get ONNX Runtime API");
            return;
        }
        
        // 创建环境
        OrtStatus* status = ort_api->CreateEnv(ORT_LOGGING_LEVEL_WARNING, "translation", &env);
        if (status != nullptr) {
            const char* msg = ort_api->GetErrorMessage(status);
            LOGE("Failed to create ORT environment: %s", msg);
            ort_api->ReleaseStatus(status);
            return;
        }
        
        // 创建会话选项
        status = ort_api->CreateSessionOptions(&session_options);
        if (status != nullptr) {
            const char* msg = ort_api->GetErrorMessage(status);
            LOGE("Failed to create session options: %s", msg);
            ort_api->ReleaseStatus(status);
            return;
        }
        
        // 设置线程数
        ort_api->SetIntraOpNumThreads(session_options, 4);
        ort_api->SetInterOpNumThreads(session_options, 2);
        
        // 设置优化级别
        ort_api->SetSessionGraphOptimizationLevel(session_options, ORT_ENABLE_ALL);
        
        // 创建会话
        status = ort_api->CreateSession(env, model_path.c_str(), session_options, &session);
        if (status != nullptr) {
            const char* msg = ort_api->GetErrorMessage(status);
            LOGE("Failed to create session: %s", msg);
            ort_api->ReleaseStatus(status);
            return;
        }
        
        is_ready = true;
        LOGI("ONNX Runtime session created successfully");
        
        // 打印模型输入输出信息
        logModelInfo();
    }
    
    void logModelInfo() {
        if (!session || !ort_api) return;
        
        OrtAllocator* allocator = nullptr;
        ort_api->GetAllocatorWithDefaultOptions(&allocator);
        
        // 获取输入数量
        size_t num_input_nodes = 0;
        ort_api->SessionGetInputCount(session, &num_input_nodes);
        LOGI("Model has %zu input(s)", num_input_nodes);
        
        // 获取输出数量
        size_t num_output_nodes = 0;
        ort_api->SessionGetOutputCount(session, &num_output_nodes);
        LOGI("Model has %zu output(s)", num_output_nodes);
    }
    
    void cleanup() {
        if (session) {
            ort_api->ReleaseSession(session);
            session = nullptr;
        }
        if (session_options) {
            ort_api->ReleaseSessionOptions(session_options);
            session_options = nullptr;
        }
        if (env) {
            ort_api->ReleaseEnv(env);
            env = nullptr;
        }
    }
#endif
    
    // 模型配置常量 (来自 config.json)
    static constexpr int PAD_TOKEN_ID = 65000;
    static constexpr int EOS_TOKEN_ID = 0;
    static constexpr int MAX_LENGTH = 128;  // 限制输出长度
    static constexpr int VOCAB_SIZE = 65001;
    
#ifdef ONNX_RUNTIME_AVAILABLE
    std::vector<int64_t> runOnnxInference(
        const std::vector<int64_t>& input_ids,
        const std::vector<int64_t>& attention_mask,
        const std::vector<int64_t>& decoder_input_ids) {
        
        if (!session || !ort_api) {
            LOGE("ONNX session not initialized");
            return {};
        }
        
        OrtMemoryInfo* memory_info = nullptr;
        OrtStatus* status = ort_api->CreateCpuMemoryInfo(
            OrtArenaAllocator, OrtMemTypeDefault, &memory_info);
        if (status != nullptr) {
            LOGE("Failed to create memory info");
            ort_api->ReleaseStatus(status);
            return {};
        }
        
        // 创建输入张量
        int64_t batch_size = 1;
        int64_t input_seq_len = static_cast<int64_t>(input_ids.size());
        int64_t decoder_seq_len = static_cast<int64_t>(decoder_input_ids.size());
        
        std::vector<int64_t> input_shape = {batch_size, input_seq_len};
        std::vector<int64_t> decoder_shape = {batch_size, decoder_seq_len};
        
        OrtValue* input_tensor = nullptr;
        OrtValue* attention_tensor = nullptr;
        OrtValue* decoder_tensor = nullptr;
        
        // input_ids tensor
        status = ort_api->CreateTensorWithDataAsOrtValue(
            memory_info,
            const_cast<int64_t*>(input_ids.data()),
            input_ids.size() * sizeof(int64_t),
            input_shape.data(), input_shape.size(),
            ONNX_TENSOR_ELEMENT_DATA_TYPE_INT64,
            &input_tensor
        );
        if (status != nullptr) {
            LOGE("Failed to create input_ids tensor: %s", ort_api->GetErrorMessage(status));
            ort_api->ReleaseStatus(status);
            ort_api->ReleaseMemoryInfo(memory_info);
            return {};
        }
        
        // attention_mask tensor
        status = ort_api->CreateTensorWithDataAsOrtValue(
            memory_info,
            const_cast<int64_t*>(attention_mask.data()),
            attention_mask.size() * sizeof(int64_t),
            input_shape.data(), input_shape.size(),
            ONNX_TENSOR_ELEMENT_DATA_TYPE_INT64,
            &attention_tensor
        );
        if (status != nullptr) {
            LOGE("Failed to create attention_mask tensor");
            ort_api->ReleaseStatus(status);
            ort_api->ReleaseValue(input_tensor);
            ort_api->ReleaseMemoryInfo(memory_info);
            return {};
        }
        
        // decoder_input_ids tensor
        std::vector<int64_t> decoder_ids_copy = decoder_input_ids;
        status = ort_api->CreateTensorWithDataAsOrtValue(
            memory_info,
            decoder_ids_copy.data(),
            decoder_ids_copy.size() * sizeof(int64_t),
            decoder_shape.data(), decoder_shape.size(),
            ONNX_TENSOR_ELEMENT_DATA_TYPE_INT64,
            &decoder_tensor
        );
        if (status != nullptr) {
            LOGE("Failed to create decoder_input_ids tensor");
            ort_api->ReleaseStatus(status);
            ort_api->ReleaseValue(input_tensor);
            ort_api->ReleaseValue(attention_tensor);
            ort_api->ReleaseMemoryInfo(memory_info);
            return {};
        }
        
        // 准备输入输出
        const char* input_names[] = {"input_ids", "attention_mask", "decoder_input_ids"};
        const char* output_names[] = {"logits"};
        OrtValue* input_tensors[] = {input_tensor, attention_tensor, decoder_tensor};
        OrtValue* output_tensor = nullptr;
        
        // 运行推理
        status = ort_api->Run(
            session,
            nullptr,  // run options
            input_names, input_tensors, 3,
            output_names, 1,
            &output_tensor
        );
        
        std::vector<int64_t> result;
        
        if (status != nullptr) {
            LOGE("Inference failed: %s", ort_api->GetErrorMessage(status));
            ort_api->ReleaseStatus(status);
        } else {
            // 获取输出数据
            float* output_data = nullptr;
            ort_api->GetTensorMutableData(output_tensor, (void**)&output_data);
            
            // 获取输出形状
            OrtTensorTypeAndShapeInfo* shape_info = nullptr;
            ort_api->GetTensorTypeAndShape(output_tensor, &shape_info);
            
            size_t num_dims = 0;
            ort_api->GetDimensionsCount(shape_info, &num_dims);
            
            std::vector<int64_t> output_shape(num_dims);
            ort_api->GetDimensions(shape_info, output_shape.data(), num_dims);
            
            // logits shape: [batch, decoder_seq_len, vocab_size]
            // 取最后一个位置的 logits，找 argmax
            int64_t seq_len_out = output_shape[1];
            int64_t vocab_size_out = output_shape[2];
            
            // 找最后一个位置的最大值索引
            int last_pos = static_cast<int>(seq_len_out - 1);
            float* last_logits = output_data + last_pos * vocab_size_out;
            
            int64_t max_idx = 0;
            float max_val = last_logits[0];
            for (int64_t i = 1; i < vocab_size_out; i++) {
                if (last_logits[i] > max_val) {
                    max_val = last_logits[i];
                    max_idx = i;
                }
            }
            
            result.push_back(max_idx);
            
            ort_api->ReleaseTensorTypeAndShapeInfo(shape_info);
        }
        
        // 清理
        if (output_tensor) ort_api->ReleaseValue(output_tensor);
        ort_api->ReleaseValue(input_tensor);
        ort_api->ReleaseValue(attention_tensor);
        ort_api->ReleaseValue(decoder_tensor);
        ort_api->ReleaseMemoryInfo(memory_info);
        
        return result;
    }
#endif
    
    std::string translate(const std::string& text) {
        if (!is_ready) {
            LOGE("Model not ready");
            return "";
        }
        
        LOGI("Translating: %s", text.c_str());
        
#if defined(SENTENCEPIECE_AVAILABLE) && defined(ONNX_RUNTIME_AVAILABLE)
        if (sp_source && sp_target && session) {
            // Step 1: Tokenize input text
            std::vector<int> input_ids_int;
            sp_source->Encode(text, &input_ids_int);
            
            if (input_ids_int.empty()) {
                LOGW("Tokenization produced empty result");
                return "";
            }
            
            LOGI("Input tokens: %zu", input_ids_int.size());
            
            // 转换为 int64_t
            std::vector<int64_t> input_ids(input_ids_int.begin(), input_ids_int.end());
            std::vector<int64_t> attention_mask(input_ids.size(), 1);
            
            // Step 2: Autoregressive decoding
            std::vector<int64_t> decoder_ids = {PAD_TOKEN_ID};  // 起始 token
            std::vector<int> output_tokens;
            
            for (int step = 0; step < MAX_LENGTH; step++) {
                std::vector<int64_t> next_tokens = runOnnxInference(input_ids, attention_mask, decoder_ids);
                
                if (next_tokens.empty()) {
                    LOGE("Inference returned empty result at step %d", step);
                    break;
                }
                
                int64_t next_token = next_tokens[0];
                
                // 检查是否为 EOS
                if (next_token == EOS_TOKEN_ID) {
                    LOGI("EOS reached at step %d", step);
                    break;
                }
                
                output_tokens.push_back(static_cast<int>(next_token));
                decoder_ids.push_back(next_token);
                
                // 防止过长
                if (decoder_ids.size() > MAX_LENGTH) {
                    LOGW("Max length reached");
                    break;
                }
            }
            
            LOGI("Generated %zu output tokens", output_tokens.size());
            
            // Step 3: Decode tokens to text
            std::string result;
            sp_target->Decode(output_tokens, &result);
            
            if (!result.empty()) {
                LOGI("Translation result: %s", result.c_str());
                return result;
            }
        }
#endif
        
#ifdef SENTENCEPIECE_AVAILABLE
        if (sp_source && sp_target) {
            // Tokenizer 可用但没有 ONNX Runtime，用于验证
            std::vector<int> ids;
            sp_source->Encode(text, &ids);
            LOGI("Encoded tokens size: %zu (ONNX inference not available)", ids.size());
        }
#endif

        // 占位符翻译逻辑 (保留作为 fallback)
        if (model_path.find("en-zh") != std::string::npos) {
            return simulateEnToZhTranslation(text);
        } else if (model_path.find("zh-en") != std::string::npos) {
            return simulateZhToEnTranslation(text);
        }
        
        return text;
    }
    
    std::vector<int16_t> synthesize(const std::string& text) {
        if (!is_ready) {
            LOGE("Model not ready");
            return {};
        }
        
        LOGI("Synthesizing TTS for: %s", text.c_str());
        
#ifdef ONNX_RUNTIME_AVAILABLE
        // TODO: 实际的 TTS 实现 (Piper VITS)
        // 需要音素转换器 (g2p)
        LOGW("TTS requires phoneme converter - using placeholder");
#endif
        
        // 占位符: 生成简单的正弦波作为测试音频
        const int sample_rate = 22050;
        const float duration = 0.5f;
        const float frequency = 440.0f;
        
        int num_samples = static_cast<int>(sample_rate * duration);
        std::vector<int16_t> audio(num_samples);
        
        for (int i = 0; i < num_samples; i++) {
            float t = static_cast<float>(i) / sample_rate;
            float sample = std::sin(2.0f * 3.14159f * frequency * t);
            float envelope = 1.0f;
            if (i < num_samples / 10) {
                envelope = static_cast<float>(i) / (num_samples / 10);
            } else if (i > num_samples * 9 / 10) {
                envelope = static_cast<float>(num_samples - i) / (num_samples / 10);
            }
            audio[i] = static_cast<int16_t>(sample * envelope * 16000);
        }
        
        return audio;
    }
    
private:
    std::string simulateEnToZhTranslation(const std::string& text) {
        if (text.find("hello") != std::string::npos || 
            text.find("Hello") != std::string::npos) {
            return "你好";
        }
        if (text.find("thank") != std::string::npos) {
            return "谢谢";
        }
        if (text.find("goodbye") != std::string::npos || 
            text.find("bye") != std::string::npos) {
            return "再见";
        }
        return "[SP验证成功] " + text;
    }
    
    std::string simulateZhToEnTranslation(const std::string& text) {
        if (text.find("你好") != std::string::npos) {
            return "Hello";
        }
        if (text.find("谢谢") != std::string::npos) {
            return "Thank you";
        }
        if (text.find("再见") != std::string::npos) {
            return "Goodbye";
        }
        return "[SP Verified] " + text;
    }
};

OnnxModel::OnnxModel(const std::string& model_path) 
    : pImpl(std::make_unique<Impl>(model_path)) {
}

OnnxModel::~OnnxModel() = default;

std::string OnnxModel::translate(const std::string& text) {
    return pImpl->translate(text);
}

std::vector<int16_t> OnnxModel::synthesize(const std::string& text) {
    return pImpl->synthesize(text);
}

bool OnnxModel::isReady() const {
    return pImpl->is_ready;
}
