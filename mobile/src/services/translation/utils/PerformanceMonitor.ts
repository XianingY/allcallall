// mobile/src/services/translation/utils/PerformanceMonitor.ts
import { TranslationResult } from '../TranslationService';

interface PerformanceMetrics {
  translationCount: number;
  totalTranslationTime: number;
  averageTranslationTime: number;
  averageConfidence: number;
  errorCount: number;
  errorRate: number;
  memoryUsage: number[];
  maxMemoryUsage: number;
  minLatency: number;
  maxLatency: number;
}

class PerformanceMonitor {
  private metrics = {
    translationCount: 0,
    totalTranslationTime: 0,
    totalConfidence: 0,
    errorCount: 0,
    memoryUsage: [] as number[],
    latencies: [] as number[]
  };

  recordTranslation(result: TranslationResult): void {
    this.metrics.translationCount++;
    this.metrics.totalTranslationTime += result.processingTime;
    this.metrics.totalConfidence += result.confidence;
    this.metrics.latencies.push(result.processingTime);

    // 只保留最近 100 条记录
    if (this.metrics.latencies.length > 100) {
      this.metrics.latencies.shift();
    }

    console.log(`[PerformanceMonitor] Translation #${this.metrics.translationCount}`, {
      time: result.processingTime,
      confidence: result.confidence,
      avgTime: this.getAverageTranslationTime()
    });
  }

  recordError(error: Error): void {
    this.metrics.errorCount++;
    console.error('[PerformanceMonitor] Error recorded:', error.message);
  }

  recordMemoryUsage(usage: number): void {
    this.metrics.memoryUsage.push(usage);
    
    // 只保留最近 50 条记录
    if (this.metrics.memoryUsage.length > 50) {
      this.metrics.memoryUsage.shift();
    }
  }

  getMetrics(): PerformanceMetrics {
    const avgTranslationTime = this.getAverageTranslationTime();
    const avgConfidence = this.metrics.translationCount > 0
      ? this.metrics.totalConfidence / this.metrics.translationCount
      : 0;
    const errorRate = this.metrics.translationCount > 0
      ? this.metrics.errorCount / this.metrics.translationCount
      : 0;

    return {
      translationCount: this.metrics.translationCount,
      totalTranslationTime: this.metrics.totalTranslationTime,
      averageTranslationTime: avgTranslationTime,
      averageConfidence: avgConfidence,
      errorCount: this.metrics.errorCount,
      errorRate: errorRate,
      memoryUsage: [...this.metrics.memoryUsage],
      maxMemoryUsage: Math.max(...this.metrics.memoryUsage, 0),
      minLatency: Math.min(...this.metrics.latencies, 0),
      maxLatency: Math.max(...this.metrics.latencies, 0)
    };
  }

  private getAverageTranslationTime(): number {
    if (this.metrics.translationCount === 0) return 0;
    return this.metrics.totalTranslationTime / this.metrics.translationCount;
  }

  reset(): void {
    this.metrics = {
      translationCount: 0,
      totalTranslationTime: 0,
      totalConfidence: 0,
      errorCount: 0,
      memoryUsage: [],
      latencies: []
    };
    console.log('[PerformanceMonitor] Metrics reset');
  }

  getPerformanceReport(): string {
    const metrics = this.getMetrics();
    return `
Performance Report:
==================
Total Translations: ${metrics.translationCount}
Average Time: ${metrics.averageTranslationTime.toFixed(2)}ms
Min/Max Latency: ${metrics.minLatency.toFixed(2)}ms / ${metrics.maxLatency.toFixed(2)}ms
Average Confidence: ${(metrics.averageConfidence * 100).toFixed(2)}%
Error Rate: ${(metrics.errorRate * 100).toFixed(2)}%
Max Memory Usage: ${(metrics.maxMemoryUsage / 1024 / 1024).toFixed(2)}MB
    `.trim();
  }
}

export default new PerformanceMonitor();
