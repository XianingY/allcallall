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

import messaging from '@react-native-firebase/messaging';
import { Platform } from 'react-native';
import { NavigationContainerRef } from '@react-navigation/native';

export type NotificationType = 'incoming_call' | 'call_ended' | 'generic';

interface IncomingCallPayload {
  type: 'incoming_call';
  call_id: string;
  from_user: string;
  from_email: string;
  display_name: string;
}

interface NotificationPayload {
  type: NotificationType;
  [key: string]: any;
}

class PushNotificationService {
  private static instance: PushNotificationService;
  private initialized: boolean = false;
  private navigationRef: React.RefObject<NavigationContainerRef> | null = null;

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
    console.log("[PushNotificationService] Initializing...");

    try {
      // 请求通知权限
      await this.requestPermission();

      // 获取 FCM Token
      await this.getFCMToken();

      // 设置消息监听器
      this.setupMessageHandlers();

      // 设置后台消息处理器
      this.setupBackgroundMessageHandler();

      // 设置退出监听器
      this.setupOnTokenRefreshListener();

      this.initialized = true;
      console.log("[PushNotificationService] ✓ Initialized successfully");
    } catch (error) {
      console.error("[PushNotificationService] Failed to initialize:", error);
    }
  }

  /**
   * 请求通知权限
   */
  private async requestPermission(): Promise<boolean> {
    console.log("[PushNotificationService] Requesting notification permission...");

    try {
      const authStatus = await messaging().requestPermission();
      const enabled =
        authStatus === messaging.AuthorizationStatus.AUTHORIZED ||
        authStatus === messaging.AuthorizationStatus.PROVISIONAL;

      if (enabled) {
        console.log("[PushNotificationService] ✓ Notification permission granted");
        return true;
      } else {
        console.warn("[PushNotificationService] ✗ Notification permission denied");
        return false;
      }
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
      const token = await messaging().getToken();
      if (token) {
        console.log("[PushNotificationService] ✓ FCM Token:", token.substring(0, 20) + "...");
        return token;
      } else {
        console.warn("[PushNotificationService] No FCM token available");
        return null;
      }
    } catch (error) {
      console.error("[PushNotificationService] Error getting FCM token:", error);
      return null;
    }
  }

  /**
   * 设置前台消息处理器
   */
  private setupMessageHandlers(): void {
    // 前台收到消息
    messaging().onMessage(async (remoteMessage) => {
      console.log("[PushNotificationService] Foreground message received:", remoteMessage);
      this.handleMessage(remoteMessage);
    });

    // 通知点击事件
    messaging().onNotificationOpenedApp((remoteMessage) => {
      console.log("[PushNotificationService] Notification opened app:", remoteMessage);
      this.handleNotificationTap(remoteMessage);
    });
  }

  /**
   * 设置后台消息处理器
   */
  private setupBackgroundMessageHandler(): void {
    // 应用在后台时收到消息
    messaging().setBackgroundMessageHandler(async (remoteMessage) => {
      console.log("[PushNotificationService] Background message received:", remoteMessage);
      this.handleMessage(remoteMessage);
    });
  }

  /**
   * 设置 Token 刷新监听器
   */
  private setupOnTokenRefreshListener(): void {
    messaging().onTokenRefresh((token) => {
      console.log("[PushNotificationService] FCM token refreshed");
      // TODO: 将新 token 发送到后端服务器
      this.sendTokenToServer(token);
    });
  }

  /**
   * 处理推送消息
   */
  private handleMessage(remoteMessage: any): void {
    try {
      const data = remoteMessage.data || {};
      const notificationType = data.type as NotificationType;

      console.log(`[PushNotificationService] Handling message type: ${notificationType}`);

      switch (notificationType) {
        case 'incoming_call':
          this.handleIncomingCall(data as IncomingCallPayload);
          break;

        case 'call_ended':
          this.handleCallEnded(data);
          break;

        default:
          console.log("[PushNotificationService] Unknown notification type:", notificationType);
      }
    } catch (error) {
      console.error("[PushNotificationService] Error handling message:", error);
    }
  }

  /**
   * 处理来电通知
   */
  private handleIncomingCall(payload: IncomingCallPayload): void {
    console.log(`[PushNotificationService] Incoming call from: ${payload.from_user}`);

    try {
      // 导航到来电页面（即使应用在后台）
      if (this.navigationRef?.current) {
        this.navigationRef.current.navigate('IncomingCall', {
          fromUser: payload.from_user,
          fromEmail: payload.from_email,
          displayName: payload.display_name,
          callId: payload.call_id,
          isFromPush: true
        });
      }

      // 触发音频和震动
      this.triggerCallNotification();
    } catch (error) {
      console.error("[PushNotificationService] Error handling incoming call:", error);
    }
  }

  /**
   * 处理通话结束通知
   */
  private handleCallEnded(data: any): void {
    console.log("[PushNotificationService] Call ended");

    // 可以在这里处理通话结束逻辑，比如更新通话记录
  }

  /**
   * 处理通知点击
   */
  private handleNotificationTap(remoteMessage: any): void {
    console.log("[PushNotificationService] Notification tapped");

    const data = remoteMessage.data || {};
    const notificationType = data.type as NotificationType;

    if (notificationType === 'incoming_call') {
      // 通知被点击，导航到来电页面
      if (this.navigationRef?.current) {
        this.navigationRef.current.navigate('IncomingCall', {
          fromUser: data.from_user,
          fromEmail: data.from_email,
          displayName: data.display_name,
          callId: data.call_id,
          isFromPush: true
        });
      }
    }
  }

  /**
   * 触发来电通知（音频 + 震动）
   */
  private triggerCallNotification(): void {
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
      // TODO: 实现将 token 发送到后端 API
      console.log("[PushNotificationService] Sending token to server:", token);

      // 示例实现：
      // await apiClient.post('/users/fcm-token', { token });
    } catch (error) {
      console.error("[PushNotificationService] Error sending token to server:", error);
    }
  }

  /**
   * 设置导航引用
   */
  public setNavigationRef(navigationRef: React.RefObject<NavigationContainerRef>): void {
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
      await messaging().unregisterDeviceForRemoteMessages();
      console.log("[PushNotificationService] FCM token unregistered");
    } catch (error) {
      console.error("[PushNotificationService] Error unregistering token:", error);
    }
  }

  /**
   * 检查通知权限状态
   */
  public async checkPermission(): Promise<boolean> {
    try {
      const authStatus = await messaging().hasPermission();
      const enabled =
        authStatus === messaging.AuthorizationStatus.AUTHORIZED ||
        authStatus === messaging.AuthorizationStatus.PROVISIONAL;

      return enabled;
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
      platform: Platform.OS,
      version: Platform.Version.toString()
    };
  }

  /**
   * 清理资源
   */
  public dispose(): void {
    console.log("[PushNotificationService] Disposing...");
    // 清理监听器等资源
  }
}

// 导出单例实例
export default PushNotificationService.getInstance();
