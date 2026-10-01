const MAX_FILE_NAME_LENGTH = 180;
const FALLBACK_FILE_NAME = "download";

/**
 * Turn an externally supplied name into one safe to append to the app document
 * directory. Native download paths are strings, so a malicious attachment name
 * such as `../../secret.txt` must never be able to escape that directory.
 */
export function resolveSafeDownloadFileName(input: string | null | undefined): string {
  const segments = (input ?? "")
    .split(/[\\/]+/)
    .map((segment) => segment.replace(/[\u0000-\u001f\u007f]/g, "").trim())
    .filter((segment) => segment.length > 0 && segment !== "." && segment !== "..");

  const fileName = segments.at(-1);
  if (!fileName) {
    return FALLBACK_FILE_NAME;
  }

  if (fileName.length <= MAX_FILE_NAME_LENGTH) {
    return fileName;
  }

  const extensionMatch = fileName.match(/(\.[^.]+)$/);
  const extension = extensionMatch?.[0] ?? "";
  const baseLimit = Math.max(1, MAX_FILE_NAME_LENGTH - extension.length);
  return `${fileName.slice(0, baseLimit)}${extension}`;
}
