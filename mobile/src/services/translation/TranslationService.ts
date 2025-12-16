// mobile/src/services/translation/TranslationService.ts
import { NativeModules } from 'react-native';
import RNFS from 'react-native-fs';
import ModelDownloader from './utils/ModelDownloader';

const { TranslationModule } = NativeModules;

export interface TranslationConfig {
  whisperModel?: 'tiny' | 'base' | 'small';
  opusModel?: string;
  ttsModel?: string;
  targetLanguage: string;
  quantization?: 'fp32' | 'int8' | 'int4';
}

export interface TranslationResult {
  originalText: string;
  translatedText: string;
  confidence: number;
  processingTime: number;
  audioUrl?: string;
}

class TranslationService {
  private isInitialized = false;
  private config: TranslationConfig | null = null;

  async initialize(config: TranslationConfig): Promise<void> {
    if (this.isInitialized) {
      console.log('[TranslationService] Already initialized');
      return;
    }
    
    console.log('[TranslationService] Initializing with config:', config);
    this.config = config;
    
    try {
      // 检查并下载模型
      await this.checkAndDownloadModels(config);
      
      // 初始化 Native Module
      if (TranslationModule) {
        await TranslationModule.initialize(
          await this.getModelPath('whisper'),
          await this.getModelPath('opus'),
          await this.getModelPath('tts'),
          config.quantization || 'int8'
        );
      }
      
      this.isInitialized = true;
      console.log('[TranslationService] Initialization complete');
    } catch (error) {
      console.error('[TranslationService] Initialization failed:', error);
      throw error;
    }
  }

  private async checkAndDownloadModels(config: TranslationConfig): Promise<void> {
    const models = [
      { name: 'whisper', path: await this.getModelPath('whisper') },
      { name: 'opus', path: await this.getModelPath('opus') },
      { name: 'tts', path: await this.getModelPath('tts') }
    ];

    for (const model of models) {
      const exists = await RNFS.exists(model.path);
      if (!exists) {
        console.log(`[TranslationService] Model ${model.name} not found, downloading...`);
        await this.downloadModel(model.name);
      } else {
        console.log(`[TranslationService] Model ${model.name} already exists`);
      }
    }
  }

  private async downloadModel(modelName: string): Promise<void> {
    const downloader = new ModelDownloader();
    await downloader.download(modelName);
  }

  async translateAudio(
    audioData: Float32Array,
    targetLanguage: string
  ): Promise<TranslationResult> {
    if (!this.isInitialized) {
      throw new Error('Translation service not initialized');
    }

    const startTime = Date.now();

    return new Promise((resolve, reject) => {
      if (!TranslationModule) {
        reject(new Error('TranslationModule not available'));
        return;
      }

      TranslationModule.translateAudio(
        this.floatArrayToBase64(audioData),
        targetLanguage,
        (result: any) => {
          resolve({
            originalText: result.originalText,
            translatedText: result.translatedText,
            confidence: result.confidence,
            processingTime: Date.now() - startTime,
            audioUrl: result.audioUrl
          });
        },
        (error: string) => {
          reject(new Error(error));
        }
      );
    });
  }

  private floatArrayToBase64(array: Float32Array): string {
    // 将 Float32Array 转换为 Base64 字符串
    const buffer = Buffer.from(array.buffer);
    return buffer.toString('base64');
  }

  private async getModelPath(modelName: string): Promise<string> {
    // 模型文件路径
    const modelFiles: { [key: string]: string } = {
      whisper: 'ggml-small-q8.bin',
      opus: 'opus-mt-en-zh-q8.onnx',
      tts: 'vits-zh-en.bin'
    };

    const fileName = modelFiles[modelName];
    if (!fileName) {
      throw new Error(`Unknown model: ${modelName}`);
    }

    return `${RNFS.DocumentDirectoryPath}/models/${modelName}/${fileName}`;
  }

  async cleanup(): Promise<void> {
    if (TranslationModule && this.isInitialized) {
      await TranslationModule.cleanup();
      this.isInitialized = false;
      this.config = null;
      console.log('[TranslationService] Cleanup complete');
    }
  }

  isReady(): boolean {
    return this.isInitialized;
  }

  getConfig(): TranslationConfig | null {
    return this.config;
  }
}

export default new TranslationService();
