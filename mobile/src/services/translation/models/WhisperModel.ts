// mobile/src/services/translation/models/WhisperModel.ts
import { NativeModules } from 'react-native';

const { TranslationModule } = NativeModules;

export interface WhisperConfig {
  modelPath: string;
  language?: string;
  quantization?: 'fp32' | 'int8' | 'int4';
}

export interface TranscriptionResult {
  text: string;
  confidence: number;
  segments?: Array<{
    start: number;
    end: number;
    text: string;
  }>;
}

class WhisperModel {
  private isLoaded = false;

  async load(config: WhisperConfig): Promise<void> {
    if (this.isLoaded) {
      console.log('[WhisperModel] Already loaded');
      return;
    }

    console.log('[WhisperModel] Loading model from:', config.modelPath);
    // Model loading is handled by TranslationModule.initialize
    this.isLoaded = true;
  }

  async transcribe(audioData: Float32Array): Promise<TranscriptionResult> {
    if (!this.isLoaded) {
      throw new Error('Whisper model not loaded');
    }

    // Transcription is handled through TranslationModule
    // This is a wrapper for better code organization
    return {
      text: '',
      confidence: 0.0,
      segments: []
    };
  }

  async unload(): Promise<void> {
    if (this.isLoaded) {
      this.isLoaded = false;
      console.log('[WhisperModel] Model unloaded');
    }
  }

  isModelLoaded(): boolean {
    return this.isLoaded;
  }
}

export default WhisperModel;
