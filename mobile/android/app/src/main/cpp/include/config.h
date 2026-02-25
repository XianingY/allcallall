#ifndef CONFIG_H_
#define CONFIG_H_

// =============================================
// SentencePiece Configuration
// =============================================
#define VERSION "0.1.99"
#define PACKAGE "sentencepiece"
#define PACKAGE_STRING "sentencepiece"
#define INSTALL_DATADIR "unknown"

// Required for util.h to avoid some platform-specific errors
#if defined(__ANDROID__)
  #define OS_ANDROID
#endif

// Protobuf internal macro (just to be safe)
#define _USE_INTERNAL_PROTOBUF 1

// =============================================
// espeak-ng Configuration
// =============================================
// Minimal configuration - no audio playback, only phoneme conversion

// Android does not use mkstemp in the same way
#define HAVE_MKSTEMP 0

// Disable optional espeak-ng features (we only need text-to-phoneme)
#define USE_ASYNC 0
#define USE_KLATT 0
#define USE_LIBPCAUDIO 0
#define USE_LIBSONIC 0
#define USE_MBROLA 0
#define USE_SPEECHPLAYER 0

// espeak-ng package version
#define PACKAGE_VERSION "1.52.0"

#endif  // CONFIG_H_
