// mobile/src/services/translation/TranslationService.ts
import { NativeEventEmitter, NativeModules, EmitterSubscription } from 'react-native';
import RNFS from 'react-native-fs';

const { TranslationModule } = NativeModules;
const { WebRTCModule } = NativeModules;

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
  audioBase64?: string;
}

class TranslationService {
  private isInitialized = false;
  private config: TranslationConfig | null = null;

  private subtitleEmitter: NativeEventEmitter | null = null;
  private subtitleSub: EmitterSubscription | null = null;

  private webrtcEmitter: NativeEventEmitter | null = null;
  private webrtcSub: EmitterSubscription | null = null;
  private webrtcTranslating = false;

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

      // 从 assets 复制模型到 files 目录（首次启动）
      await this.copyModelsFromAssets();

      // Fail loud if required model files are missing.
      await this.verifyInstalledModels();

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

  private async verifyInstalledModels(): Promise<void> {
    const base = `${RNFS.DocumentDirectoryPath}/models`;
    const required = [
      // Whisper
      `${base}/whisper/ggml-small-q8.bin`,
      // Opus-MT (bidirectional)
      `${base}/opus/en-zh/opus-mt-en-zh-q8.onnx`,
      `${base}/opus/en-zh/source.spm`,
      `${base}/opus/en-zh/target.spm`,
      `${base}/opus/zh-en/opus-mt-zh-en-q8.onnx`,
      `${base}/opus/zh-en/source.spm`,
      `${base}/opus/zh-en/target.spm`,
      // Piper TTS
      `${base}/tts/zh/zh_CN-huayan-medium.onnx`,
      `${base}/tts/zh/zh_CN-huayan-medium.onnx.json`,
      `${base}/tts/en/en_US-amy-medium.onnx`,
      `${base}/tts/en/en_US-amy-medium.onnx.json`,
    ];

    const missing: string[] = [];
    for (const path of required) {
      const exists = await RNFS.exists(path);
      if (!exists) missing.push(path);
    }

    if (missing.length > 0) {
      throw new Error(
        `Missing offline translation model files (copied from APK assets failed or assets not packaged):\n` +
          missing.join('\n')
      );
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
   * 从 APK assets 复制模型到 files 目录（首次启动时）
   */
  private async copyModelsFromAssets(): Promise<void> {
    const modelsDir = `${RNFS.DocumentDirectoryPath}/models`;

    // 检查是否已经复制过
    const whisperPath = `${modelsDir}/whisper/ggml-small-q8.bin`;
    const exists = await RNFS.exists(whisperPath);
    if (exists) {
      console.log('[TranslationService] Models already copied from assets');
      return;
    }

    console.log('[TranslationService] Copying models from assets (first run)...');
    console.log('[TranslationService] This may take a few minutes...');

    try {
      // 创建目录结构
      await RNFS.mkdir(modelsDir);
      await RNFS.mkdir(`${modelsDir}/whisper`);
      await RNFS.mkdir(`${modelsDir}/opus`);
      await RNFS.mkdir(`${modelsDir}/tts`);
      await RNFS.mkdir(`${modelsDir}/tts/en`);
      await RNFS.mkdir(`${modelsDir}/tts/zh`);

      // 复制 Whisper 模型
      console.log('[TranslationService] Copying Whisper model...');
      await RNFS.copyFileAssets(
        'models/whisper/ggml-small-q8.bin',
        `${modelsDir}/whisper/ggml-small-q8.bin`
      );

      // 复制 ONNX 翻译模型 (en-zh)
      console.log('[TranslationService] Copying translation models (en-zh)...');
      await this.copyAssetDir('models/opus/en-zh', `${modelsDir}/opus/en-zh`);

      // 复制 ONNX 翻译模型 (zh-en)
      console.log('[TranslationService] Copying translation models (zh-en)...');
      await this.copyAssetDir('models/opus/zh-en', `${modelsDir}/opus/zh-en`);

      // 复制 TTS 模型
      console.log('[TranslationService] Copying TTS models...');
      await this.copyAssetDir('models/tts/en', `${modelsDir}/tts/en`);
      await this.copyAssetDir('models/tts/zh', `${modelsDir}/tts/zh`);

      console.log('[TranslationService] ✓ All models copied successfully!');
    } catch (error) {
      console.error('[TranslationService] Error copying models:', error);
      throw new Error('Failed to copy models from assets. Please reinstall the app.');
    }
  }

  /**
   * 复制 asset 目录下的所有文件
   */
  private async copyAssetDir(assetPath: string, destPath: string): Promise<void> {
    await RNFS.mkdir(destPath);
    const files = await RNFS.readDirAssets(assetPath);

    for (const file of files) {
      if (file.isFile()) {
        await RNFS.copyFileAssets(
          `${assetPath}/${file.name}`,
          `${destPath}/${file.name}`
        );
      }
    }
  }

  async translateAudio(
    audioData: Float32Array,
    targetLanguage: string
  ): Promise<TranslationResult> {
    if (!this.isInitialized) {
      throw new Error('Translation service not initialized');
    }

    if (!TranslationModule) {
      throw new Error('TranslationModule not available');
    }

    const startTime = Date.now();
    const result = await TranslationModule.translateAudio(
      this.float32ArrayToBase64(audioData),
      targetLanguage
    );

    return {
      originalText: result?.originalText ?? '',
      translatedText: result?.translatedText ?? '',
      confidence: typeof result?.confidence === 'number' ? result.confidence : 0,
      processingTime: Date.now() - startTime,
      audioBase64: result?.audioBase64 ?? '',
    };
  }

  // Phase-2 validation path (Android): record mic PCM natively and translate.
  async recordAndTranslate(
    durationMs: number,
    targetLanguage: string
  ): Promise<TranslationResult> {
    if (!this.isInitialized) {
      throw new Error('Translation service not initialized');
    }
    if (!TranslationModule || typeof TranslationModule.recordAndTranslate !== 'function') {
      throw new Error('TranslationModule.recordAndTranslate not available');
    }

    const startTime = Date.now();
    const result = await TranslationModule.recordAndTranslate(durationMs, targetLanguage);
    return {
      originalText: result?.originalText ?? '',
      translatedText: result?.translatedText ?? '',
      confidence: typeof result?.confidence === 'number' ? result.confidence : 0,
      processingTime: Date.now() - startTime,
      audioBase64: result?.audioBase64 ?? '',
    };
  }

  async startWebRTCCallMicTranslation(
    targetLanguage: string,
    onSubtitle: (result: TranslationResult & { timestampMs: number }) => void,
    chunkMs: number = 2000
  ): Promise<void> {
    if (!this.isInitialized) {
      throw new Error('Translation service not initialized');
    }
    if (!WebRTCModule || typeof WebRTCModule.startLocalAudioCapture !== 'function') {
      throw new Error('WebRTCModule.startLocalAudioCapture not available');
    }
    if (!WebRTCModule || typeof WebRTCModule.stopLocalAudioCapture !== 'function') {
      throw new Error('WebRTCModule.stopLocalAudioCapture not available');
    }

    this.stopWebRTCCallMicTranslationSync();

    this.webrtcEmitter = new NativeEventEmitter(WebRTCModule);
    this.webrtcSub = this.webrtcEmitter.addListener('webrtcAudioChunk', async (payload: any) => {
      if (this.webrtcTranslating) return;
      this.webrtcTranslating = true;

      try {
        const pcmBase64 = String(payload?.pcm16Base64 ?? '');
        const sampleRate = typeof payload?.sampleRate === 'number' ? payload.sampleRate : 48000;
        const channels = typeof payload?.channels === 'number' ? payload.channels : 1;
        const ts = typeof payload?.timestampMs === 'number' ? payload.timestampMs : Date.now();

        const audio = this.pcm16Base64ToFloat32Mono16k(pcmBase64, sampleRate, channels);
        const result = await this.translateAudio(audio, targetLanguage);
        onSubtitle({ ...result, timestampMs: ts });
      } catch (e) {
        console.warn('[TranslationService] webrtcAudioChunk translate failed:', e);
      } finally {
        this.webrtcTranslating = false;
      }
    });

    WebRTCModule.startLocalAudioCapture(chunkMs);
  }

  async stopWebRTCCallMicTranslation(): Promise<void> {
    this.stopWebRTCCallMicTranslationSync();
    if (WebRTCModule && typeof WebRTCModule.stopLocalAudioCapture === 'function') {
      WebRTCModule.stopLocalAudioCapture();
    }
  }

  private stopWebRTCCallMicTranslationSync(): void {
    if (this.webrtcSub) {
      this.webrtcSub.remove();
      this.webrtcSub = null;
    }
    this.webrtcEmitter = null;
    this.webrtcTranslating = false;
  }

  private stopSubtitlesSync(): void {
    if (this.subtitleSub) {
      this.subtitleSub.remove();
      this.subtitleSub = null;
    }
    this.subtitleEmitter = null;
  }

  private base64ToBytes(base64: string): Uint8Array {
    const atobFn = (globalThis as any).atob as ((data: string) => string) | undefined;
    if (typeof atobFn === 'function') {
      const binary = atobFn(base64);
      const bytes = new Uint8Array(binary.length);
      for (let i = 0; i < binary.length; i++) {
        bytes[i] = binary.charCodeAt(i);
      }
      return bytes;
    }

    const BufferCtor = (globalThis as any).Buffer as any;
    if (BufferCtor && typeof BufferCtor.from === 'function') {
      return new Uint8Array(BufferCtor.from(base64, 'base64'));
    }

    throw new Error('No base64 decoder available in this runtime');
  }

  private pcm16Base64ToFloat32Mono16k(base64: string, sampleRate: number, channels: number): Float32Array {
    const bytes = this.base64ToBytes(base64);
    if (bytes.length < 2) return new Float32Array(0);

    const view = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength);
    const totalSamples = Math.floor(bytes.length / 2);
    const frames = Math.floor(totalSamples / Math.max(1, channels));

    const mono = new Float32Array(frames);
    for (let i = 0; i < frames; i++) {
      const sampleIndex = i * channels;
      const int16 = view.getInt16(sampleIndex * 2, true);
      mono[i] = int16 / 32768.0;
    }

    if (sampleRate === 16000) {
      return mono;
    }

    if (sampleRate === 48000) {
      const outLen = Math.floor(mono.length / 3);
      const out = new Float32Array(outLen);
      let oi = 0;
      for (let i = 0; i + 2 < mono.length; i += 3) {
        out[oi++] = mono[i];
      }
      return out;
    }

    // Unsupported sample rate: return empty to avoid feeding wrong data.
    return new Float32Array(0);
  }

  private float32ArrayToBase64(array: Float32Array): string {
    // Encode raw float32 bytes as base64; native side decodes bytes -> float32.
    const bytes = new Uint8Array(array.buffer);

    const btoaFn = (globalThis as any).btoa as ((data: string) => string) | undefined;
    if (typeof btoaFn === 'function') {
      let binary = '';
      const chunkSize = 0x8000;
      for (let i = 0; i < bytes.length; i += chunkSize) {
        const chunk = bytes.subarray(i, i + chunkSize);
        binary += String.fromCharCode(...chunk);
      }
      return btoaFn(binary);
    }

    const BufferCtor = (globalThis as any).Buffer as any;
    if (BufferCtor && typeof BufferCtor.from === 'function') {
      return BufferCtor.from(bytes).toString('base64');
    }

    throw new Error('No base64 encoder available in this runtime');
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
