// mobile/src/services/translation/TranslationService.ts
import { NativeModules } from 'react-native';
import RNFS from 'react-native-fs';

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
      // 复制 espeak-ng 数据（用于 TTS）
      await this.copyEspeakData();

      // 检查本地模型（不进行网络下载）
      await this.checkModelsExist(config);

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

  /**
   * 复制 espeak-ng 数据从 assets 到 files 目录
   * espeak-ng 需要这些数据文件进行 text-to-phoneme 转换
   */
  private async copyEspeakData(): Promise<void> {
    const espeakDataDir = `${RNFS.DocumentDirectoryPath}/espeak-ng-data`;

    // 检查是否已经复制过
    const exists = await RNFS.exists(espeakDataDir);
    if (exists) {
      console.log('[TranslationService] espeak-ng data already exists');
      return;
    }

    console.log('[TranslationService] Copying espeak-ng data from assets...');

    try {
      // 创建目录
      await RNFS.mkdir(espeakDataDir);
      await RNFS.mkdir(`${espeakDataDir}/lang`);
      await RNFS.mkdir(`${espeakDataDir}/voices`);

      // 需要复制的文件列表
      const files = [
        'en_dict',
        'cmn_dict',
        'intonations',
        'phondata',
        'phonindex',
        'phontab',
      ];

      // 复制主要文件
      for (const file of files) {
        await RNFS.copyFileAssets(
          `espeak-ng-data/${file}`,
          `${espeakDataDir}/${file}`
        );
      }

      // 复制 lang 和 voices 子目录内容
      // 注意: RNFS.copyFileAssets 不能递归复制目录
      // 需要使用 readDirAssets 列出文件
      const langFiles = await RNFS.readDirAssets('espeak-ng-data/lang');
      for (const item of langFiles) {
        if (item.isFile()) {
          await RNFS.copyFileAssets(
            `espeak-ng-data/lang/${item.name}`,
            `${espeakDataDir}/lang/${item.name}`
          );
        }
      }

      const voicesFiles = await RNFS.readDirAssets('espeak-ng-data/voices');
      for (const item of voicesFiles) {
        if (item.isFile()) {
          await RNFS.copyFileAssets(
            `espeak-ng-data/voices/${item.name}`,
            `${espeakDataDir}/voices/${item.name}`
          );
        } else if (item.isDirectory()) {
          // 创建子目录并复制
          await RNFS.mkdir(`${espeakDataDir}/voices/${item.name}`);
          const subFiles = await RNFS.readDirAssets(`espeak-ng-data/voices/${item.name}`);
          for (const subItem of subFiles) {
            if (subItem.isFile()) {
              await RNFS.copyFileAssets(
                `espeak-ng-data/voices/${item.name}/${subItem.name}`,
                `${espeakDataDir}/voices/${item.name}/${subItem.name}`
              );
            }
          }
        }
      }

      console.log('[TranslationService] espeak-ng data copied successfully');
    } catch (error) {
      console.error('[TranslationService] Failed to copy espeak-ng data:', error);
      // 不抛出错误，TTS 将使用占位符音频
    }
  }

  /**
   * 检查模型是否存在，不进行网络下载
   * 模型应该通过 adb push 或其他方式预先放置到设备上
   */
  private async checkModelsExist(config: TranslationConfig): Promise<void> {
    console.log('[TranslationService] Checking local models...');

    const modelChecks = [
      { name: 'whisper', type: 'STT' },
      { name: 'opus', type: 'Translation' },
      { name: 'tts', type: 'TTS' },
    ];

    const missingModels: string[] = [];

    for (const model of modelChecks) {
      const path = await this.getModelPath(model.name);
      const exists = await RNFS.exists(path);

      if (exists) {
        console.log(`[TranslationService] ✓ ${model.type} model found: ${path}`);
      } else {
        console.warn(`[TranslationService] ✗ ${model.type} model NOT found: ${path}`);
        missingModels.push(`${model.type} (${model.name})`);
      }
    }

    if (missingModels.length > 0) {
      console.warn('[TranslationService] Missing models:', missingModels.join(', '));
      console.warn('[TranslationService] 请将模型文件复制到设备上的正确路径');
      console.warn('[TranslationService] 翻译功能可能无法正常工作');
      // 不抛出错误，允许应用继续运行（使用占位符功能）
    }
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
    // Updated model files for bidirectional translation
    const modelFiles: { [key: string]: any } = {
      whisper: 'ggml-small-q8.bin',
      opus: {
        'en-zh': 'opus-mt-en-zh-q8.onnx',
        'zh-en': 'opus-mt-zh-en-q8.onnx'
      },
      tts: {
        'zh': 'zh_CN-huayan-medium.onnx',
        'en': 'en_US-amy-medium.onnx'
      }
    };

    const modelFile = modelFiles[modelName];
    if (!modelFile) {
      throw new Error(`Unknown model: ${modelName}`);
    }

    // For whisper, return full path; for others, return directory path
    if (typeof modelFile === 'string') {
      return `${RNFS.DocumentDirectoryPath}/models/${modelName}/${modelFile}`;
    } else {
      // Return directory path for opus and tts (contains multiple model files)
      return `${RNFS.DocumentDirectoryPath}/models/${modelName}`;
    }
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
