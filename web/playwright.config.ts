import { defineConfig, devices } from "@playwright/test";

export default defineConfig({
  testDir: "./tests/e2e",
  timeout: 30_000,
  use: { baseURL: process.env.WEB_E2E_BASE_URL || "http://127.0.0.1:4173", trace: "retain-on-failure", launchOptions: { args: ["--use-fake-device-for-media-stream", "--use-fake-ui-for-media-stream"] } },
  webServer: process.env.WEB_E2E_BASE_URL ? undefined : {
    command: "npm run preview -- --port 4173",
    port: 4173,
    reuseExistingServer: true,
  },
  projects: [
    { name: "desktop", use: { ...devices["Desktop Chrome"], browserName: "chromium" } },
    { name: "tablet", use: { ...devices["iPad (gen 7)"], browserName: "chromium" } },
    { name: "mobile", use: { ...devices["Pixel 7"], browserName: "chromium" } },
  ],
});
