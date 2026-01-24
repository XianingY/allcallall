// mobile/src/components/translation/TranslationOverlay.tsx
import React, { useState, useEffect } from 'react';
import {
  View,
  Text,
  StyleSheet,
  Animated,
  Dimensions
} from 'react-native';

import { SubtitleItem } from '../../store/useSubtitleStore';

interface TranslationOverlayProps {
  subtitles: SubtitleItem[];
  isVisible: boolean;
  language: string;
  onClear?: () => void;
}

const { width } = Dimensions.get('window');

const TranslationOverlay: React.FC<TranslationOverlayProps> = ({
  subtitles,
  isVisible,
  language,
  onClear
}) => {
  const [fadeAnim] = useState(new Animated.Value(0));

  useEffect(() => {
    if (isVisible && subtitles.length > 0) {
      Animated.sequence([
        Animated.timing(fadeAnim, {
          toValue: 1,
          duration: 300,
          useNativeDriver: true
        }),
        Animated.delay(3000),
        Animated.timing(fadeAnim, {
          toValue: 0,
          duration: 500,
          useNativeDriver: true
        })
      ]).start(() => {
        if (onClear) {
          onClear();
        }
      });
    }
  }, [subtitles, isVisible]);

  if (!isVisible || subtitles.length === 0) {
    return null;
  }

  const latestSubtitle = subtitles[subtitles.length - 1];

  return (
    <View style={styles.container}>
      <Animated.View
        style={[
          styles.subtitleContainer,
          { opacity: fadeAnim }
        ]}
      >
        {latestSubtitle.original && (
          <Text style={styles.originalText}>
            {latestSubtitle.original}
          </Text>
        )}
        <Text style={styles.translatedText}>
          {latestSubtitle.translated}
        </Text>
        <Text style={styles.languageTag}>
          {language === 'zh' ? '中文' : language === 'en' ? 'English' : language}
        </Text>
      </Animated.View>
    </View>
  );
};

const styles = StyleSheet.create({
  container: {
    position: 'absolute',
    bottom: 120,
    left: 0,
    right: 0,
    alignItems: 'center',
    zIndex: 1000
  },
  subtitleContainer: {
    backgroundColor: 'rgba(0, 0, 0, 0.8)',
    borderRadius: 12,
    paddingHorizontal: 16,
    paddingVertical: 12,
    maxWidth: width * 0.9,
    alignItems: 'center'
  },
  originalText: {
    color: '#ccc',
    fontSize: 14,
    marginBottom: 4,
    textAlign: 'center'
  },
  translatedText: {
    color: '#fff',
    fontSize: 18,
    fontWeight: 'bold',
    textAlign: 'center'
  },
  languageTag: {
    color: '#3b82f6',
    fontSize: 12,
    marginTop: 4
  }
});

export default TranslationOverlay;
