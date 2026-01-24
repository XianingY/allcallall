// mobile/src/screens/TranslationScreen.tsx
import React, { useState, useEffect } from 'react';
import {
  View,
  Text,
  StyleSheet,
  TouchableOpacity,
  ScrollView,
  ActivityIndicator,
  Alert
} from 'react-native';
import TranslationService from '../services/translation/TranslationService';
import TranslationControl from '../components/translation/TranslationControl';
import ModelDownloader from '../services/translation/utils/ModelDownloader';
import PerformanceMonitor from '../services/translation/utils/PerformanceMonitor';

const TranslationScreen: React.FC = () => {
  const [isInitialized, setIsInitialized] = useState(false);
  const [isInitializing, setIsInitializing] = useState(false);
  const [translationEnabled, setTranslationEnabled] = useState(false);
  const [targetLanguage, setTargetLanguage] = useState('zh');
  const [isTranslating, setIsTranslating] = useState(false);
  const [originalText, setOriginalText] = useState('');
  const [translatedText, setTranslatedText] = useState('');
  const [lastLatencyMs, setLastLatencyMs] = useState<number | null>(null);
  const [modelsStatus, setModelsStatus] = useState({
    whisper: false,
    opus: false,
    tts: false
  });

  useEffect(() => {
    checkModelsStatus();
  }, []);

  const checkModelsStatus = async () => {
    const downloader = new ModelDownloader();
    const status = {
      whisper: await downloader.checkModelExists('whisper'),
      opus: await downloader.checkModelExists('opus'),
      tts: await downloader.checkModelExists('tts')
    };
    setModelsStatus(status);
  };

  const initializeTranslation = async () => {
    setIsInitializing(true);
    try {
      await TranslationService.initialize({
        whisperModel: 'small',
        targetLanguage: targetLanguage,
        quantization: 'int8'
      });
      setIsInitialized(true);
      await checkModelsStatus();
      Alert.alert('成功', '翻译服务已初始化');
    } catch (error) {
      console.error('Failed to initialize translation:', error);
      Alert.alert('错误', '翻译服务初始化失败');
    } finally {
      setIsInitializing(false);
    }
  };

  const handleToggleTranslation = (enabled: boolean) => {
    if (enabled && !isInitialized) {
      Alert.alert('提示', '请先初始化翻译服务');
      return;
    }
    setTranslationEnabled(enabled);
  };

  const showPerformanceReport = () => {
    const report = PerformanceMonitor.getPerformanceReport();
    Alert.alert('性能报告', report);
  };

  const recordAndTranslateOnce = async () => {
    if (!isInitialized) {
      Alert.alert('提示', '请先初始化翻译服务');
      return;
    }

    setIsTranslating(true);
    setOriginalText('');
    setTranslatedText('');
    setLastLatencyMs(null);

    const startedAt = Date.now();
    try {
      // Android-only: native mic capture -> offline translation.
      const result = await TranslationService.recordAndTranslate(3000, targetLanguage);
      setOriginalText(result.originalText);
      setTranslatedText(result.translatedText);
      setLastLatencyMs(Date.now() - startedAt);
      PerformanceMonitor.recordTranslation(result);
    } catch (error) {
      console.error('recordAndTranslateOnce failed:', error);
      Alert.alert(
        '错误',
        `录音翻译失败: ${error instanceof Error ? error.message : String(error)}`
      );
    } finally {
      setIsTranslating(false);
    }
  };

  return (
    <ScrollView style={styles.container}>
      <View style={styles.section}>
        <Text style={styles.sectionTitle}>模型状态</Text>
        <View style={styles.statusContainer}>
          <StatusItem label="Whisper" status={modelsStatus.whisper} />
          <StatusItem label="Opus-MT" status={modelsStatus.opus} />
          <StatusItem label="TTS (Piper)" status={modelsStatus.tts} />
        </View>
      </View>

      <View style={styles.section}>
        <Text style={styles.sectionTitle}>服务控制</Text>
        <TouchableOpacity
          style={[
            styles.button,
            isInitialized && styles.buttonSuccess,
            isInitializing && styles.buttonDisabled
          ]}
          onPress={initializeTranslation}
          disabled={isInitializing || isInitialized}
        >
          {isInitializing ? (
            <ActivityIndicator color="#fff" />
          ) : (
            <Text style={styles.buttonText}>
              {isInitialized ? '✓ 已初始化' : '初始化翻译服务'}
            </Text>
          )}
        </TouchableOpacity>

        {isInitialized && (
          <TranslationControl
            isEnabled={translationEnabled}
            onToggle={handleToggleTranslation}
            targetLanguage={targetLanguage}
            onLanguageChange={setTargetLanguage}
            originalText={originalText}
            translatedText={translatedText}
            isTranslating={isTranslating}
          />
        )}

        {isInitialized && (
          <TouchableOpacity
            style={[styles.button, isTranslating && styles.buttonDisabled]}
            onPress={recordAndTranslateOnce}
            disabled={isTranslating}
          >
            {isTranslating ? (
              <ActivityIndicator color="#fff" />
            ) : (
              <Text style={styles.buttonText}>录音 3 秒并翻译（验证）</Text>
            )}
          </TouchableOpacity>
        )}

        {isInitialized && lastLatencyMs != null && (
          <Text style={styles.infoText}>本次耗时: {lastLatencyMs}ms</Text>
        )}
      </View>

      <View style={styles.section}>
        <Text style={styles.sectionTitle}>性能监控</Text>
        <TouchableOpacity
          style={styles.button}
          onPress={showPerformanceReport}
        >
          <Text style={styles.buttonText}>查看性能报告</Text>
        </TouchableOpacity>
      </View>

      <View style={styles.section}>
        <Text style={styles.infoText}>
          实时翻译功能基于离线AI模型，包括：{'\n'}
          • Whisper-small: 语音识别{'\n'}
          • Opus-MT: 文本翻译{'\n'}
          • Piper: 语音合成{'\n\n'}
          总模型大小: ~264MB{'\n'}
          目标延迟: &lt;500ms
        </Text>
      </View>
    </ScrollView>
  );
};

interface StatusItemProps {
  label: string;
  status: boolean;
}

const StatusItem: React.FC<StatusItemProps> = ({ label, status }) => (
  <View style={styles.statusItem}>
    <Text style={styles.statusLabel}>{label}</Text>
    <View style={[styles.statusIndicator, status && styles.statusIndicatorActive]}>
      <Text style={styles.statusText}>{status ? '已下载' : '未下载'}</Text>
    </View>
  </View>
);

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: '#f5f5f5'
  },
  section: {
    backgroundColor: '#fff',
    padding: 16,
    marginVertical: 8,
    borderRadius: 8
  },
  sectionTitle: {
    fontSize: 18,
    fontWeight: 'bold',
    marginBottom: 12,
    color: '#333'
  },
  statusContainer: {
    gap: 8
  },
  statusItem: {
    flexDirection: 'row',
    justifyContent: 'space-between',
    alignItems: 'center',
    paddingVertical: 8
  },
  statusLabel: {
    fontSize: 16,
    color: '#666'
  },
  statusIndicator: {
    backgroundColor: '#e0e0e0',
    paddingHorizontal: 12,
    paddingVertical: 4,
    borderRadius: 12
  },
  statusIndicatorActive: {
    backgroundColor: '#4caf50'
  },
  statusText: {
    color: '#fff',
    fontSize: 12,
    fontWeight: '600'
  },
  button: {
    backgroundColor: '#3b82f6',
    padding: 16,
    borderRadius: 8,
    alignItems: 'center',
    marginVertical: 8
  },
  buttonSuccess: {
    backgroundColor: '#4caf50'
  },
  buttonDisabled: {
    backgroundColor: '#ccc'
  },
  buttonText: {
    color: '#fff',
    fontSize: 16,
    fontWeight: '600'
  },
  infoText: {
    fontSize: 14,
    color: '#666',
    lineHeight: 22
  }
});

export default TranslationScreen;
