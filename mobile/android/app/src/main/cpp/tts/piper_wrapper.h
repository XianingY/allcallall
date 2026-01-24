// mobile/android/app/src/main/cpp/tts/piper_wrapper.h
#ifndef PIPER_WRAPPER_H
#define PIPER_WRAPPER_H

#include <string>
#include <vector>
#include <memory>

/**
 * Piper TTS wrapper for text-to-speech synthesis
 * Uses ONNX Runtime to load Piper VITS models
 */
class PiperTTS {
public:
    /**
     * Load a Piper TTS model from file
     * @param model_path Path to the .onnx TTS model file
     */
    explicit PiperTTS(const std::string& model_path);
    
    /**
     * Destructor - cleanup resources
     */
    ~PiperTTS();
    
    /**
     * Synthesize speech from text
     * @param text Input text to synthesize
     * @return Audio samples as 16-bit PCM at model's sample rate
     */
    std::vector<int16_t> synthesize(const std::string& text);
    
    /**
     * Get the sample rate of the TTS model
     * @return Sample rate in Hz (typically 22050)
     */
    int getSampleRate() const;
    
    /**
     * Check if model is loaded and ready
     * @return true if model is ready
     */
    bool isReady() const;

private:
    class Impl;
    std::unique_ptr<Impl> pImpl;
};

#endif // PIPER_WRAPPER_H
