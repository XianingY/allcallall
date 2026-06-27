const { app, BrowserWindow, Menu, Notification, shell } = require("electron");
const fs = require("fs");
const os = require("os");
const path = require("path");

const { createRouteHelpers } = require("./route-utils.cjs");

const {
  isInternalWebURL,
  normalizeRouteTarget,
  routeURL,
} = createRouteHelpers(process.env.ALLCALLALL_WEB_URL || "http://localhost:5173");
const DOWNLOADS_DIR = process.env.ALLCALLALL_DOWNLOAD_DIR || path.join(os.homedir(), "Downloads", "AllCallAll");
const ALLOWED_EXTERNAL_PROTOCOLS = new Set(["http:", "https:", "mailto:"]);

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

  mainWindow.loadURL(routeURL("/meetings"));

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
