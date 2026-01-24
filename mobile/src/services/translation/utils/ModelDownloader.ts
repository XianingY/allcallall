// mobile/src/services/translation/utils/ModelDownloader.ts
import RNFS from 'react-native-fs';

export interface DownloadProgress {
  bytesWritten: number;
  contentLength: number;
  progress: number;
}

export type ProgressCallback = (progress: DownloadProgress) => void;

class ModelDownloader {
  // Note: In this repo the preferred path is to bundle models into APK assets and
  // copy them on first run via `TranslationService.initialize()`.
  // This helper is primarily used for inspecting whether models are installed.

  async download(
    modelName: string,
    onProgress?: ProgressCallback
  ): Promise<string> {
    void onProgress;
    throw new Error(
      `Direct download is not supported in-app for '${modelName}'. ` +
        'Install bundled models by running TranslationService.initialize() (copies from APK assets).'
    );
  }

  private getExpectedPaths(modelName: string): string[] {
    const base = `${RNFS.DocumentDirectoryPath}/models`;

    switch (modelName) {
      case 'whisper':
        return [`${base}/whisper/ggml-small-q8.bin`];

      case 'opus':
        return [
          `${base}/opus/en-zh/opus-mt-en-zh-q8.onnx`,
          `${base}/opus/en-zh/source.spm`,
          `${base}/opus/en-zh/target.spm`,
          `${base}/opus/zh-en/opus-mt-zh-en-q8.onnx`,
          `${base}/opus/zh-en/source.spm`,
          `${base}/opus/zh-en/target.spm`,
        ];

      case 'tts':
        return [
          `${base}/tts/zh/zh_CN-huayan-medium.onnx`,
          `${base}/tts/zh/zh_CN-huayan-medium.onnx.json`,
          `${base}/tts/en/en_US-amy-medium.onnx`,
          `${base}/tts/en/en_US-amy-medium.onnx.json`,
        ];

      default:
        throw new Error(`Unknown model: ${modelName}`);
    }
  }

  async checkModelExists(modelName: string): Promise<boolean> {
    const paths = this.getExpectedPaths(modelName);
    for (const path of paths) {
      const exists = await RNFS.exists(path);
      if (!exists) return false;
    }
    return true;
  }

  async getModelSize(modelName: string): Promise<number> {
    // Size is meaningful for single-file models only.
    if (modelName !== 'whisper') return 0;

    const [filePath] = this.getExpectedPaths(modelName);
    const exists = await RNFS.exists(filePath);
    if (!exists) return 0;

    const stat = await RNFS.stat(filePath);
    return typeof stat.size === 'string' ? parseInt(stat.size, 10) : stat.size;
  }

  async deleteModel(modelName: string): Promise<void> {
    const modelDir = `${RNFS.DocumentDirectoryPath}/models/${modelName}`;
    const exists = await RNFS.exists(modelDir);
    if (exists) {
      await RNFS.unlink(modelDir);
      console.log(`[ModelDownloader] Deleted model: ${modelName}`);
    }
  }

  async deleteAllModels(): Promise<void> {
    const modelsDir = `${RNFS.DocumentDirectoryPath}/models`;
    const exists = await RNFS.exists(modelsDir);
    if (exists) {
      await RNFS.unlink(modelsDir);
      console.log('[ModelDownloader] Deleted all models');
    }
  }
}

export default ModelDownloader;
