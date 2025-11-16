# AllCallAll Mobile App (Android Focus)

React Native (Expo) prototype that pairs with the Go backend to deliver email-first calling.

## Features

- Email/password registration & login (JWT stored securely via AsyncStorage).
- Contacts management: add/remove by email, list with real-time presence snapshot.
- WebSocket signaling client built on backend protocol (`call.invite`, `call.accept`, `ice.candidate`, `call.end`).
- Call overlay UI for outgoing/incoming calls and simple ICE test messages.
- Presence polling every 10 seconds to surface online users.

## Project Structure

```
mobile/
├── App.tsx                  # Providers + navigation entry point
├── app.json                 # Expo configuration
├── package.json             # Dependencies & scripts
├── tsconfig.json / babel.config.js
└── src/
    ├── api/                 # REST + signaling clients
    ├── components/          # Reusable UI elements (buttons, overlays, badges)
    ├── config/              # Runtime configuration (API/WS hosts)
    ├── context/             # Auth & Signaling providers
    ├── navigation/          # React Navigation stacks
    └── screens/             # Login, Register, Contacts (main screen)
```

## Prerequisites

- Node.js 18+
- Yarn or npm
- Expo CLI (`npm install -g expo-cli`) *optional, `npx expo` works too*
- Android Studio / emulator or physical Android device（真机需授予麦克风、摄像头及蓝牙权限，用于音频采集与耳机连接）
- Backend running (see `infra/docker-compose.yml`)
- Microphone permission enabled on device/emulator (通话前务必确认授予)

## Getting Started

```bash
cd mobile
npm install            # or yarn install (确保安装 react-native-webrtc)
npm run start          # starts Metro bundler
```

### Android emulator

```bash
npm run android
```

### Physical device (Expo Go)

1. Ensure phone & dev machine on same network.
2. Run `npm run start` to display the Expo QR code.
3. Scan code with Expo Go app to load the project.

## Environment Notes

- The app auto-selects `http://10.0.2.2:8080` for Android emulators and `http://localhost:8080` for iOS/Web.
- If backend runs on a different host or via HTTPS, update `src/config/index.ts` accordingly.
- WebSocket auth relies on custom headers (supported on React Native Android/iOS 0.74+). Expo Go also supports this.

## Manual Testing Flow

1. Launch backend via `docker compose up -d --build`.
2. Install & run the app (`npm install`, `npm run android`).
3. Register or log in with an email (e.g., `alice@example.com`).
4. Add another account (e.g., `bob@example.com`) from Contacts screen and log in on a second device/emulator.
5. Observe presence updates (green dot for online users).
6. Tap “呼叫 / Call” on a contact启动呼叫：系统会请求麦克风权限，随后开始采集音频。
7. 被叫端点击“接受”，双方通过 WebRTC 自动交换 SDP/ICE，音频会在后台播放（`RTCView`隐藏渲染）。
8. “结束 / End” 按钮会关闭通话并释放音频资源。

## Next Steps / Enhancements

- 视频通话：在现有音频基础上追加摄像头采集与渲染。
- Persist signaling events + call logs.
- Implement push notifications for incoming calls when app backgrounded.
- Add localization framework (currently inline Chinese/English text).
