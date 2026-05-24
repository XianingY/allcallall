const { contextBridge } = require("electron");

contextBridge.exposeInMainWorld("allcallallDesktop", {
  shell: "electron",
});
