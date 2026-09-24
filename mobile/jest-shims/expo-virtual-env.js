// Shim for Expo's virtual `expo/virtual/env` module.
//
// Metro (via @expo/metro-config) injects this module at build time. babel-preset-expo's
// `inline-env-vars` plugin rewrites every `process.env.EXPO_PUBLIC_*` reference into a
// named import `import { env } from 'expo/virtual/env'`, so under jest the shim MUST
// export a named `env` object (an empty one is fine: tests rely on the secure defaults
// in src/config when no EXPO_PUBLIC_* var is set). The published `expo` package does not
// ship a real file for it and jest-expo does not map it, hence this shim.
globalThis.__DEV__ = true;
module.exports = { env: globalThis.process ? globalThis.process.env : {} };
