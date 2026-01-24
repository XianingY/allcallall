// mobile/android/app/src/main/cpp/tts/piper_wrapper.cpp
#include "piper_wrapper.h"
#include <android/log.h>
#include <fstream>
#include <sstream>
#include <cmath>
#include <unordered_map>
#include <vector>

#ifdef ONNX_RUNTIME_AVAILABLE
#include "onnxruntime_c_api.h"
#endif

#ifdef ESPEAK_NG_AVAILABLE
#include "espeak-ng/speak_lib.h"
#endif

#define LOG_TAG "PiperTTS"
#define LOGI(...) __android_log_print(ANDROID_LOG_INFO, LOG_TAG, __VA_ARGS__)
#define LOGE(...) __android_log_print(ANDROID_LOG_ERROR, LOG_TAG, __VA_ARGS__)
#define LOGW(...) __android_log_print(ANDROID_LOG_WARN, LOG_TAG, __VA_ARGS__)

class PiperTTS::Impl {
public:
    std::string model_path;
    std::string config_path;
    bool is_ready = false;
    int sample_rate = 22050;  // Piper 默认采样率
    std::string language;
    std::string espeak_voice;
    
    // 模型推理参数
    float noise_scale = 0.667f;
    float length_scale = 1.0f;
    float noise_w = 0.8f;
    
    // Phoneme ID 映射
    std::unordered_map<std::string, int> phoneme_to_id;
    
#ifdef ONNX_RUNTIME_AVAILABLE
    const OrtApi* ort_api = nullptr;
    OrtEnv* env = nullptr;
    OrtSessionOptions* session_options = nullptr;
    OrtSession* session = nullptr;
#endif

#ifdef ESPEAK_NG_AVAILABLE
    bool espeak_initialized = false;
#endif
    
    explicit Impl(const std::string& path) : model_path(path) {
        LOGI("Loading Piper TTS model from: %s", path.c_str());
        
        // 检测语言和配置路径
        config_path = path + ".json";
        
        if (path.find("zh_CN") != std::string::npos) {
            language = "zh";
            espeak_voice = "cmn";  // 普通话
            LOGI("Detected Chinese TTS model");
        } else if (path.find("en_US") != std::string::npos) {
            language = "en";
            espeak_voice = "en-us";
            LOGI("Detected English TTS model");
        } else {
            language = "en";
            espeak_voice = "en";
        }
        
        // 检查模型文件是否存在
        std::ifstream file(path);
        if (!file.good()) {
            LOGW("Piper TTS model file not found: %s", path.c_str());
            is_ready = true;  // 使用占位符
            return;
        }
        file.close();
        
        // 加载配置文件中的 phoneme_id_map
        loadConfig();
        
#ifdef ESPEAK_NG_AVAILABLE
        initEspeak();
#endif

#ifdef ONNX_RUNTIME_AVAILABLE
        initOnnxRuntime();
#endif
        
        if (!phoneme_to_id.empty()) {
            LOGI("Piper TTS initialized with %zu phonemes", phoneme_to_id.size());
        }
    }
    
    ~Impl() {
#ifdef ONNX_RUNTIME_AVAILABLE
        cleanup();
#endif
#ifdef ESPEAK_NG_AVAILABLE
        if (espeak_initialized) {
            espeak_Terminate();
        }
#endif
    }
    
    void loadConfig() {
        std::ifstream config_file(config_path);
        if (!config_file.good()) {
            LOGW("Config file not found: %s", config_path.c_str());
            return;
        }
        
        // 简单的 JSON 解析 phoneme_id_map
        // 完整实现应使用 JSON 库，这里使用简化方法
        std::stringstream buffer;
        buffer << config_file.rdbuf();
        std::string json_content = buffer.str();
        
        // 解析 noise_scale, length_scale, noise_w
        size_t ns_pos = json_content.find("\"noise_scale\":");
        if (ns_pos != std::string::npos) {
            noise_scale = std::stof(json_content.substr(ns_pos + 14, 10));
        }
        
        size_t ls_pos = json_content.find("\"length_scale\":");
        if (ls_pos != std::string::npos) {
            length_scale = std::stof(json_content.substr(ls_pos + 15, 10));
        }
        
        size_t nw_pos = json_content.find("\"noise_w\":");
        if (nw_pos != std::string::npos) {
            noise_w = std::stof(json_content.substr(nw_pos + 10, 10));
        }
        
        // 解析 phoneme_id_map (简化解析，实际应使用 JSON 库)
        // 格式: "phoneme": [id]
        size_t map_start = json_content.find("\"phoneme_id_map\":");
        if (map_start == std::string::npos) {
            LOGW("phoneme_id_map not found in config");
            return;
        }
        
        size_t map_end = json_content.find("},", map_start);
        if (map_end == std::string::npos) {
            map_end = json_content.find("}}", map_start);
        }
        
        std::string map_content = json_content.substr(map_start, map_end - map_start + 2);
        
        // 简单解析每个 "phoneme": [id] 对
        size_t pos = 0;
        while ((pos = map_content.find("\"", pos)) != std::string::npos) {
            size_t key_start = pos + 1;
            size_t key_end = map_content.find("\"", key_start);
            if (key_end == std::string::npos) break;
            
            std::string key = map_content.substr(key_start, key_end - key_start);
            
            size_t bracket_start = map_content.find("[", key_end);
            size_t bracket_end = map_content.find("]", bracket_start);
            if (bracket_start == std::string::npos || bracket_end == std::string::npos) break;
            
            std::string id_str = map_content.substr(bracket_start + 1, bracket_end - bracket_start - 1);
            
            // 去除空格
            id_str.erase(0, id_str.find_first_not_of(" \t\n"));
            id_str.erase(id_str.find_last_not_of(" \t\n") + 1);
            
            if (!id_str.empty() && !key.empty() && key != "phoneme_id_map") {
                try {
                    int id = std::stoi(id_str);
                    phoneme_to_id[key] = id;
                } catch (...) {}
            }
            
            pos = bracket_end + 1;
        }
        
        LOGI("Loaded %zu phoneme mappings", phoneme_to_id.size());
    }
    
#ifdef ESPEAK_NG_AVAILABLE
    void initEspeak() {
        // 推断 espeak-ng-data 路径
        // 模型路径通常在 /data/data/com.xxx/files/models/tts/xx/model.onnx
        // espeak-ng-data 应该在 /data/data/com.xxx/files/espeak-ng-data
        std::string espeak_data_path;
        
        size_t files_pos = model_path.find("/files/");
        if (files_pos != std::string::npos) {
            espeak_data_path = model_path.substr(0, files_pos) + "/files/espeak-ng-data";
        } else {
            // Fallback to compiled-in path
            espeak_data_path = "/data/data/com.allcallall/files/espeak-ng-data";
        }
        
        LOGI("espeak-ng data path: %s", espeak_data_path.c_str());
        
        // 初始化 espeak-ng (不播放音频，仅转换音素)
        int result = espeak_Initialize(AUDIO_OUTPUT_SYNCHRONOUS, 0, espeak_data_path.c_str(), 0);
        if (result == -1) {
            LOGE("Failed to initialize espeak-ng with path: %s", espeak_data_path.c_str());
            
            // 尝试再次不带路径初始化（使用编译时默认路径）
            result = espeak_Initialize(AUDIO_OUTPUT_SYNCHRONOUS, 0, nullptr, 0);
            if (result == -1) {
                LOGE("Failed to initialize espeak-ng without path");
                return;
            }
        }
        
        // 设置语音
        espeak_SetVoiceByName(espeak_voice.c_str());
        espeak_initialized = true;
        LOGI("espeak-ng initialized with voice: %s", espeak_voice.c_str());
    }
    
    std::string textToPhonemes(const std::string& text) {
        if (!espeak_initialized) {
            LOGW("espeak-ng not initialized");
            return "";
        }
        
        // 设置语音
        espeak_SetVoiceByName(espeak_voice.c_str());
        
        // 将文本转换为 IPA 音素
        const char* input = text.c_str();
        const void** input_ptr = (const void**)&input;
        
        // phonememode: bit 1 = IPA output, bits 8-23 = separator
        int phonememode = 0x02 | (0x20 << 8);  // IPA + space separator
        
        std::string phonemes;
        while (*input_ptr != nullptr) {
            const char* result = espeak_TextToPhonemes(input_ptr, espeakCHARS_UTF8, phonememode);
            if (result && *result) {
                if (!phonemes.empty()) phonemes += " ";
                phonemes += result;
            }
        }
        
        LOGI("Phonemes: %s", phonemes.c_str());
        return phonemes;
    }
#endif
    
    std::vector<int64_t> phonemesToIds(const std::string& phonemes) {
        std::vector<int64_t> ids;
        
        // 添加起始标记
        if (phoneme_to_id.count("^")) {
            ids.push_back(phoneme_to_id["^"]);
        }
        
        // 逐个转换音素
        std::istringstream iss(phonemes);
        std::string phoneme;
        while (iss >> phoneme) {
            // 逐字符处理 (IPA 字符可能是多字节 UTF-8)
            size_t i = 0;
            while (i < phoneme.length()) {
                // 处理 UTF-8 多字节字符
                size_t char_len = 1;
                unsigned char c = phoneme[i];
                if ((c & 0x80) == 0) char_len = 1;
                else if ((c & 0xE0) == 0xC0) char_len = 2;
                else if ((c & 0xF0) == 0xE0) char_len = 3;
                else if ((c & 0xF8) == 0xF0) char_len = 4;
                
                std::string ch = phoneme.substr(i, char_len);
                
                if (phoneme_to_id.count(ch)) {
                    ids.push_back(phoneme_to_id[ch]);
                }
                
                i += char_len;
            }
            
            // 添加空格
            if (phoneme_to_id.count(" ")) {
                ids.push_back(phoneme_to_id[" "]);
            }
        }
        
        // 添加结束标记
        if (phoneme_to_id.count("$")) {
            ids.push_back(phoneme_to_id["$"]);
        }
        
        return ids;
    }
    
#ifdef ONNX_RUNTIME_AVAILABLE
    void initOnnxRuntime() {
        ort_api = OrtGetApiBase()->GetApi(ORT_API_VERSION);
        if (!ort_api) {
            LOGE("Failed to get ONNX Runtime API");
            return;
        }
        
        OrtStatus* status = ort_api->CreateEnv(ORT_LOGGING_LEVEL_WARNING, "piper_tts", &env);
        if (status != nullptr) {
            LOGE("Failed to create ORT environment");
            ort_api->ReleaseStatus(status);
            return;
        }
        
        status = ort_api->CreateSessionOptions(&session_options);
        if (status != nullptr) {
            LOGE("Failed to create session options");
            ort_api->ReleaseStatus(status);
            return;
        }
        
        status = ort_api->CreateSession(env, model_path.c_str(), session_options, &session);
        if (status != nullptr) {
            LOGE("Failed to create ONNX session: %s", ort_api->GetErrorMessage(status));
            ort_api->ReleaseStatus(status);
            return;
        }
        
        is_ready = true;
        LOGI("ONNX Runtime session created for TTS");
    }
    
    std::vector<float> runInference(const std::vector<int64_t>& phoneme_ids) {
        if (!session || !ort_api) {
            return {};
        }
        
        OrtMemoryInfo* memory_info = nullptr;
        ort_api->CreateCpuMemoryInfo(OrtArenaAllocator, OrtMemTypeDefault, &memory_info);
        
        // 创建输入张量
        int64_t batch_size = 1;
        int64_t seq_len = static_cast<int64_t>(phoneme_ids.size());
        
        std::vector<int64_t> input_shape = {batch_size, seq_len};
        std::vector<int64_t> lengths_shape = {batch_size};
        std::vector<int64_t> scales_shape = {3};
        
        std::vector<int64_t> lengths = {seq_len};
        std::vector<float> scales = {noise_scale, length_scale, noise_w};
        
        OrtValue* input_tensor = nullptr;
        OrtValue* lengths_tensor = nullptr;
        OrtValue* scales_tensor = nullptr;
        
        // input tensor
        std::vector<int64_t> input_copy = phoneme_ids;
        ort_api->CreateTensorWithDataAsOrtValue(
            memory_info, input_copy.data(), input_copy.size() * sizeof(int64_t),
            input_shape.data(), input_shape.size(),
            ONNX_TENSOR_ELEMENT_DATA_TYPE_INT64, &input_tensor
        );
        
        // lengths tensor
        ort_api->CreateTensorWithDataAsOrtValue(
            memory_info, lengths.data(), lengths.size() * sizeof(int64_t),
            lengths_shape.data(), lengths_shape.size(),
            ONNX_TENSOR_ELEMENT_DATA_TYPE_INT64, &lengths_tensor
        );
        
        // scales tensor
        ort_api->CreateTensorWithDataAsOrtValue(
            memory_info, scales.data(), scales.size() * sizeof(float),
            scales_shape.data(), scales_shape.size(),
            ONNX_TENSOR_ELEMENT_DATA_TYPE_FLOAT, &scales_tensor
        );
        
        const char* input_names[] = {"input", "input_lengths", "scales"};
        const char* output_names[] = {"output"};
        OrtValue* input_tensors[] = {input_tensor, lengths_tensor, scales_tensor};
        OrtValue* output_tensor = nullptr;
        
        OrtStatus* status = ort_api->Run(
            session, nullptr,
            input_names, input_tensors, 3,
            output_names, 1, &output_tensor
        );
        
        std::vector<float> audio_data;
        
        if (status == nullptr && output_tensor != nullptr) {
            float* output_data = nullptr;
            ort_api->GetTensorMutableData(output_tensor, (void**)&output_data);
            
            OrtTensorTypeAndShapeInfo* shape_info = nullptr;
            ort_api->GetTensorTypeAndShape(output_tensor, &shape_info);
            
            size_t element_count = 0;
            ort_api->GetTensorShapeElementCount(shape_info, &element_count);
            
            audio_data.assign(output_data, output_data + element_count);
            
            ort_api->ReleaseTensorTypeAndShapeInfo(shape_info);
            ort_api->ReleaseValue(output_tensor);
        } else if (status != nullptr) {
            LOGE("TTS inference failed: %s", ort_api->GetErrorMessage(status));
            ort_api->ReleaseStatus(status);
        }
        
        ort_api->ReleaseValue(input_tensor);
        ort_api->ReleaseValue(lengths_tensor);
        ort_api->ReleaseValue(scales_tensor);
        ort_api->ReleaseMemoryInfo(memory_info);
        
        return audio_data;
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
    
    std::vector<int16_t> synthesize(const std::string& text) {
        if (!is_ready) {
            LOGW("TTS model not ready, using placeholder");
            return generatePlaceholderAudio(text);
        }
        
        LOGI("Synthesizing speech for: %s (language: %s)", text.c_str(), language.c_str());
        
#if defined(ESPEAK_NG_AVAILABLE) && defined(ONNX_RUNTIME_AVAILABLE)
        // 1. 文本转音素
        std::string phonemes = textToPhonemes(text);
        if (phonemes.empty()) {
            LOGW("Phoneme conversion failed");
            return generatePlaceholderAudio(text);
        }
        
        // 2. 音素转 ID
        std::vector<int64_t> phoneme_ids = phonemesToIds(phonemes);
        if (phoneme_ids.empty()) {
            LOGW("Phoneme to ID conversion failed");
            return generatePlaceholderAudio(text);
        }
        
        LOGI("Phoneme IDs count: %zu", phoneme_ids.size());
        
        // 3. ONNX 推理
        std::vector<float> audio_float = runInference(phoneme_ids);
        if (audio_float.empty()) {
            LOGW("ONNX inference failed");
            return generatePlaceholderAudio(text);
        }
        
        // 4. 转换为 int16
        std::vector<int16_t> audio(audio_float.size());
        for (size_t i = 0; i < audio_float.size(); i++) {
            float sample = audio_float[i];
            sample = std::max(-1.0f, std::min(1.0f, sample));
            audio[i] = static_cast<int16_t>(sample * 32767.0f);
        }
        
        LOGI("Generated %zu audio samples", audio.size());
        return audio;
#else
        // Fallback to placeholder
        return generatePlaceholderAudio(text);
#endif
    }
    
private:
    std::vector<int16_t> generatePlaceholderAudio(const std::string& text) {
        float base_duration = 0.15f;
        float total_duration = std::min(5.0f, base_duration * text.length());
        
        int num_samples = static_cast<int>(sample_rate * total_duration);
        std::vector<int16_t> audio(num_samples);
        
        std::vector<float> frequencies = {300, 350, 280, 400, 320};
        int samples_per_segment = num_samples / frequencies.size();
        
        for (size_t seg = 0; seg < frequencies.size() && seg * samples_per_segment < num_samples; seg++) {
            float freq = frequencies[seg];
            int start = seg * samples_per_segment;
            int end = std::min(start + samples_per_segment, num_samples);
            
            for (int i = start; i < end; i++) {
                float t = static_cast<float>(i - start) / sample_rate;
                float sample = std::sin(2.0f * 3.14159f * freq * t);
                sample += 0.3f * std::sin(4.0f * 3.14159f * freq * t);
                sample *= 0.5f;
                
                float envelope = 1.0f;
                int seg_samples = end - start;
                int local_i = i - start;
                if (local_i < seg_samples / 10) {
                    envelope = static_cast<float>(local_i) / (seg_samples / 10);
                } else if (local_i > seg_samples * 9 / 10) {
                    envelope = static_cast<float>(seg_samples - local_i) / (seg_samples / 10);
                }
                
                audio[i] = static_cast<int16_t>(sample * envelope * 20000);
            }
        }
        
        LOGI("Generated %d placeholder audio samples", num_samples);
        return audio;
    }
};

PiperTTS::PiperTTS(const std::string& model_path) 
    : pImpl(std::make_unique<Impl>(model_path)) {
}

PiperTTS::~PiperTTS() = default;

std::vector<int16_t> PiperTTS::synthesize(const std::string& text) {
    return pImpl->synthesize(text);
}

int PiperTTS::getSampleRate() const {
    return pImpl->sample_rate;
}

bool PiperTTS::isReady() const {
    return pImpl->is_ready;
}
