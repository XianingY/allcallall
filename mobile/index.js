// Local app entry, resolvable under npm-workspaces hoisting.
//
// The classic Expo template points `main` at `node_modules/expo/AppEntry.js`,
// which only resolves when `expo` is installed inside the project's own
// node_modules. In this monorepo the workspace root hoists `expo`, so that
// relative path does not exist and `expo export` cannot find an entry.
// A project-local entry with a relative `./App` import works in both layouts.
import { registerRootComponent } from "expo";

import App from "./App";

registerRootComponent(App);
