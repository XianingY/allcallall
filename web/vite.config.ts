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
      // These thresholds are a regression floor set at the honest baseline
      // measured after fixing the coverage include (previously it only covered
      // lib/realtime/stores and excluded the api/components/pages tests) and
      // adding realtime unit tests. They prevent coverage from dropping. The
      // large untested React page/component tree is tracked follow-up work;
      // raise these numbers as more unit tests land.
      thresholds: { lines: 15, functions: 35, branches: 55, statements: 15 },
    },
  },
});
