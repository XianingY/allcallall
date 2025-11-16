package com.allcallall.mobile

import android.os.Build
import android.os.Bundle
import android.content.Intent
import android.net.Uri

import com.facebook.react.ReactActivity
import com.facebook.react.ReactActivityDelegate
import com.facebook.react.defaults.DefaultNewArchitectureEntryPoint.fabricEnabled
import com.facebook.react.defaults.DefaultReactActivityDelegate

import expo.modules.ReactActivityDelegateWrapper

class MainActivity : ReactActivity() {
  override fun onCreate(savedInstanceState: Bundle?) {
    // Set the theme to AppTheme BEFORE onCreate to support
    // coloring the background, status bar, and navigation bar.
    // This is required for expo-splash-screen.
    setTheme(R.style.AppTheme)
    
    // 解决localhost问题：在Intent中修正开发服务器地址
    val intent = intent
    if (intent != null && intent.action == Intent.ACTION_VIEW) {
      val uri = intent.data
      if (uri != null) {
        val uriString = uri.toString()
        android.util.Log.d("MainActivity", "Original URI: $uriString")
        
        // 如果URL中包含localhost或127.0.0.1，替换为实际的LAN IP
        if (uriString.contains("localhost") || uriString.contains("127.0.0.1")) {
          val correctedUri = uriString
            .replace("localhost:8081", "192.168.1.30:8081")
            .replace("127.0.0.1:8081", "192.168.1.30:8081")
            .replace("localhost", "192.168.1.30")
            .replace("127.0.0.1", "192.168.1.30")
          android.util.Log.d("MainActivity", "Corrected URI: $correctedUri")
          intent.data = Uri.parse(correctedUri)
        }
      }
    }
    
    super.onCreate(null)
  }

  override fun getMainComponentName(): String = "main"

  override fun createReactActivityDelegate(): ReactActivityDelegate {
    return ReactActivityDelegateWrapper(
          this,
          BuildConfig.IS_NEW_ARCHITECTURE_ENABLED,
          object : DefaultReactActivityDelegate(
              this,
              mainComponentName,
              fabricEnabled
          ){})
  }

  override fun invokeDefaultOnBackPressed() {
      if (Build.VERSION.SDK_INT <= Build.VERSION_CODES.R) {
          if (!moveTaskToBack(false)) {
              super.invokeDefaultOnBackPressed()
          }
          return
      }

      super.invokeDefaultOnBackPressed()
  }
}
