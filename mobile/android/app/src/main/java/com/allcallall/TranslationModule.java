// android/app/src/main/java/com/allcallall/TranslationModule.java
package com.allcallall;

import androidx.annotation.NonNull;
import com.facebook.react.bridge.ReactApplicationContext;
import com.facebook.react.bridge.ReactContextBaseJavaModule;
import com.facebook.react.bridge.ReactMethod;
import com.facebook.react.bridge.Promise;
import com.facebook.react.bridge.WritableMap;
import com.facebook.react.bridge.WritableNativeMap;
import android.util.Log;

public class TranslationModule extends ReactContextBaseJavaModule {
    private static final String TAG = "TranslationModule";
    private final ReactApplicationContext reactContext;
    private boolean isInitialized = false;

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
