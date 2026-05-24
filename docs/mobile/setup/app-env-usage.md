# 移动端环境配置

AllCallAll 移动端现在只使用 Expo 的 `EXPO_PUBLIC_*` 变量控制接口地址和运行开关，不再使用 `APP_ENV`。

## 默认行为

- 未设置任何变量时，默认连接 `http://127.0.0.1:8080`
- WebSocket 默认连接 `ws://127.0.0.1:8080`
- Android 真机联调通常需要 `adb reverse tcp:8080 tcp:8080`

## 支持的变量

```bash
EXPO_PUBLIC_API_HTTP=http://10.0.2.2:8080
EXPO_PUBLIC_API_WS=ws://10.0.2.2:8080
EXPO_PUBLIC_FORCE_TLS=0
EXPO_PUBLIC_RESTRICTED_NETWORK=0
EXPO_PUBLIC_SIGNALING_TRANSPORT=auto
EXPO_PUBLIC_SIGNALING_SHAPING=0
EXPO_PUBLIC_TRANSLATION_SOURCE_LANG=zh
EXPO_PUBLIC_TRANSLATION_TARGET_LANG=en
```

## 常见场景

本地默认:

```bash
cd mobile
npm start
```

指定局域网后端:

```bash
cd mobile
EXPO_PUBLIC_API_HTTP=http://192.168.1.20:8080 \
EXPO_PUBLIC_API_WS=ws://192.168.1.20:8080 \
npm start
```

强制 HTTPS/WSS:

```bash
cd mobile
EXPO_PUBLIC_API_HTTP=http://api.example.com \
EXPO_PUBLIC_API_WS=ws://api.example.com \
EXPO_PUBLIC_FORCE_TLS=1 \
npm start
```

受限网络下优先轮询:

```bash
cd mobile
EXPO_PUBLIC_SIGNALING_TRANSPORT=poll \
EXPO_PUBLIC_RESTRICTED_NETWORK=1 \
npm start
```

## 验证

```bash
cd mobile
bash scripts/verify-app-env.sh
```
