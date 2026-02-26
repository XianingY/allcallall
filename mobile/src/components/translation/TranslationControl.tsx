import React, { useMemo, useState } from 'react';
import {
  View,
  Text,
  StyleSheet,
  Switch,
  TouchableOpacity,
  Modal,
  ScrollView,
  ActivityIndicator
} from 'react-native';

export type TranslationMode = 'offline' | 'online' | 'hybrid';
export type TranslationOnlineStatus = 'idle' | 'connecting' | 'connected' | 'fallback' | 'retrying' | 'error';

interface TranslationControlProps {
  isEnabled: boolean;
  onToggle: (enabled: boolean) => void;
  targetLanguage?: string;
  onLanguageChange?: (language: string) => void;
  sourceLanguage?: string;
  onSourceLanguageChange?: (language: string) => void;
  onTargetLanguageChange?: (language: string) => void;
  translationMode?: TranslationMode;
  onlineStatus?: TranslationOnlineStatus;
  fallbackReason?: string | null;
  translationServiceStatus?: 'idle' | 'initializing' | 'ready' | 'failed';
  translationServiceError?: string | null;
  onRetryInitialize?: () => void;
  originalText?: string;
  translatedText?: string;
  isTranslating?: boolean;
  detectedLanguage?: string;
  onPlayAudio?: () => void;
}

const SUPPORTED_LANGUAGES = [
  { code: 'zh', name: 'Chinese', nativeName: '中文' },
  { code: 'en', name: 'English', nativeName: 'English' }
];

const modeLabel = (mode: TranslationMode) => {
  switch (mode) {
    case 'offline':
      return 'offline';
    case 'online':
      return 'online';
    case 'hybrid':
    default:
      return 'hybrid';
  }
};

const onlineStatusLabel = (status: TranslationOnlineStatus) => {
  switch (status) {
    case 'connected':
      return '在线已连接';
    case 'connecting':
      return '在线连接中';
    case 'fallback':
      return '离线兜底中';
    case 'retrying':
      return '在线重试中';
    case 'error':
      return '在线异常';
    default:
      return '待机';
  }
};

const TranslationControl: React.FC<TranslationControlProps> = ({
  isEnabled,
  onToggle,
  targetLanguage = 'en',
  onLanguageChange,
  sourceLanguage = 'zh',
  onSourceLanguageChange,
  onTargetLanguageChange,
  translationMode = 'hybrid',
  onlineStatus = 'idle',
  fallbackReason = null,
  translationServiceStatus = 'idle',
  translationServiceError = null,
  onRetryInitialize,
}) => {
  const [pickerType, setPickerType] = useState<'source' | 'target' | null>(null);

  const effectiveTargetSetter = onTargetLanguageChange ?? onLanguageChange;
  const source = useMemo(
    () => SUPPORTED_LANGUAGES.find((lang) => lang.code === sourceLanguage),
    [sourceLanguage]
  );
  const target = useMemo(
    () => SUPPORTED_LANGUAGES.find((lang) => lang.code === targetLanguage),
    [targetLanguage]
  );

  return (
    <View style={styles.container}>
      <View style={styles.controlBar}>
        <View style={styles.toggleContainer}>
          <Text style={styles.label}>实时翻译</Text>
          <Switch
            value={isEnabled}
            onValueChange={onToggle}
            trackColor={{ false: '#767577', true: '#81b0ff' }}
            thumbColor={isEnabled ? '#3b82f6' : '#f4f3f4'}
          />
        </View>
        <View style={styles.modeBadge}>
          <Text style={styles.modeBadgeText}>{modeLabel(translationMode)}</Text>
        </View>
      </View>

      {isEnabled ? (
        <View style={styles.translationPanel}>
          <View style={styles.languageRow}>
            <TouchableOpacity
              style={styles.languageButton}
              onPress={() => setPickerType('source')}
            >
              <Text style={styles.languageButtonLabel}>源语言</Text>
              <Text style={styles.languageButtonValue}>{source?.nativeName ?? '未设置'}</Text>
            </TouchableOpacity>

            <Text style={styles.arrow}>→</Text>

            <TouchableOpacity
              style={styles.languageButton}
              onPress={() => setPickerType('target')}
            >
              <Text style={styles.languageButtonLabel}>目标语言</Text>
              <Text style={styles.languageButtonValue}>{target?.nativeName ?? '未设置'}</Text>
            </TouchableOpacity>
          </View>

          <View style={styles.statusRow}>
            <Text style={styles.statusText}>链路状态：{onlineStatusLabel(onlineStatus)}</Text>
            {translationServiceStatus === 'initializing' ? (
              <ActivityIndicator size="small" color="#3b82f6" />
            ) : null}
          </View>

          {translationServiceStatus === 'failed' ? (
            <View style={styles.errorBox}>
              <Text style={styles.errorText} numberOfLines={2}>
                初始化失败：{translationServiceError || '未知错误'}
              </Text>
              {onRetryInitialize ? (
                <TouchableOpacity style={styles.retryButton} onPress={onRetryInitialize}>
                  <Text style={styles.retryButtonText}>重试</Text>
                </TouchableOpacity>
              ) : null}
            </View>
          ) : null}

          {fallbackReason ? (
            <Text style={styles.fallbackText}>回退原因：{fallbackReason}</Text>
          ) : null}
        </View>
      ) : null}

      <Modal
        visible={pickerType !== null}
        transparent={true}
        animationType="fade"
        onRequestClose={() => setPickerType(null)}
      >
        <TouchableOpacity
          style={styles.modalOverlay}
          activeOpacity={1}
          onPress={() => setPickerType(null)}
        >
          <View style={styles.languagePicker}>
            <Text style={styles.pickerTitle}>{pickerType === 'source' ? '选择源语言' : '选择目标语言'}</Text>
            <ScrollView>
              {SUPPORTED_LANGUAGES.map((lang) => (
                <TouchableOpacity
                  key={lang.code}
                  style={styles.languageOption}
                  onPress={() => {
                    if (pickerType === 'source') {
                      onSourceLanguageChange?.(lang.code);
                    } else {
                      effectiveTargetSetter?.(lang.code);
                    }
                    setPickerType(null);
                  }}
                >
                  <Text style={styles.languageOptionText}>
                    {lang.nativeName} ({lang.name})
                  </Text>
                </TouchableOpacity>
              ))}
            </ScrollView>
          </View>
        </TouchableOpacity>
      </Modal>
    </View>
  );
};

const styles = StyleSheet.create({
  container: {
    marginVertical: 8
  },
  controlBar: {
    flexDirection: 'row',
    alignItems: 'center',
    backgroundColor: 'rgba(255, 255, 255, 0.1)',
    borderRadius: 12,
    paddingHorizontal: 16,
    paddingVertical: 12
  },
  toggleContainer: {
    flexDirection: 'row',
    alignItems: 'center',
    flex: 1
  },
  label: {
    color: '#fff',
    fontSize: 16,
    marginRight: 12,
    fontWeight: '500'
  },
  modeBadge: {
    backgroundColor: 'rgba(59,130,246,0.2)',
    borderWidth: 1,
    borderColor: 'rgba(59,130,246,0.5)',
    borderRadius: 999,
    paddingHorizontal: 10,
    paddingVertical: 4
  },
  modeBadgeText: {
    color: '#bfdbfe',
    fontSize: 12,
    fontWeight: '700'
  },
  translationPanel: {
    backgroundColor: 'rgba(255, 255, 255, 0.05)',
    borderRadius: 12,
    padding: 12,
    marginTop: 10,
    gap: 10
  },
  languageRow: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
    gap: 8
  },
  languageButton: {
    flex: 1,
    backgroundColor: 'rgba(30,41,59,0.7)',
    borderRadius: 10,
    paddingHorizontal: 10,
    paddingVertical: 8
  },
  languageButtonLabel: {
    color: '#94a3b8',
    fontSize: 11
  },
  languageButtonValue: {
    color: '#fff',
    marginTop: 2,
    fontSize: 15,
    fontWeight: '600'
  },
  arrow: {
    color: '#93c5fd',
    fontSize: 18,
    fontWeight: '700'
  },
  statusRow: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between'
  },
  statusText: {
    color: '#dbeafe',
    fontSize: 13
  },
  errorBox: {
    borderRadius: 10,
    paddingHorizontal: 10,
    paddingVertical: 8,
    backgroundColor: 'rgba(220, 38, 38, 0.22)',
    flexDirection: 'row',
    alignItems: 'center',
    gap: 8
  },
  errorText: {
    color: '#fecaca',
    fontSize: 12,
    flex: 1
  },
  retryButton: {
    backgroundColor: '#ef4444',
    borderRadius: 10,
    paddingHorizontal: 10,
    paddingVertical: 6
  },
  retryButtonText: {
    color: '#fff',
    fontSize: 12,
    fontWeight: '700'
  },
  fallbackText: {
    color: '#fcd34d',
    fontSize: 12
  },
  modalOverlay: {
    flex: 1,
    backgroundColor: 'rgba(0, 0, 0, 0.5)',
    justifyContent: 'center',
    alignItems: 'center'
  },
  languagePicker: {
    backgroundColor: '#fff',
    borderRadius: 12,
    padding: 20,
    width: '80%',
    maxHeight: '60%'
  },
  pickerTitle: {
    fontSize: 18,
    fontWeight: '700',
    marginBottom: 16,
    textAlign: 'center',
    color: '#111827'
  },
  languageOption: {
    paddingVertical: 12,
    borderBottomWidth: 1,
    borderBottomColor: '#f3f4f6'
  },
  languageOptionText: {
    fontSize: 16,
    textAlign: 'center',
    color: '#111827'
  }
});

export default TranslationControl;
