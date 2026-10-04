// Permission-gate check for the desktop shell.
//
// The allow/deny decision in main.cjs is the only thing standing between a
// loaded web origin and the device camera and microphone, and it is pure logic
// buried inside an Electron main process that cannot be exercised without a
// window. This harness loads main.cjs with a stubbed `electron` module, captures
// the two handlers it registers, and asserts the decision table.
//
// What it pins down:
//   - camera/microphone from the app's own web origin are allowed
//   - the same request from any other origin is denied
//   - every non-media permission (geolocation, notifications, clipboard-read,
//     display-capture, ...) is denied
//   - a check with no resolvable origin (service worker case, where Electron
//     passes webContents = null and an empty requestingOrigin) denies instead of
//     throwing, which is what the old `origin || webContents.getURL()` did
//
// Run with: node scripts/permission-check.cjs

const assert = require("assert");
const Module = require("module");

let handlers = {
  request: null,
  check: null,
};

const noopWebRequest = { onHeadersReceived() {} };

const electronStub = {
  app: {
    isPackaged: false,
    requestSingleInstanceLock: () => true,
    on() {},
    // Run the whenReady callback synchronously: installPermissionHandlers lives
    // in there, and a deferred then() would leave the handlers uncaptured.
    whenReady: () => ({ then: (fn) => fn() }),
    setAppUserModelId() {},
    setAsDefaultProtocolClient() {},
    quit() {},
  },
  BrowserWindow: Object.assign(
    function BrowserWindow() {
      return {
        webContents: {
          on() {},
          setWindowOpenHandler() {},
          session: { on() {} },
          getURL: () => "https://desktop.example.com/meetings",
        },
        on() {},
        loadURL: () => Promise.resolve(),
        isMinimized: () => false,
      };
    },
    { getAllWindows: () => [] }
  ),
  dialog: { showMessageBox: () => Promise.resolve({ response: 1 }) },
  Menu: { setApplicationMenu() {}, buildFromTemplate: (t) => t },
  Notification: function Notification() {
    return { show() {} };
  },
  session: {
    defaultSession: {
      webRequest: noopWebRequest,
      setPermissionRequestHandler(fn) {
        handlers.request = fn;
      },
      setPermissionCheckHandler(fn) {
        handlers.check = fn;
      },
    },
  },
  shell: { openExternal() {}, openPath() {} },
};

const originalLoad = Module._load;
Module._load = function patchedLoad(request, parent, isMain) {
  if (request === "electron") {
    return electronStub;
  }
  return originalLoad.call(this, request, parent, isMain);
};

function loadHandlers(entrypoint) {
  handlers = { request: null, check: null };
  for (const modulePath of ["../main.cjs", "../src/main/index.cjs"]) {
    try {
      delete require.cache[require.resolve(modulePath)];
    } catch {
      // The canonical path intentionally does not exist before the extraction.
    }
  }
  require(entrypoint);
  assert.ok(handlers.request, `setPermissionRequestHandler was never registered by ${entrypoint}`);
  assert.ok(handlers.check, `setPermissionCheckHandler was never registered by ${entrypoint}`);
  return { ...handlers };
}

process.env.ALLCALLALL_WEB_URL = "https://desktop.example.com";

const INTERNAL = "https://desktop.example.com/meetings";
const EXTERNAL = "https://evil.example.com/meetings";
const fakeWebContents = { getURL: () => INTERNAL };

function request(activeHandlers, permission, details, webContents) {
  let decision;
  activeHandlers.request(
    webContents === undefined ? fakeWebContents : webContents,
    permission,
    (allowed) => {
      decision = allowed;
    },
    details
  );
  return decision;
}

function check(activeHandlers, permission, requestingOrigin, details, webContents) {
  return activeHandlers.check(
    webContents === undefined ? fakeWebContents : webContents,
    permission,
    requestingOrigin,
    details
  );
}

function checkPermissionHandlers(entrypoint) {
  const activeHandlers = loadHandlers(entrypoint);

  // Allowed: camera and microphone, from the app's own origin only.
  for (const permission of ["media", "microphone", "camera"]) {
    assert.strictEqual(request(activeHandlers, permission, { requestingUrl: INTERNAL, securityOrigin: INTERNAL }), true, `${permission} from the app origin should be allowed`);
    assert.strictEqual(check(activeHandlers, permission, INTERNAL, { securityOrigin: INTERNAL }), true, `${permission} check from the app origin should pass`);
    assert.strictEqual(request(activeHandlers, permission, { requestingUrl: EXTERNAL }), false, `${permission} from a foreign origin must be denied`);
    assert.strictEqual(check(activeHandlers, permission, EXTERNAL), false, `${permission} check from a foreign origin must fail`);
  }

// Denied: everything outside the media allowlist, including screen capture,
// which Electron now reports as its own `display-capture` permission.
  for (const permission of [
    "display-capture",
    "geolocation",
    "geolocation-approximate",
    "notifications",
    "clipboard-read",
    "clipboard-sanitized-write",
    "pointerLock",
    "hid",
    "usb",
    "serial",
    "persistent-storage",
    "local-network-access",
    "openExternal",
    "unknown",
  ]) {
    assert.strictEqual(request(activeHandlers, permission, { requestingUrl: INTERNAL }), false, `${permission} must be denied`);
    assert.strictEqual(check(activeHandlers, permission, INTERNAL), false, `${permission} check must fail`);
  }

// No resolvable origin (service worker checks arrive with webContents = null
// and an empty requestingOrigin): deny, and do not throw on the null.
  assert.strictEqual(check(activeHandlers, "media", "", undefined, null), false, "a check with no origin must fail closed");
  assert.strictEqual(request(activeHandlers, "media", undefined, null), false, "a request with no origin must fail closed");
  assert.strictEqual(check(activeHandlers, "media", undefined, undefined, null), false, "a check with neither origin nor webContents must fail closed");
}

try {
  checkPermissionHandlers("../main.cjs");
  checkPermissionHandlers("../src/main/index.cjs");
} finally {
  Module._load = originalLoad;
}

console.log("[desktop-permission-check] passed");
