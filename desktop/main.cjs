const { app, BrowserWindow, dialog, Menu, Notification, session, shell } = require("electron");
const fs = require("fs");
const os = require("os");
const path = require("path");

const BASE_WEB_URL = process.env.ALLCALLALL_WEB_URL || "http://localhost:5173";

// A packaged app that was built without ALLCALLALL_WEB_URL silently points at
// the dev server on localhost and shows an empty white window, with nothing
// in the UI explaining why. Say so at startup so the cause is visible.
if (app.isPackaged && (BASE_WEB_URL.includes("localhost") || BASE_WEB_URL.includes("127.0.0.1"))) {
  console.error(
    "[AllCallAll] ALLCALLALL_WEB_URL is not set or still points at localhost (" +
      BASE_WEB_URL + "). The packaged app will not be able to load. Set it to the " +
      "production web origin at build time."
  );
}

const { createRouteHelpers } = require("./route-utils.cjs");

const {
  isInternalWebURL,
  normalizeRouteTarget,
  routeURL,
} = createRouteHelpers(BASE_WEB_URL);

// Vite 开发服务器会向页面注入 inline 脚本，因此只有在目标是本机开发服务器时才放宽
// script-src；一旦指向生产 Web 资源即自动收紧，避免把 dev 期的妥协带进线上。
function isLoopbackTarget(target) {
  try {
    const { hostname } = new URL(target);
    return hostname === "localhost" || hostname === "127.0.0.1" || hostname === "::1";
  } catch {
    return false;
  }
}

// 给所有渲染进程响应加上 CSP 头（默认同源，禁止 unsafe-eval/外部源）。
const DESKTOP_CSP = [
  "default-src 'self'",
  isLoopbackTarget(BASE_WEB_URL) ? "script-src 'self' 'unsafe-inline'" : "script-src 'self'",
  "style-src 'self' 'unsafe-inline'",
  "img-src 'self' data: blob:",
  "font-src 'self' data:",
  // Kept as a scheme wildcard on purpose: a desktop build can be pointed at a
  // self-hosted backend, so the API origin is not known at build time. The web
  // app's own CSP (infra/nginx.tls.conf) enumerates origins because there the
  // backend is known. Narrowing this one requires knowing every origin a
  // deployment uses, which is not something to guess at.
  "connect-src 'self' https: wss:",
  "media-src 'self' blob:",
  // Directives the nginx CSP already had and this one was missing. They cost
  // nothing: nothing in the app loads plugins, workers or a manifest, and
  // without base-uri an injected <base> tag can redirect every relative URL.
  "object-src 'none'",
  "base-uri 'self'",
  "worker-src 'self'",
  "manifest-src 'self'",
  "frame-ancestors 'none'",
].join("; ");

session.defaultSession.webRequest.onHeadersReceived((details, callback) => {
  callback({
    responseHeaders: {
      ...details.responseHeaders,
      "Content-Security-Policy": [DESKTOP_CSP],
    },
  });
});

const DOWNLOADS_DIR = process.env.ALLCALLALL_DOWNLOAD_DIR || path.join(os.homedir(), "Downloads", "AllCallAll");
const ALLOWED_EXTERNAL_PROTOCOLS = new Set(["http:", "https:", "mailto:"]);

// 会议是桌面端的一等公民（默认窗口就加载 /meetings），麦克风/摄像头必须可用。
// Electron/Chromium 对音视频设备的 permission 值主要是 "media"，个别路径会落到
// "microphone"/"camera"，一并接受；屏幕捕获（display-capture）暂不放行——它属于
// 更高风险面，确认产品需要时按同样范式追加到该集合即可。
const ALLOWED_MEDIA_PERMISSIONS = new Set(["media", "microphone", "camera"]);

// 双重判定：请求来源必须是本应用已信任的 Web 资源，且权限类型在媒体白名单内。
function isTrustedMediaRequest(requestingURL, permission) {
  if (!ALLOWED_MEDIA_PERMISSIONS.has(permission)) {
    return false;
  }
  return typeof requestingURL === "string" && isInternalWebURL(requestingURL);
}

let mainWindow = null;
let pendingRouteTarget = null;

function ensureDownloadsDir() {
  fs.mkdirSync(DOWNLOADS_DIR, { recursive: true });
}

function openExternalURL(target) {
  try {
    const parsed = new URL(target);
    if (!ALLOWED_EXTERNAL_PROTOCOLS.has(parsed.protocol)) {
      return;
    }
    void shell.openExternal(target);
  } catch {
    // Ignore malformed external targets from untrusted pages.
  }
}

function openRouteTarget(target) {
  const normalized = normalizeRouteTarget(target);
  if (!normalized || !mainWindow) {
    return false;
  }
  void mainWindow.loadURL(normalized);
  return true;
}

function createWindow() {
  mainWindow = new BrowserWindow({
    width: 1440,
    height: 920,
    minWidth: 1080,
    minHeight: 720,
    autoHideMenuBar: true,
    webPreferences: {
      preload: path.join(__dirname, "preload.cjs"),
      contextIsolation: true,
      nodeIntegration: false,
    },
  });

  // Loading is the one failure path a React error boundary cannot catch: if
  // the web origin is unreachable the window is simply blank. Retry with a
  // short backoff first (covers a server that is still starting up), then ask
  // the user, because a white window with no explanation is unrecoverable
  // from the user's side.
  let loadAttempt = 0;
  const loadApp = () => {
    loadAttempt += 1;
    mainWindow.loadURL(routeURL("/meetings")).catch((error) => {
      console.error("[AllCallAll] loadURL failed:", error);
    });
  };
  loadApp();

  mainWindow.webContents.on("did-fail-load", (_event, errorCode, errorDescription, _url, isMainFrame) => {
    if (!isMainFrame) {
      return;
    }
    // ERR_ABORTED (-3) happens on normal navigation away; not a failure.
    if (errorCode === -3) {
      return;
    }
    if (loadAttempt <= 3) {
      const delay = loadAttempt * 1000;
      console.warn(`[AllCallAll] load failed (${errorDescription}); retrying in ${delay}ms`);
      setTimeout(loadApp, delay);
      return;
    }
    dialog
      .showMessageBox(mainWindow, {
        type: "error",
        title: "无法加载 AllCallAll",
        message: `无法连接到 ${BASE_WEB_URL}`,
        detail: `${errorDescription}（错误码 ${errorCode}）\n\n请确认 Web 服务可访问；若这是自建部署，检查 ALLCALLALL_WEB_URL 配置。`,
        buttons: ["重试", "退出"],
        defaultId: 0,
        cancelId: 1,
      })
      .then(({ response }) => {
        if (response === 0) {
          loadAttempt = 0;
          loadApp();
          return;
        }
        app.quit();
      })
      .catch(() => {});
  });

  // 按"请求来源 + 权限类型"白名单放行：只有来自本应用 Web 资源的音视频请求才通过，
  // 其余（定位、通知、剪贴板读取、屏幕捕获等）一律拒绝。
  mainWindow.webContents.session.setPermissionRequestHandler((webContents, permission, callback, details) => {
    const requestingURL =
      details && typeof details.requestingUrl === "string" && details.requestingUrl
        ? details.requestingUrl
        : webContents.getURL();
    callback(isTrustedMediaRequest(requestingURL, permission));
  });

  mainWindow.webContents.session.setPermissionCheckHandler((webContents, permission, origin) => {
    return isTrustedMediaRequest(origin || webContents.getURL(), permission);
  });

  mainWindow.webContents.setWindowOpenHandler(({ url }) => {
    if (!openRouteTarget(url)) {
      openExternalURL(url);
    }
    return { action: "deny" };
  });

  mainWindow.webContents.on("will-navigate", (event, url) => {
    if (isInternalWebURL(url)) {
      return;
    }
    event.preventDefault();
    if (!openRouteTarget(url)) {
      openExternalURL(url);
    }
  });

  mainWindow.webContents.session.on("will-download", (_event, item) => {
    ensureDownloadsDir();
    const downloadPath = path.join(DOWNLOADS_DIR, path.basename(item.getFilename()));
    item.setSavePath(downloadPath);
    item.once("done", (_doneEvent, state) => {
      if (state === "completed") {
        new Notification({
          title: "AllCallAll",
          body: `下载完成：${item.getFilename()}`,
        }).show();
      }
    });
  });

  mainWindow.on("closed", () => {
    mainWindow = null;
  });
}

function focusMainWindow() {
  if (!mainWindow) {
    createWindow();
    return;
  }
  if (mainWindow.isMinimized()) {
    mainWindow.restore();
  }
  mainWindow.focus();
}

function buildMenu() {
  const template = [
    {
      label: "AllCallAll",
      submenu: [
        { role: "about" },
        {
          label: "Open Meetings",
          click: () => {
            focusMainWindow();
            if (mainWindow) {
              void mainWindow.loadURL(routeURL("/meetings"));
            }
          },
        },
        {
          label: "Open Downloads Folder",
          click: () => {
            ensureDownloadsDir();
            void shell.openPath(DOWNLOADS_DIR);
          },
        },
        { type: "separator" },
        { role: "quit" },
      ],
    },
    {
      label: "Window",
      submenu: [{ role: "reload" }, { role: "toggledevtools" }, { role: "minimize" }, { role: "close" }],
    },
  ];
  Menu.setApplicationMenu(Menu.buildFromTemplate(template));
}

const gotSingleInstanceLock = app.requestSingleInstanceLock();

if (!gotSingleInstanceLock) {
  app.quit();
} else {
  app.on("second-instance", (_event, commandLine) => {
    focusMainWindow();
    const target = commandLine.find((value) => normalizeRouteTarget(value));
    if (target) {
      openRouteTarget(target);
    }
  });
}

app.whenReady().then(() => {
  app.setAppUserModelId("com.allcallall.desktop");
  app.setAsDefaultProtocolClient("allcallall");
  buildMenu();
  createWindow();
  if (pendingRouteTarget) {
    openRouteTarget(pendingRouteTarget);
    pendingRouteTarget = null;
  }

  app.on("activate", () => {
    if (BrowserWindow.getAllWindows().length === 0) {
      createWindow();
    }
  });
});

app.on("open-url", (event, url) => {
  event.preventDefault();
  if (!app.isReady()) {
    pendingRouteTarget = url;
    return;
  }
  focusMainWindow();
  openRouteTarget(url);
});

app.on("window-all-closed", () => {
  if (process.platform !== "darwin") {
    app.quit();
  }
});
