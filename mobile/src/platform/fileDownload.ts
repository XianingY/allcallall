import { Linking, Platform } from "react-native";
import { File, Paths } from "expo-file-system";

import { resolveSafeDownloadFileName } from "./downloadFileName";

export interface FileDownloadRequest {
  fromUrl: string;
  headers?: Record<string, string>;
}

export interface DownloadResult {
  location: string;
}

export interface FileDownloadAdapter {
  download(request: FileDownloadRequest, fileName: string): Promise<DownloadResult>;
  open(result: DownloadResult): Promise<void>;
}

const webAdapter: FileDownloadAdapter = {
  async download(request, fileName) {
    const response = await fetch(request.fromUrl, {
      headers: request.headers,
    });
    if (!response.ok) {
      throw new Error(`download failed with status ${response.status}`);
    }

    const blob = await response.blob();
    const objectUrl = URL.createObjectURL(blob);
    const anchor = document.createElement("a");
    anchor.href = objectUrl;
    anchor.download = fileName;
    anchor.click();
    setTimeout(() => URL.revokeObjectURL(objectUrl), 5000);

    return { location: objectUrl };
  },
  async open() {
    return;
  },
};

const nativeAdapter: FileDownloadAdapter = {
  async download(request, fileName) {
    const destination = new File(
      Paths.document,
      resolveSafeDownloadFileName(fileName),
    );
    const file = await File.downloadFileAsync(request.fromUrl, destination, {
      headers: request.headers,
      idempotent: true,
    });
    return { location: file.uri };
  },
  async open(result) {
    try {
      await Linking.openURL(result.location);
    } catch {
      // Caller can show fallback UI.
    }
  },
};

const fileDownloadAdapter: FileDownloadAdapter =
  Platform.OS === "web" ? webAdapter : nativeAdapter;

export default fileDownloadAdapter;
