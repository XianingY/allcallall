import { chromium } from "playwright";
import fs from "node:fs";
import http from "node:http";
import path from "node:path";
import { fileURLToPath } from "node:url";

let baseURL = (process.env.WEB_SMOKE_BASE_URL || "http://127.0.0.1:8081").replace(/\/+$/, "");
const configuredExportDir = process.env.WEB_SMOKE_EXPORT_DIR;
const exportDir = configuredExportDir || "dist";
const email = process.env.WEB_SMOKE_EMAIL;
const password = process.env.WEB_SMOKE_PASSWORD;
const roomId = process.env.WEB_SMOKE_ROOM_ID;
const conversationId = process.env.WEB_SMOKE_CONVERSATION_ID;
const shouldJoinMeeting = process.env.WEB_SMOKE_JOIN_MEETING === "1";
const shouldDownloadRecording = process.env.WEB_SMOKE_DOWNLOAD_RECORDING === "1";

const contentTypes = new Map([
  [".html", "text/html; charset=utf-8"],
  [".js", "application/javascript; charset=utf-8"],
  [".css", "text/css; charset=utf-8"],
  [".json", "application/json; charset=utf-8"],
  [".png", "image/png"],
  [".jpg", "image/jpeg"],
  [".jpeg", "image/jpeg"],
  [".mp3", "audio/mpeg"],
]);

const startStaticExportServer = async () => {
  if (process.env.WEB_SMOKE_BASE_URL) {
    return null;
  }
  const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..", exportDir);
  const rootWithSeparator = `${root}${path.sep}`;
  if (!fs.existsSync(path.join(root, "index.html"))) {
    if (!configuredExportDir) {
      return null;
    }
    throw new Error(`WEB_SMOKE_EXPORT_DIR does not contain index.html: ${root}`);
  }
  const server = http.createServer((request, response) => {
    const pathname = decodeURIComponent(new URL(request.url || "/", "http://127.0.0.1").pathname);
    const requestedPath = path.normalize(path.join(root, pathname));
    const safePath = requestedPath === root || requestedPath.startsWith(rootWithSeparator)
      ? requestedPath
      : path.join(root, "index.html");
    const filePath = fs.existsSync(safePath) && fs.statSync(safePath).isFile()
      ? safePath
      : path.join(root, "index.html");
    response.setHeader("Content-Type", contentTypes.get(path.extname(filePath)) || "application/octet-stream");
    fs.createReadStream(filePath).pipe(response);
  });
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  const address = server.address();
  if (!address || typeof address === "string") {
    throw new Error("failed to start static web smoke server");
  }
  baseURL = `http://127.0.0.1:${address.port}`;
  return server;
};

const route = (path) => `${baseURL}${path.startsWith("/") ? path : `/${path}`}`;

const waitForUsablePage = async (page, label) => {
  await page.waitForLoadState("domcontentloaded", { timeout: 15_000 });
  await page.waitForTimeout(500);
  const text = (await page.locator("body").innerText({ timeout: 10_000 })).trim();
  if (text.length < 8) {
    throw new Error(`${label} rendered an empty page`);
  }
  return text;
};

const visit = async (page, path, label) => {
  await page.goto(route(path), { waitUntil: "domcontentloaded", timeout: 20_000 });
  return waitForUsablePage(page, label);
};

const assertContainsAny = (text, label, candidates) => {
  if (!candidates.some((candidate) => text.includes(candidate))) {
    throw new Error(`${label} did not contain any expected text: ${candidates.join(", ")}`);
  }
};

const clickFirstVisibleText = async (page, labels, timeout = 8_000) => {
  const deadline = Date.now() + timeout;
  while (Date.now() < deadline) {
    for (const label of labels) {
      const locator = page.getByText(label, { exact: false }).first();
      if (await locator.isVisible().catch(() => false)) {
        await locator.click();
        return true;
      }
    }
    await page.waitForTimeout(250);
  }
  return false;
};

const loginIfConfigured = async (page) => {
  if (!email || !password) {
    console.log("[web-smoke] WEB_SMOKE_EMAIL/PASSWORD not set; running anonymous route smoke only.");
    return false;
  }

  const text = await visit(page, "/meetings", "login route");
  if (!text.includes("登录") && !text.includes("Login")) {
    return true;
  }

  const inputs = page.locator("input");
  await inputs.nth(0).fill(email);
  await inputs.nth(1).fill(password);
  await page.getByText(/登录 \/ Login|Login/).click();
  await page.waitForTimeout(1200);
  const afterLogin = await waitForUsablePage(page, "post-login page");
  assertContainsAny(afterLogin, "post-login page", ["Agent Lab", "Meetings", "会议", "Inbox"]);
  return true;
};

const main = async () => {
  const staticServer = await startStaticExportServer();
  const browser = await chromium.launch({
    headless: true,
    args: [
      "--use-fake-ui-for-media-stream",
      "--use-fake-device-for-media-stream",
    ],
  });
  const context = await browser.newContext({
    viewport: { width: 1440, height: 960 },
    acceptDownloads: true,
  });
  const page = await context.newPage();
  const pageErrors = [];
  page.on("pageerror", (error) => pageErrors.push(error.message));

  try {
    const loggedIn = await loginIfConfigured(page);

    const agentDemoText = await visit(page, "/agent-demo", "Agent demo route");
    assertContainsAny(agentDemoText, "Agent demo route", loggedIn ? ["Agent Lab", "Knowledge", "Approvals"] : ["AllCallAll", "Login", "登录"]);

    const meetingsText = await visit(page, "/meetings", "Meetings");
    assertContainsAny(meetingsText, "Meetings", loggedIn ? ["Meetings", "Quick actions", "会议"] : ["AllCallAll", "Login", "登录"]);

    const roomPath = roomId ? `/rooms/${roomId}` : "/rooms/1";
    const roomText = await visit(page, roomPath, "room route");
    assertContainsAny(roomText, "room route", loggedIn ? ["会议", "Join", "加入"] : ["AllCallAll", "Login", "登录"]);
    if (loggedIn && shouldJoinMeeting) {
      const clickedJoin = await clickFirstVisibleText(page, ["加入会议", "Join Meeting", "Join"]);
      if (!clickedJoin) {
        throw new Error("room route did not expose a join action");
      }
      const meetingText = await waitForUsablePage(page, "meeting page");
      assertContainsAny(meetingText, "meeting page", ["离开会议", "Leave", "Connected", "会议状态"]);
      const clickedLeave = await clickFirstVisibleText(page, ["离开会议", "Leave"]);
      if (!clickedLeave) {
        throw new Error("meeting page did not expose a leave action");
      }
      const afterLeaveText = await waitForUsablePage(page, "post-leave page");
      assertContainsAny(afterLeaveText, "post-leave page", ["协作线程", "Meetings", "会议", "Inbox"]);
    }

    const conversationPath = conversationId ? `/conversations/${conversationId}` : "/conversations/1";
    const conversationText = await visit(page, conversationPath, "conversation route");
    assertContainsAny(conversationText, "conversation route", loggedIn ? ["协作线程", "Inbox", "Conversation"] : ["AllCallAll", "Login", "登录"]);

    const sessionsText = await visit(page, "/sessions", "sessions route");
    assertContainsAny(sessionsText, "sessions route", loggedIn ? ["登录会话", "Active Sessions", "Refresh"] : ["AllCallAll", "Login", "登录"]);

    if (loggedIn && shouldDownloadRecording) {
      const recordingsText = await visit(page, "/recordings", "recordings route");
      assertContainsAny(recordingsText, "recordings route", ["录音存档", "Recordings", "下载"]);
      const downloadPromise = page.waitForEvent("download", { timeout: 15_000 });
      const clickedDownload = await clickFirstVisibleText(page, ["下载", "Download"]);
      if (!clickedDownload) {
        throw new Error("recordings route did not expose a download action");
      }
      const download = await downloadPromise;
      const suggestedName = download.suggestedFilename();
      if (!suggestedName) {
        throw new Error("recording download did not provide a filename");
      }
    }

    if (pageErrors.length > 0) {
      throw new Error(`browser page errors:\n${pageErrors.join("\n")}`);
    }

    console.log("[web-smoke] passed");
  } finally {
    await browser.close();
    staticServer?.close();
  }
};

main().catch((error) => {
  console.error("[web-smoke] failed:", error);
  process.exitCode = 1;
});
