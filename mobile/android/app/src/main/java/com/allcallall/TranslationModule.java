// android/app/src/main/java/com/allcallall/TranslationModule.java
package com.allcallall;

import android.media.AudioFormat;
import android.media.AudioRecord;
import android.media.MediaRecorder;
import android.os.SystemClock;
import android.util.Base64;
import androidx.annotation.NonNull;
import com.facebook.react.bridge.ReactApplicationContext;
import com.facebook.react.bridge.ReactContextBaseJavaModule;
import com.facebook.react.bridge.ReactMethod;
import com.facebook.react.bridge.Promise;
import com.facebook.react.modules.core.DeviceEventManagerModule;
import com.facebook.react.bridge.WritableMap;
import com.facebook.react.bridge.WritableNativeMap;
import android.util.Log;

import java.nio.ByteBuffer;
import java.nio.ByteOrder;

import java.util.Arrays;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

public class TranslationModule extends ReactContextBaseJavaModule {
    private static final String TAG = "TranslationModule";
    private final ReactApplicationContext reactContext;
    private boolean isInitialized = false;

    private static volatile TranslationModule instance = null;

    private final Object micLock = new Object();
    private final ExecutorService micExecutor = Executors.newSingleThreadExecutor();
    private boolean micRunning = false;
    private boolean micProcessing = false;
    private int micChunkSamples = 16000 * 2;
    private String micTargetLanguage = "zh";
    private float[] micBuffer = new float[16000 * 10];
    private int micBufferLen = 0;

    // 加载 native 库
    static {
        try {
            System.loadLibrary("translation-lib");
            Log.i(TAG, "Native library loaded successfully");
        } catch (UnsatisfiedLinkError e) {
            Log.e(TAG, "Failed to load native library", e);
        }
    }

    public TranslationModule(ReactApplicationContext reactContext) {
        super(reactContext);
        this.reactContext = reactContext;
        instance = this;
    }

    @NonNull
    @Override
    public String getName() {
        return "TranslationModule";
    }

    @ReactMethod
    public void initialize(
        String whisperPath,
        String opusPath,
        String ttsPath,
        String quantization,
        Promise promise
    ) {
        try {
            Log.d(TAG, "Initializing translation models...");
            Log.d(TAG, "Whisper path: " + whisperPath);
            Log.d(TAG, "Opus path: " + opusPath);
            Log.d(TAG, "TTS path: " + ttsPath);
            Log.d(TAG, "Quantization: " + quantization);

            nativeInitialize(
                whisperPath,
                opusPath,
                ttsPath,
                quantization
            );
            
            isInitialized = true;
            Log.i(TAG, "Translation models initialized successfully");
            promise.resolve(true);
        } catch (Exception e) {
            Log.e(TAG, "Failed to initialize translation models", e);
            promise.reject("INIT_ERROR", e.getMessage());
        }
    }

    @ReactMethod
    public void translateAudio(
        String audioDataBase64,
        String targetLanguage,
        Promise promise
    ) {
        if (!isInitialized) {
            promise.reject("NOT_INITIALIZED", "Translation service not initialized");
            return;
        }

        try {
            Log.d(TAG, "Translating audio to: " + targetLanguage);
            
            String jsonResult = nativeTranslateAudio(
                audioDataBase64,
                targetLanguage
            );
            
            // Parse JSON response from native code
            org.json.JSONObject json = new org.json.JSONObject(jsonResult);

            String nativeError = json.optString("error", "");
            if (nativeError != null && !nativeError.isEmpty()) {
                promise.reject("NATIVE_ERROR", nativeError);
                return;
            }
            
            WritableMap response = new WritableNativeMap();
            response.putString("originalText", json.optString("originalText", ""));
            response.putString("translatedText", json.optString("translatedText", ""));
            response.putDouble("confidence", json.optDouble("confidence", 0.9));
            response.putString("audioBase64", json.optString("audioBase64", ""));
            
            Log.d(TAG, "Translation complete: " + json.optString("translatedText"));
            promise.resolve(response);
        } catch (org.json.JSONException e) {
            Log.e(TAG, "Failed to parse translation result", e);
            promise.reject("PARSE_ERROR", e.getMessage());
        } catch (Exception e) {
            Log.e(TAG, "Translation failed", e);
            promise.reject("TRANSLATE_ERROR", e.getMessage());
        }
    }

    /**
     * Phase-2 validation helper: record mic PCM (16kHz mono) and run offline translation.
     *
     * NOTE: Do not use this during an active WebRTC call. WebRTC already owns the mic on Android.
     */
    @ReactMethod
    public void recordAndTranslate(
        int durationMs,
        String targetLanguage,
        Promise promise
    ) {
        if (!isInitialized) {
            promise.reject("NOT_INITIALIZED", "Translation service not initialized");
            return;
        }

        final int sampleRate = 16000;
        final int channelConfig = AudioFormat.CHANNEL_IN_MONO;
        final int audioFormat = AudioFormat.ENCODING_PCM_16BIT;
        final int minBuffer = AudioRecord.getMinBufferSize(sampleRate, channelConfig, audioFormat);
        if (minBuffer <= 0) {
            promise.reject("RECORDER_ERROR", "Failed to get min buffer size for AudioRecord");
            return;
        }

        final int bufferSize = Math.max(minBuffer, sampleRate * 2); // at least ~1s of PCM16
        AudioRecord recorder = null;

        try {
            recorder = new AudioRecord(
                MediaRecorder.AudioSource.VOICE_RECOGNITION,
                sampleRate,
                channelConfig,
                audioFormat,
                bufferSize
            );

            if (recorder.getState() != AudioRecord.STATE_INITIALIZED) {
                promise.reject("RECORDER_ERROR", "AudioRecord not initialized");
                return;
            }

            recorder.startRecording();

            final long deadline = SystemClock.elapsedRealtime() + Math.max(200, durationMs);
            final short[] pcm16 = new short[bufferSize / 2];

            // Float32 bytes (little-endian) to match native decoder (memcpy into float vector).
            final ByteBuffer floatBytes = ByteBuffer.allocate((sampleRate * durationMs / 1000 + sampleRate) * 4)
                .order(ByteOrder.LITTLE_ENDIAN);

            while (SystemClock.elapsedRealtime() < deadline) {
                int read = recorder.read(pcm16, 0, pcm16.length);
                if (read <= 0) {
                    continue;
                }
                for (int i = 0; i < read; i++) {
                    float f = pcm16[i] / 32768.0f;
                    floatBytes.putFloat(f);
                }
            }

            floatBytes.flip();
            byte[] raw = new byte[floatBytes.remaining()];
            floatBytes.get(raw);

            String audioBase64 = Base64.encodeToString(raw, Base64.NO_WRAP);
            String jsonResult = nativeTranslateAudio(audioBase64, targetLanguage);

            org.json.JSONObject json = new org.json.JSONObject(jsonResult);
            String nativeError = json.optString("error", "");
            if (nativeError != null && !nativeError.isEmpty()) {
                promise.reject("NATIVE_ERROR", nativeError);
                return;
            }

            WritableMap response = new WritableNativeMap();
            response.putString("originalText", json.optString("originalText", ""));
            response.putString("translatedText", json.optString("translatedText", ""));
            response.putDouble("confidence", json.optDouble("confidence", 0.9));
            response.putString("audioBase64", json.optString("audioBase64", ""));

            promise.resolve(response);
        } catch (Exception e) {
            Log.e(TAG, "recordAndTranslate failed", e);
            promise.reject("RECORDER_ERROR", e.getMessage());
        } finally {
            if (recorder != null) {
                try {
                    recorder.stop();
                } catch (Exception ignored) {}
                try {
                    recorder.release();
                } catch (Exception ignored) {}
            }
        }
    }

    @ReactMethod
    public void startCallMicTranslation(
        String targetLanguage,
        int chunkMs,
        Promise promise
    ) {
        if (!isInitialized) {
            promise.reject("NOT_INITIALIZED", "Translation service not initialized");
            return;
        }
        if (targetLanguage == null || targetLanguage.isEmpty()) {
            promise.reject("INVALID_ARGUMENT", "targetLanguage is required");
            return;
        }
        if (chunkMs < 500) {
            chunkMs = 500;
        }

        final int chunkSamples = (int) Math.max(16000, (long) 16000 * (long) chunkMs / 1000L);
        synchronized (micLock) {
            micTargetLanguage = targetLanguage;
            micChunkSamples = chunkSamples;
            micBufferLen = 0;
            micProcessing = false;
            micRunning = true;
        }
        Log.i(TAG, "Call mic translation enabled (chunkSamples=" + chunkSamples + ")");
        promise.resolve(true);
    }

    @ReactMethod
    public void stopCallMicTranslation(Promise promise) {
        synchronized (micLock) {
            micRunning = false;
            micBufferLen = 0;
            micProcessing = false;
        }
        promise.resolve(true);
    }

    public static void onWebRtcAudioRecordSamplesReady(
        byte[] pcmBytes,
        int sampleRate,
        int channelCount
    ) {
        TranslationModule inst = instance;
        if (inst == null) return;
        inst.handleWebRtcMicSamples(pcmBytes, sampleRate, channelCount);
    }

    private void handleWebRtcMicSamples(byte[] pcmBytes, int sampleRate, int channelCount) {
        if (pcmBytes == null || pcmBytes.length == 0) return;

        final float[] f32;
        synchronized (micLock) {
            if (!micRunning) return;
        }

        // Expect PCM16LE.
        if ((pcmBytes.length % 2) != 0) return;

        final int totalSamples = pcmBytes.length / 2;
        final int frames = channelCount > 0 ? totalSamples / channelCount : totalSamples;

        // Downmix to mono by taking channel 0.
        final short[] mono = new short[frames];
        ByteBuffer.wrap(pcmBytes).order(ByteOrder.LITTLE_ENDIAN);
        for (int i = 0; i < frames; i++) {
            int byteIndex = (i * channelCount) * 2;
            if (byteIndex + 1 >= pcmBytes.length) break;
            short s = (short) ((pcmBytes[byteIndex] & 0xff) | (pcmBytes[byteIndex + 1] << 8));
            mono[i] = s;
        }

        // Resample to 16kHz. WebRTC mic is commonly 48kHz.
        if (sampleRate == 48000) {
            int outLen = frames / 3;
            f32 = new float[outLen];
            int oi = 0;
            for (int i = 0; i + 2 < frames; i += 3) {
                f32[oi++] = mono[i] / 32768.0f;
            }
        } else if (sampleRate == 16000) {
            f32 = new float[frames];
            for (int i = 0; i < frames; i++) {
                f32[i] = mono[i] / 32768.0f;
            }
        } else {
            return;
        }

        maybeEnqueueMicChunk(f32, System.currentTimeMillis());
    }

    private void maybeEnqueueMicChunk(final float[] newSamples, final long tsMs) {
        final float[] chunkToProcess;
        final String targetLang;
        synchronized (micLock) {
            if (!micRunning) return;

            int needed = micBufferLen + newSamples.length;
            if (needed > micBuffer.length) {
                int newCap = Math.max(needed, micBuffer.length * 2);
                micBuffer = Arrays.copyOf(micBuffer, newCap);
            }
            System.arraycopy(newSamples, 0, micBuffer, micBufferLen, newSamples.length);
            micBufferLen += newSamples.length;

            if (micProcessing || micBufferLen < micChunkSamples) {
                return;
            }

            chunkToProcess = Arrays.copyOf(micBuffer, micChunkSamples);
            micBufferLen -= micChunkSamples;
            if (micBufferLen > 0) {
                System.arraycopy(micBuffer, micChunkSamples, micBuffer, 0, micBufferLen);
            }
            micProcessing = true;
            targetLang = micTargetLanguage;
        }

        micExecutor.submit(() -> {
            try {
                String resultJson = encodeAndTranslateFloat32(chunkToProcess, targetLang);
                if (resultJson == null) return;

                org.json.JSONObject json = new org.json.JSONObject(resultJson);
                String nativeError = json.optString("error", "");
                if (nativeError != null && !nativeError.isEmpty()) {
                    Log.w(TAG, "Mic translation native error: " + nativeError);
                    return;
                }

                WritableMap payload = new WritableNativeMap();
                payload.putString("originalText", json.optString("originalText", ""));
                payload.putString("translatedText", json.optString("translatedText", ""));
                payload.putDouble("confidence", json.optDouble("confidence", 0.9));
                payload.putString("audioBase64", json.optString("audioBase64", ""));
                payload.putDouble("timestampMs", (double) tsMs);

                reactContext
                    .getJSModule(DeviceEventManagerModule.RCTDeviceEventEmitter.class)
                    .emit("offlineTranslationMicSubtitle", payload);
            } catch (Exception e) {
                Log.e(TAG, "Mic translation processing error", e);
            } finally {
                synchronized (micLock) {
                    micProcessing = false;
                }
            }
        });
    }

    private String encodeAndTranslateFloat32(float[] samples, String targetLanguage) {
        try {
            ByteBuffer floatBytes = ByteBuffer.allocate(samples.length * 4).order(ByteOrder.LITTLE_ENDIAN);
            for (float f : samples) {
                floatBytes.putFloat(f);
            }
            byte[] raw = floatBytes.array();
            String audioBase64 = Base64.encodeToString(raw, Base64.NO_WRAP);
            return nativeTranslateAudio(audioBase64, targetLanguage);
        } catch (Exception e) {
            Log.e(TAG, "encodeAndTranslateFloat32 failed", e);
            return null;
        }
    }

    @ReactMethod
    public void cleanup(Promise promise) {
        try {
            if (isInitialized) {
                nativeCleanup();
                isInitialized = false;
                Log.i(TAG, "Translation service cleaned up");
            }
            promise.resolve(true);
        } catch (Exception e) {
            Log.e(TAG, "Cleanup failed", e);
            promise.reject("CLEANUP_ERROR", e.getMessage());
        }
    }

    // Native 方法声明
    private native void nativeInitialize(
        String whisperPath,
        String opusPath,
        String ttsPath,
        String quantization
    );

    private native String nativeTranslateAudio(
        String audioDataBase64,
        String targetLanguage
    );

    private native void nativeCleanup();
}
