import { Platform } from "react-native";

// 附件选择的统一返回：uri 在 web 上是 objectURL（并保留 blob 原件），
// 在 native 上是 file:// 路径，直接交给 FormData multipart 上传。
export interface PickedAttachment {
  uri: string;
  name: string;
  type: string;
  size: number;
  blob?: Blob;
}

// 与后端 openapi 契约一致的硬上限（26MB）。
export const MAX_ATTACHMENT_BYTES = 26 * 1024 * 1024;

const pickOnWeb = (): Promise<PickedAttachment | null> =>
  new Promise((resolve) => {
    const input = document.createElement("input");
    input.type = "file";
    input.onchange = () => {
      const file = input.files?.[0];
      if (!file) {
        resolve(null);
        return;
      }
      resolve({
        uri: URL.createObjectURL(file),
        name: file.name,
        type: file.type || "application/octet-stream",
        size: file.size,
        blob: file,
      });
    };
    input.oncancel = () => resolve(null);
    input.click();
  });

const pickOnNative = async (): Promise<PickedAttachment | null> => {
  // expo-document-picker 属于 Expo SDK，但需要随 dev client 重新构建原生包。
  // 未安装时优雅降级：返回 null，由调用方提示。
  let picker: {
    getDocumentAsync: () => Promise<{
      canceled?: boolean;
      cancelled?: boolean;
      assets?: { uri: string; name?: string; mimeType?: string; size?: number }[];
    }>;
  };
  try {
    picker = require("expo-document-picker");
  } catch {
    return null;
  }
  const result = await picker.getDocumentAsync();
  if (result.canceled ?? result.cancelled) {
    return null;
  }
  const asset = result.assets?.[0];
  if (!asset) {
    return null;
  }
  return {
    uri: asset.uri,
    name: asset.name ?? "attachment",
    type: asset.mimeType ?? "application/octet-stream",
    size: asset.size ?? 0,
  };
};

/** 打开系统文件选择器；用户取消返回 null。 */
export const pickAttachmentFile = (): Promise<PickedAttachment | null> =>
  Platform.OS === "web" ? pickOnWeb() : pickOnNative();

/** native 上是否具备文件选择能力（取决于是否安装了 expo-document-picker）。 */
export const hasNativeAttachmentPicker = (): boolean => {
  if (Platform.OS === "web") {
    return true;
  }
  try {
    require("expo-document-picker");
    return true;
  } catch {
    return false;
  }
};