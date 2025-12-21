// mobile/src/components/translation/TranslationControl.tsx
import React, { useState, useCallback } from 'react';
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

interface TranslationControlProps {
  isEnabled: boolean;
  onToggle: (enabled: boolean) => void;
  targetLanguage: string;
  onLanguageChange: (language: string) => void;
  // New props for translation display
  originalText?: string;
  translatedText?: string;
  isTranslating?: boolean;
  detectedLanguage?: string;
  onPlayAudio?: () => void;
}

const SUPPORTED_LANGUAGES = [
  { code: 'zh', name: 'Chinese', nativeName: '中文' },
  { code: 'en', name: 'English', nativeName: 'English' },
  { code: 'ja', name: 'Japanese', nativeName: '日本語' },
  { code: 'ko', name: 'Korean', nativeName: '한국어' },
  { code: 'es', name: 'Spanish', nativeName: 'Español' },
  { code: 'fr', name: 'French', nativeName: 'Français' }
];

const TranslationControl: React.FC<TranslationControlProps> = ({
  isEnabled,
  onToggle,
  targetLanguage,
  onLanguageChange,
  originalText = '',
  translatedText = '',
  isTranslating = false,
  detectedLanguage = '',
  onPlayAudio
}) => {
  const [showLanguagePicker, setShowLanguagePicker] = useState(false);
  const currentLanguage = SUPPORTED_LANGUAGES.find(
    lang => lang.code === targetLanguage
  );

  // Determine translation direction display
  const getDirectionText = useCallback(() => {
    if (!detectedLanguage) return '';
    const isEnglish = detectedLanguage === 'en';
    if (targetLanguage === 'zh') {
      return isEnglish ? '英 → 中' : '中 → 中';
    } else if (targetLanguage === 'en') {
      return isEnglish ? '英 → 英' : '中 → 英';
    }
    return '';
  }, [detectedLanguage, targetLanguage]);

  return (
    <View style={styles.container}>
      {/* Control Bar */}
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

        {isEnabled && (
          <TouchableOpacity
            style={styles.languageButton}
            onPress={() => setShowLanguagePicker(true)}
          >
            <Text style={styles.languageButtonText}>
              {currentLanguage?.nativeName || '选择语言'}
            </Text>
          </TouchableOpacity>
        )}
      </View>

      {/* Translation Display Area */}
      {isEnabled && (
        <View style={styles.translationDisplay}>
          {/* Direction Indicator */}
          {detectedLanguage && (
            <View style={styles.directionContainer}>
              <Text style={styles.directionText}>{getDirectionText()}</Text>
            </View>
          )}

          {/* Loading Indicator */}
          {isTranslating && (
            <View style={styles.loadingContainer}>
              <ActivityIndicator size="small" color="#3b82f6" />
              <Text style={styles.loadingText}>翻译中...</Text>
            </View>
          )}

          {/* Original Text */}
          {originalText && (
            <View style={styles.textContainer}>
              <Text style={styles.textLabel}>原文:</Text>
              <Text style={styles.originalText}>{originalText}</Text>
            </View>
          )}

          {/* Translated Text */}
          {translatedText && (
            <View style={styles.textContainer}>
              <Text style={styles.textLabel}>译文:</Text>
              <Text style={styles.translatedText}>{translatedText}</Text>
            </View>
          )}

          {/* Play Audio Button */}
          {translatedText && onPlayAudio && (
            <TouchableOpacity
              style={styles.playButton}
              onPress={onPlayAudio}
            >
              <Text style={styles.playButtonIcon}>🔊</Text>
              <Text style={styles.playButtonText}>播放翻译</Text>
            </TouchableOpacity>
          )}
        </View>
      )}

      {/* Language Picker Modal */}
      <Modal
        visible={showLanguagePicker}
        transparent={true}
        animationType="fade"
        onRequestClose={() => setShowLanguagePicker(false)}
      >
        <TouchableOpacity
          style={styles.modalOverlay}
          activeOpacity={1}
          onPress={() => setShowLanguagePicker(false)}
        >
          <View style={styles.languagePicker}>
            <Text style={styles.pickerTitle}>选择目标语言</Text>
            <ScrollView>
              {SUPPORTED_LANGUAGES.map(lang => (
                <TouchableOpacity
                  key={lang.code}
                  style={[
                    styles.languageOption,
                    targetLanguage === lang.code && styles.selectedOption
                  ]}
                  onPress={() => {
                    onLanguageChange(lang.code);
                    setShowLanguagePicker(false);
                  }}
                >
                  <Text
                    style={[
                      styles.languageOptionText,
                      targetLanguage === lang.code && styles.selectedOptionText
                    ]}
                  >
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
  languageButton: {
    backgroundColor: '#3b82f6',
    borderRadius: 20,
    paddingHorizontal: 16,
    paddingVertical: 8,
    marginLeft: 12
  },
  languageButtonText: {
    color: '#fff',
    fontSize: 14,
    fontWeight: '600'
  },
  // Translation Display Styles
  translationDisplay: {
    backgroundColor: 'rgba(255, 255, 255, 0.05)',
    borderRadius: 12,
    padding: 16,
    marginTop: 12
  },
  directionContainer: {
    alignItems: 'center',
    marginBottom: 12
  },
  directionText: {
    color: '#81b0ff',
    fontSize: 18,
    fontWeight: '600'
  },
  loadingContainer: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'center',
    paddingVertical: 8
  },
  loadingText: {
    color: '#999',
    fontSize: 14,
    marginLeft: 8
  },
  textContainer: {
    marginVertical: 8
  },
  textLabel: {
    color: '#999',
    fontSize: 12,
    marginBottom: 4
  },
  originalText: {
    color: '#ccc',
    fontSize: 16,
    lineHeight: 22
  },
  translatedText: {
    color: '#fff',
    fontSize: 18,
    fontWeight: '500',
    lineHeight: 26
  },
  playButton: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'center',
    backgroundColor: '#3b82f6',
    borderRadius: 20,
    paddingHorizontal: 20,
    paddingVertical: 10,
    marginTop: 12,
    alignSelf: 'center'
  },
  playButtonIcon: {
    fontSize: 18,
    marginRight: 8
  },
  playButtonText: {
    color: '#fff',
    fontSize: 14,
    fontWeight: '600'
  },
  // Modal Styles
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
    fontWeight: 'bold',
    marginBottom: 16,
    textAlign: 'center',
    color: '#333'
  },
  languageOption: {
    paddingVertical: 12,
    borderBottomWidth: 1,
    borderBottomColor: '#f0f0f0'
  },
  selectedOption: {
    backgroundColor: '#e3f2fd'
  },
  languageOptionText: {
    fontSize: 16,
    textAlign: 'center',
    color: '#333'
  },
  selectedOptionText: {
    color: '#3b82f6',
    fontWeight: 'bold'
  }
});

export default TranslationControl;
