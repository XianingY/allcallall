/**
 * 推送通知服务
 * 支持后台和被杀进程状态下的来电通知
 *
 * 功能特性：
 * - FCM (Firebase Cloud Messaging) 集成
 * - APNs (Apple Push Notification Service) 支持
 * - 推送权限管理
 * - 推送消息处理
 * - 来电通知处理
 */
import { Platform } from 'react-native';
import { NavigationContainerRef } from '@react-navigation/native';

import { saveFCMToken } from '../api/users';
import { getAppVersion, getDeviceName, getPlatformTarget } from "../platform/appMetadata";
import pushAdapter from "../platform/pushAdapter";

export type NotificationType = 'incoming_call' | 'call_ended' | 'generic';

interface IncomingCallPayload {
  type: 'incoming_call';
  call_id: string;
  from_user: string;
  from_email: string;
  display_name: string;
}

class PushNotificationService {
  private static instance: PushNotificationService;
  private initialized: boolean = false;
  private navigationRef: React.RefObject<NavigationContainerRef<any>> | null = null;
  private currentToken: string | null = null;
  private authToken: string | null = null;
  private notificationsEnabled: boolean = true;

  private constructor() {
    this.initialize();
  }

  public static getInstance(): PushNotificationService {
    if (!PushNotificationService.instance) {
      PushNotificationService.instance = new PushNotificationService();
    }
    return PushNotificationService.instance;
  }

  /**
   * 初始化推送通知服务
   */
  private async initialize(): Promise<void> {
    try {
      if (this.notificationsEnabled && pushAdapter.isSupported()) {
        await this.requestPermission();
      }
      await this.getFCMToken();
      this.setupMessageHandlers();
      this.setupBackgroundMessageHandler();
      this.setupOnTokenRefreshListener();

      this.initialized = true;
    } catch (error) {
      console.error("[PushNotificationService] Failed to initialize:", error);
    }
  }

  /**
   * 请求通知权限
   */
  private async requestPermission(): Promise<boolean> {
    try {
      const enabled = await pushAdapter.requestPermission();
      if (!enabled) {
        console.warn("[PushNotificationService] Notification permission denied or unsupported");
      }
      return enabled;
    } catch (error) {
      console.error("[PushNotificationService] Error requesting permission:", error);
      return false;
    }
  }

  /**
   * 获取 FCM Token
   */
  private async getFCMToken(): Promise<string | null> {
    try {
      const token = await pushAdapter.getToken();
      if (token) {
        this.currentToken = token;
        return token;
      }
      this.currentToken = null;
      console.warn("[PushNotificationService] No FCM token available");
      return null;
    } catch (error) {
      console.error("[PushNotificationService] Error getting FCM token:", error);
      return null;
    }
  }

  /**
   * 设置前台消息处理器
   */
  private setupMessageHandlers(): void {
    pushAdapter.onMessage(async (remoteMessage) => {
      this.handleMessage(remoteMessage);
    });

    pushAdapter.onNotificationOpenedApp((remoteMessage) => {
      this.handleNotificationTap(remoteMessage);
    });
  }

  /**
   * 设置后台消息处理器
   */
  private setupBackgroundMessageHandler(): void {
    pushAdapter.setBackgroundMessageHandler(async (remoteMessage) => {
      this.handleMessage(remoteMessage);
    });
  }

  /**
   * 设置 Token 刷新监听器
   */
  private setupOnTokenRefreshListener(): void {
    pushAdapter.onTokenRefresh((token) => {
      void this.sendTokenToServer(token);
    });
  }

  /**
   * 处理推送消息
   */
  private handleMessage(remoteMessage: any): void {
    if (!this.notificationsEnabled) {
      return;
    }

    try {
      const data = remoteMessage.data || {};
      const notificationType = data.type as NotificationType;

      switch (notificationType) {
        case 'incoming_call':
          this.handleIncomingCall(data as IncomingCallPayload);
          break;

        case 'call_ended':
          this.handleCallEnded(data);
          break;

        default:
          console.warn("[PushNotificationService] Unknown notification type:", notificationType);
      }
    } catch (error) {
      console.error("[PushNotificationService] Error handling message:", error);
    }
  }

  /**
   * 处理来电通知
   */
  private handleIncomingCall(_payload: IncomingCallPayload): void {
    if (!this.notificationsEnabled) {
      return;
    }

    try {
      if (!this.navigationRef?.current) {
        console.warn("[PushNotificationService] Navigation ref not ready; rely on signaling state");
      }
      this.triggerCallNotification();
    } catch (error) {
      console.error("[PushNotificationService] Error handling incoming call:", error);
    }
  }

  /**
   * 处理通话结束通知
   */
  private handleCallEnded(data: any): void {
    void data;
  }

  /**
   * 处理通知点击
   */
  private handleNotificationTap(remoteMessage: any): void {
    if (!this.notificationsEnabled) {
      return;
    }

    const data = remoteMessage.data || {};
    const notificationType = data.type as NotificationType;

    if (notificationType === 'incoming_call') {
      console.warn("[PushNotificationService] incoming_call tap received; waiting signaling state");
    }
  }

  /**
   * 触发来电通知（音频 + 震动）
   */
  private triggerCallNotification(): void {
    if (!this.notificationsEnabled) {
      return;
    }

    // 动态导入以避免循环依赖
    import('./AudioServiceExpo').then(({ default: AudioService }) => {
      AudioService.play("incoming_call");
    });

    import('./VibrationService').then(({ default: VibrationService }) => {
      VibrationService.vibrate("incoming_call");
    });
  }

  /**
   * 将 FCM Token 发送到服务器
   */
  private async sendTokenToServer(token: string): Promise<void> {
    try {
      this.currentToken = token;
      if (!this.authToken) {
        return;
      }
      await saveFCMToken(this.authToken, token, {
        provider: pushAdapter.getProvider(),
        platform: getPlatformTarget(),
        device_name: getDeviceName(),
        app_version: getAppVersion(),
      });
    } catch (error) {
      console.error("[PushNotificationService] Error syncing token to backend:", error);
    }
  }

  /**
   * 在用户登录后发送 FCM Token 到后端
   * 应该在认证成功后调用此方法
   */
  public async sendCurrentTokenToBackend(authToken: string): Promise<void> {
    this.authToken = authToken;
    if (!this.currentToken) {
      await this.getFCMToken();
    }
    if (!this.currentToken) {
      console.warn("[PushNotificationService] No FCM token available");
      return;
    }

    await saveFCMToken(this.authToken, this.currentToken, {
      provider: pushAdapter.getProvider(),
      platform: getPlatformTarget(),
      device_name: getDeviceName(),
      app_version: getAppVersion(),
    });
  }

  public setAuthToken(authToken: string | null): void {
    this.authToken = authToken;
  }

  public setNotificationsEnabled(enabled: boolean): void {
    this.notificationsEnabled = enabled;
    if (enabled) {
      void this.requestPermission();
    }
  }

  public areNotificationsEnabled(): boolean {
    return this.notificationsEnabled;
  }

  /**
   * 设置导航引用
   */
  public setNavigationRef(navigationRef: React.RefObject<NavigationContainerRef<any>>): void {
    this.navigationRef = navigationRef;
  }

  /**
   * 获取当前 FCM Token
   */
  public async getCurrentToken(): Promise<string | null> {
    return await this.getFCMToken();
  }

  /**
   * 取消注册 FCM Token
   */
  public async unregisterToken(): Promise<void> {
    try {
      await pushAdapter.unregister();
      this.currentToken = null;
    } catch (error) {
      console.error("[PushNotificationService] Error unregistering token:", error);
    }
  }

  /**
   * 检查通知权限状态
   */
  public async checkPermission(): Promise<boolean> {
    try {
      return pushAdapter.hasPermission();
    } catch (error) {
      console.error("[PushNotificationService] Error checking permission:", error);
      return false;
    }
  }

  /**
   * 获取平台信息
   */
  public getPlatformInfo(): { platform: string; version: string } {
    return {
      platform: getPlatformTarget(),
      version: Platform.Version.toString()
    };
  }

  /**
   * 清理资源
   */
  public dispose(): void {
    this.navigationRef = null;
  }
}

// 导出单例实例
export default PushNotificationService.getInstance();
