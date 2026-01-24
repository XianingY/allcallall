// mobile/android/app/src/main/cpp/onnx/onnx_wrapper.h
#ifndef ONNX_WRAPPER_H
#define ONNX_WRAPPER_H

#include <string>
#include <vector>
#include <memory>

/**
 * ONNX Model wrapper for translation models (Opus-MT)
 * Uses ONNX Runtime to load and run .onnx models
 */
class OnnxModel {
public:
    /**
     * Load an ONNX model from file
     * @param model_path Path to the .onnx model file
     */
    explicit OnnxModel(const std::string& model_path);
    
    /**
     * Destructor - cleanup resources
     */
    ~OnnxModel();
    
    /**
     * Translate text using the loaded model
     * @param text Input text to translate
     * @return Translated text
     */
    std::string translate(const std::string& text);
    
    /**
     * Synthesize speech from text (for TTS models)
     * @param text Input text to synthesize
     * @return Audio samples as 16-bit PCM
     */
    std::vector<int16_t> synthesize(const std::string& text);
    
    /**
     * Check if model is loaded and ready
     * @return true if model is ready
     */
    bool isReady() const;

private:
    class Impl;
    std::unique_ptr<Impl> pImpl;
};

#endif // ONNX_WRAPPER_H
