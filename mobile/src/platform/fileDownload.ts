import { Linking, Platform } from "react-native";

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
    const RNFS = require("react-native-fs");
    const destination = `${RNFS.DocumentDirectoryPath}/${fileName}`;
    const result = await RNFS.downloadFile({
      fromUrl: request.fromUrl,
      headers: request.headers,
      toFile: destination,
      background: true,
      discretionary: true,
    }).promise;
    if (result.statusCode < 200 || result.statusCode >= 300) {
      throw new Error(`download failed with status ${result.statusCode}`);
    }
    return { location: destination };
  },
  async open(result) {
    try {
      await Linking.openURL(`file://${result.location}`);
    } catch {
      // Caller can show fallback UI.
    }
  },
};

const fileDownloadAdapter: FileDownloadAdapter =
  Platform.OS === "web" ? webAdapter : nativeAdapter;

export default fileDownloadAdapter;
