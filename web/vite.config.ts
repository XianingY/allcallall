import path from "node:path";
import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  // mobile pins React 19.2.3 (Expo 57 / RN 0.86), so npm hoists react and
  // react-dom to the workspace root while web/ stays on 18.3.1. react-i18next
  // satisfies both workspaces and gets deduped to the root copy, which means it
  // binds to React 19 while web's components render with React 18: the hook
  // then calls useContext on a React whose dispatcher is null and the app dies
  // on "Cannot read properties of null (reading 'useContext')". Deduping both
  // names to this project's copy keeps a single React in the graph. src/
  // components/PageState.tsx documents the same hazard from the other side by
  // avoiding the hook entirely.
  resolve: {
    alias: { "@": path.resolve(__dirname, "src") },
    dedupe: ["react", "react-dom"],
  },
  build: {
    chunkSizeWarningLimit: 820,
    // Vite 8 unifies dev and production bundling on Rolldown (Rust). The old
    // `build.rollupOptions.output.manualChunks` is gone: the object form was
    // removed and the function form is deprecated, so the vendor split now
    // lives in `rolldownOptions.output.codeSplitting.groups`.
    rolldownOptions: {
      output: {
        codeSplitting: {
          groups: [
            // React has to be claimed by exactly one group. Rolldown pulls a
            // matched group's dependencies in recursively, so without this
            // group @xyflow and firebase both dragged react in, the catch-all
            // matched it as well, and React shipped in two chunks: every hook
            // call then failed with "Cannot read properties of null (reading
            // 'useContext')". A group with a higher priority removes its
            // modules from the other groups, so this one wins everywhere.
            {
              name: "vendor-react",
              test: /node_modules[\\/](react|react-dom|scheduler|react-router|react-router-dom)[\\/]/,
              priority: 30,
            },
            { name: "vendor-revenuecat", test: /node_modules[\\/](@revenuecat[\\/]|[^\\/]*[Pp]urchases)/, priority: 20 },
            { name: "vendor-firebase", test: /node_modules[\\/]firebase/, priority: 20 },
            { name: "vendor-agent-graph", test: /node_modules[\\/]@xyflow[\\/]/, priority: 20 },
            // Catch-all last, and lowest priority so it only takes what no
            // other group claimed.
            { name: "vendor-core", test: /node_modules/, priority: 0 },
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
