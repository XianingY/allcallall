import React, { useMemo } from 'react';
import {
  View,
  Text,
  StyleSheet,
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
}) => {
  const visibleSubtitles = useMemo(() => {
    return subtitles.slice(-3);
  }, [subtitles]);

  if (!isVisible || visibleSubtitles.length === 0) {
    return null;
  }

  return (
    <View style={styles.container} pointerEvents="none">
      {visibleSubtitles.map((subtitle) => {
        const isPartial = !subtitle.isFinal;
        return (
          <View
            key={`${subtitle.segmentId}:${subtitle.revision}`}
            style={[
              styles.subtitleContainer,
              isPartial ? styles.partialSubtitleContainer : styles.finalSubtitleContainer,
            ]}
          >
            {subtitle.original ? (
              <Text style={isPartial ? styles.partialOriginalText : styles.originalText}>
                {subtitle.original}
              </Text>
            ) : null}
            <Text style={isPartial ? styles.partialTranslatedText : styles.translatedText}>
              {subtitle.translated}
            </Text>
            <Text style={styles.metaText}>
              {language === 'zh' ? '中文' : language === 'en' ? 'English' : language}
              {' · '}
              {subtitle.source === 'online' ? '在线' : '对端'}
              {' · '}
              {subtitle.isFinal ? 'final' : 'partial'}
            </Text>
          </View>
        );
      })}
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
    zIndex: 1000,
    gap: 8,
  },
  subtitleContainer: {
    borderRadius: 12,
    paddingHorizontal: 16,
    paddingVertical: 12,
    maxWidth: width * 0.92,
    alignItems: 'center',
    borderWidth: 1,
  },
  finalSubtitleContainer: {
    backgroundColor: 'rgba(0, 0, 0, 0.84)',
    borderColor: 'rgba(255, 255, 255, 0.16)',
  },
  partialSubtitleContainer: {
    backgroundColor: 'rgba(0, 0, 0, 0.56)',
    borderColor: 'rgba(255, 255, 255, 0.08)',
  },
  originalText: {
    color: '#d1d5db',
    fontSize: 13,
    marginBottom: 4,
    textAlign: 'center',
  },
  partialOriginalText: {
    color: '#9ca3af',
    fontSize: 12,
    marginBottom: 4,
    textAlign: 'center',
  },
  translatedText: {
    color: '#ffffff',
    fontSize: 18,
    fontWeight: '700',
    textAlign: 'center',
  },
  partialTranslatedText: {
    color: '#d1d5db',
    fontSize: 16,
    fontWeight: '500',
    textAlign: 'center',
  },
  metaText: {
    marginTop: 4,
    color: '#60a5fa',
    fontSize: 11,
  },
});

export default TranslationOverlay;
