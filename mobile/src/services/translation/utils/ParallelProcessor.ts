// mobile/src/services/translation/utils/ParallelProcessor.ts
import { MediaStream } from 'react-native-webrtc';
import TranslationService from '../TranslationService';
import AudioProcessor, { AudioChunk } from './AudioProcessor';
import PerformanceMonitor from './PerformanceMonitor';
import { SubtitleItem } from '../../../components/translation/TranslationOverlay';

class ParallelProcessor {
  private isProcessing = false;
  private currentLanguage = 'zh';

  async processAudioStream(
    audioStream: MediaStream,
    onSubtitle: (subtitle: SubtitleItem) => void,
    targetLanguage: string = 'zh'
  ): Promise<void> {
    if (this.isProcessing) {
      console.warn('[ParallelProcessor] Already processing');
      return;
    }

    this.isProcessing = true;
    this.currentLanguage = targetLanguage;

    console.log('[ParallelProcessor] Starting parallel audio processing');

    await AudioProcessor.processAudioStream(audioStream, async (chunk: AudioChunk) => {
      try {
        await this.processChunk(chunk, onSubtitle);
      } catch (error) {
        console.error('[ParallelProcessor] Error processing chunk:', error);
        PerformanceMonitor.recordError(error as Error);
      }
    });
  }

  private async processChunk(
    chunk: AudioChunk,
    onSubtitle: (subtitle: SubtitleItem) => void
  ): Promise<void> {
    console.log('[ParallelProcessor] Processing audio chunk', {
      size: chunk.data.length,
      sampleRate: chunk.sampleRate,
      timestamp: chunk.timestamp
    });

    try {
      // 翻译音频
      const result = await TranslationService.translateAudio(
        chunk.data,
        this.currentLanguage
      );

      // 记录性能指标
      PerformanceMonitor.recordTranslation(result);

      // 如果有翻译结果，创建字幕
      if (result.translatedText && result.translatedText.trim()) {
        const subtitle: SubtitleItem = {
          id: `subtitle-${chunk.timestamp}`,
          original: result.originalText || '',
          translated: result.translatedText,
          timestamp: chunk.timestamp
        };

        onSubtitle(subtitle);
      }
    } catch (error) {
      console.error('[ParallelProcessor] Translation failed:', error);
      throw error;
    }
  }

  stopProcessing(): void {
    if (this.isProcessing) {
      AudioProcessor.stopProcessing();
      this.isProcessing = false;
      console.log('[ParallelProcessor] Stopped processing');
    }
  }

  isActive(): boolean {
    return this.isProcessing;
  }

  setTargetLanguage(language: string): void {
    this.currentLanguage = language;
    console.log('[ParallelProcessor] Target language changed to:', language);
  }
}

export default ParallelProcessor;
