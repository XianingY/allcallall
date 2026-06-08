import { chromium } from "playwright";
import fs from "node:fs";
import http from "node:http";
import path from "node:path";
import { fileURLToPath } from "node:url";

let baseURL = (process.env.WEB_SMOKE_BASE_URL || "http://127.0.0.1:8081").replace(/\/+$/, "");
const exportDir = process.env.WEB_SMOKE_EXPORT_DIR;
const email = process.env.WEB_SMOKE_EMAIL;
const password = process.env.WEB_SMOKE_PASSWORD;
const roomId = process.env.WEB_SMOKE_ROOM_ID;
const conversationId = process.env.WEB_SMOKE_CONVERSATION_ID;

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
  if (!exportDir || process.env.WEB_SMOKE_BASE_URL) {
    return null;
  }
  const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..", exportDir);
  const rootWithSeparator = `${root}${path.sep}`;
  if (!fs.existsSync(path.join(root, "index.html"))) {
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
  assertContainsAny(afterLogin, "post-login page", ["Meetings", "会议", "Inbox"]);
  return true;
};

const main = async () => {
  const staticServer = await startStaticExportServer();
  const browser = await chromium.launch({ headless: true });
  const page = await browser.newPage({ viewport: { width: 1440, height: 960 } });
  const pageErrors = [];
  page.on("pageerror", (error) => pageErrors.push(error.message));

  try {
    const loggedIn = await loginIfConfigured(page);

    const meetingsText = await visit(page, "/meetings", "Meetings");
    assertContainsAny(meetingsText, "Meetings", loggedIn ? ["Meetings", "Quick actions", "会议"] : ["AllCallAll", "Login", "登录"]);

    const roomPath = roomId ? `/rooms/${roomId}` : "/rooms/1";
    const roomText = await visit(page, roomPath, "room route");
    assertContainsAny(roomText, "room route", loggedIn ? ["会议", "Join", "加入"] : ["AllCallAll", "Login", "登录"]);

    const conversationPath = conversationId ? `/conversations/${conversationId}` : "/conversations/1";
    const conversationText = await visit(page, conversationPath, "conversation route");
    assertContainsAny(conversationText, "conversation route", loggedIn ? ["协作线程", "Inbox", "Conversation"] : ["AllCallAll", "Login", "登录"]);

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
