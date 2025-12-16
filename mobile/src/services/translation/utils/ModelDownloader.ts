// mobile/src/services/translation/utils/ModelDownloader.ts
import RNFS from 'react-native-fs';

export interface DownloadProgress {
  bytesWritten: number;
  contentLength: number;
  progress: number;
}

export type ProgressCallback = (progress: DownloadProgress) => void;

class ModelDownloader {
  private modelUrls: { [key: string]: string } = {
    whisper: 'https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-small-q8_0.bin',
    opus: 'https://huggingface.co/Helsinki-NLP/opus-mt-en-zh/resolve/main/model.onnx',
    tts: 'https://huggingface.co/coqui/VITS/resolve/main/model.bin'
  };

  async download(
    modelName: string,
    onProgress?: ProgressCallback
  ): Promise<string> {
    const url = this.modelUrls[modelName];
    if (!url) {
      throw new Error(`Unknown model: ${modelName}`);
    }

    const modelDir = `${RNFS.DocumentDirectoryPath}/models/${modelName}`;
    const fileName = this.getFileName(modelName);
    const filePath = `${modelDir}/${fileName}`;

    console.log(`[ModelDownloader] Starting download: ${modelName}`);
    console.log(`[ModelDownloader] URL: ${url}`);
    console.log(`[ModelDownloader] Destination: ${filePath}`);

    // 创建目录
    const dirExists = await RNFS.exists(modelDir);
    if (!dirExists) {
      await RNFS.mkdir(modelDir);
    }

    try {
      // 下载文件
      const downloadResult = await RNFS.downloadFile({
        fromUrl: url,
        toFile: filePath,
        progress: (res) => {
          if (onProgress) {
            onProgress({
              bytesWritten: res.bytesWritten,
              contentLength: res.contentLength,
              progress: res.bytesWritten / res.contentLength
            });
          }
        }
      }).promise;

      if (downloadResult.statusCode === 200) {
        console.log(`[ModelDownloader] Download complete: ${modelName}`);
        return filePath;
      } else {
        throw new Error(`Download failed with status: ${downloadResult.statusCode}`);
      }
    } catch (error) {
      console.error(`[ModelDownloader] Download error:`, error);
      // 清理失败的下载
      const exists = await RNFS.exists(filePath);
      if (exists) {
        await RNFS.unlink(filePath);
      }
      throw error;
    }
  }

  private getFileName(modelName: string): string {
    const fileNames: { [key: string]: string } = {
      whisper: 'ggml-small-q8.bin',
      opus: 'opus-mt-en-zh-q8.onnx',
      tts: 'vits-zh-en.bin'
    };
    return fileNames[modelName] || 'model.bin';
  }

  async checkModelExists(modelName: string): Promise<boolean> {
    const modelDir = `${RNFS.DocumentDirectoryPath}/models/${modelName}`;
    const fileName = this.getFileName(modelName);
    const filePath = `${modelDir}/${fileName}`;
    return await RNFS.exists(filePath);
  }

  async getModelSize(modelName: string): Promise<number> {
    const modelDir = `${RNFS.DocumentDirectoryPath}/models/${modelName}`;
    const fileName = this.getFileName(modelName);
    const filePath = `${modelDir}/${fileName}`;
    
    const exists = await RNFS.exists(filePath);
    if (!exists) {
      return 0;
    }

    const stat = await RNFS.stat(filePath);
    return parseInt(stat.size, 10);
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
