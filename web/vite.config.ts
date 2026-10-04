import path from "node:path";
import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  resolve: { alias: { "@": path.resolve(__dirname, "src") } },
  build: {
    chunkSizeWarningLimit: 820,
    // Vite 8 unifies dev and production bundling on Rolldown (Rust). The old
    // `build.rollupOptions.output.manualChunks` is gone: the object form was
    // removed and the function form is deprecated, so the vendor split now
    // lives in `rolldownOptions.output.codeSplitting.groups`. Groups are
    // matched in order, first hit wins, and the catch-all keeps what used to
    // fall through to `vendor-core`.
    rolldownOptions: {
      output: {
        codeSplitting: {
          groups: [
            { name: "vendor-revenuecat", test: /node_modules[\\/](@revenuecat[\\/]|[^\\/]*[Pp]urchases)/ },
            { name: "vendor-firebase", test: /node_modules[\\/]firebase/ },
            { name: "vendor-agent-graph", test: /node_modules[\\/]@xyflow[\\/]/ },
            { name: "vendor-core", test: /node_modules/ },
          ],
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
      // Regression floor. Recalibrated for the Vitest 5 coverage engine,
      // which uses AST-aware remapping and counts implicit branches that the
      // old v8 provider missed (actuals: 66.49% lines, 52.41% statements,
      // 45.56% branches, 37.75% functions). Small slack below the measured
      // value keeps CI stable across minor report variance. Raise these as
      // more unit tests land.
      thresholds: { lines: 66, functions: 37, branches: 45, statements: 52 },
    },
  },
});
