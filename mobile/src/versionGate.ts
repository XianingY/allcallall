/**
 * Version gate logic, free of any React Native import so it can be tested on the
 * plain Node runner. The hook and the screens import from here; the test does
 * too, which is the point - importing the hook would pull in react-native's
 * Flow-typed entry point, which esbuild cannot parse outside Metro.
 */

export interface AppVersionCheck {
  platform: string;
  current_version: string;
  min_supported_version: string;
  latest_version: string;
  update_required: boolean;
  update_allowed: boolean;
  policy_configured: boolean;
  force_update: boolean;
  message: string;
}

/**
 * What the app should do about its own version.
 *
 *   checking - in flight; the UI holds back rather than flashing the app
 *   allowed  - no action needed
 *   optional - a newer release exists and the user may keep working
 *   blocked  - this build is below the supported floor; continuing is refused
 *   unknown  - the check itself failed or timed out
 *
 * `unknown` deliberately does not block. The check is an HTTP call that depends
 * on the network, the backend being up, and the endpoint being deployed; if any
 * of those is missing, refusing to open the app turns a backend problem into an
 * outage on every installed client. A client that cannot reach the server is
 * exactly the client most likely to need a new build.
 */
export type VersionGateState =
  | { status: "checking" }
  | { status: "allowed" }
  | { status: "optional"; check: AppVersionCheck }
  | { status: "blocked"; check: AppVersionCheck }
  | { status: "unknown"; reason: string };

/**
 * Decides what to do given the server's answer.
 *
 * Order matters: `update_allowed` is checked first because a build the server
 * has withdrawn support for must be told to update regardless of whether a
 * newer version also happens to be flagged. `force_update` is deliberately not
 * treated as blocking on its own - it is a hint for how to present the blocking
 * screen, and honouring it here would let a policy typo lock everyone out.
 */
export const decideVersionGate = (check: AppVersionCheck): VersionGateState => {
  if (!check.update_allowed) {
    return { status: "blocked", check };
  }
  if (check.update_required) {
    return { status: "optional", check };
  }
  return { status: "allowed" };
};
