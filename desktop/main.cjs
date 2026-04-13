const { app, BrowserWindow, Menu, Notification, shell } = require("electron");
const fs = require("fs");
const os = require("os");
const path = require("path");

const DEV_SERVER_URL = process.env.ALLCALLALL_WEB_URL || "http://localhost:8081";
const DOWNLOADS_DIR = process.env.ALLCALLALL_DOWNLOAD_DIR || path.join(os.homedir(), "Downloads", "AllCallAll");

let mainWindow = null;

function ensureDownloadsDir() {
  fs.mkdirSync(DOWNLOADS_DIR, { recursive: true });
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

  mainWindow.loadURL(DEV_SERVER_URL);

  mainWindow.webContents.setWindowOpenHandler(({ url }) => {
    void shell.openExternal(url);
    return { action: "deny" };
  });

  mainWindow.webContents.on("will-navigate", (event, url) => {
    if (!url.startsWith(DEV_SERVER_URL)) {
      event.preventDefault();
      void shell.openExternal(url);
    }
  });

  mainWindow.webContents.session.on("will-download", (_event, item) => {
    ensureDownloadsDir();
    const downloadPath = path.join(DOWNLOADS_DIR, item.getFilename());
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
              void mainWindow.loadURL(`${DEV_SERVER_URL}/meetings`);
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
    const target = commandLine.find((value) => value.startsWith("allcallall://") || value.includes("/rooms/"));
    if (target && mainWindow) {
      const normalized = target.startsWith("allcallall://rooms/")
        ? `${DEV_SERVER_URL}/rooms/${target.replace("allcallall://rooms/", "")}`
        : target;
      void mainWindow.loadURL(normalized);
    }
  });
}

app.whenReady().then(() => {
  buildMenu();
  createWindow();

  app.on("activate", () => {
    if (BrowserWindow.getAllWindows().length === 0) {
      createWindow();
    }
  });
});

app.on("window-all-closed", () => {
  if (process.platform !== "darwin") {
    app.quit();
  }
});
