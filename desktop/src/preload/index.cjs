const { contextBridge } = require("electron");

contextBridge.exposeInMainWorld("allcallallDesktop", {
  shell: "electron",
  platform: process.platform,
  downloadsManaged: true,
});
