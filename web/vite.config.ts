import path from "node:path";
import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  resolve: { alias: { "@": path.resolve(__dirname, "src") } },
  build: {
    chunkSizeWarningLimit: 820,
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (!id.includes("node_modules")) return undefined;
          if (id.includes("@revenuecat") || id.includes("Purchases.")) return "vendor-revenuecat";
          if (id.includes("firebase")) return "vendor-firebase";
          if (id.includes("@xyflow")) return "vendor-agent-graph";
          return "vendor-core";
        },
      },
    },
  },
  server: {
    port: 5173,
    proxy: {
      "/api": { target: "http://127.0.0.1:8080", changeOrigin: true, ws: true },
    },
  },
  test: {
    environment: "jsdom",
    setupFiles: "./src/test/setup.ts",
    include: ["src/**/*.{test,spec}.{ts,tsx}"],
    coverage: {
      provider: "v8",
      reporter: ["text", "json-summary"],
      include: [
        "src/lib/**/*.ts",
        "src/realtime/**/*.{ts,tsx}",
        "src/api/**/*.ts",
        "src/components/**/*.{ts,tsx}",
        "src/pages/**/*.{ts,tsx}",
      ],
      // Regression floor, measured after the coverage include fix and the
      // realtime/api/page/MCP/settings panel unit tests landed (actuals:
      // ~58.5% lines, ~70.9% branches, ~49% functions). Small slack below the
      // measured value keeps CI stable across minor report variance.
      // Raise these as more unit tests land.
      thresholds: { lines: 57, functions: 48, branches: 69, statements: 57 },
    },
  },
});
