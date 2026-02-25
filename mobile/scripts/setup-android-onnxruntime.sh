#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MOBILE_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
ORT_VERSION="${ORT_VERSION:-1.16.3}"
ORT_DIR="${MOBILE_ROOT}/android/app/src/main/cpp/libs/onnxruntime"

if [[ -f "${ORT_DIR}/headers/onnxruntime_c_api.h" && -f "${ORT_DIR}/jni/arm64-v8a/libonnxruntime.so" ]]; then
  echo "ONNX Runtime Android native libs already prepared: ${ORT_DIR}"
  exit 0
fi

echo "Preparing ONNX Runtime Android native libs (version: ${ORT_VERSION})..."
mkdir -p "${ORT_DIR}"
rm -rf "${ORT_DIR}/headers" "${ORT_DIR}/jni" "${ORT_DIR}/META-INF" "${ORT_DIR}/R.txt" "${ORT_DIR}/AndroidManifest.xml" "${ORT_DIR}/classes.jar"

TMP_AAR="$(mktemp "/tmp/onnxruntime-android-${ORT_VERSION}.XXXXXX")"
cleanup() {
  rm -f "${TMP_AAR}"
}
trap cleanup EXIT

URLS=(
  "https://maven.aliyun.com/repository/central/com/microsoft/onnxruntime/onnxruntime-android/${ORT_VERSION}/onnxruntime-android-${ORT_VERSION}.aar"
  "https://repo.maven.apache.org/maven2/com/microsoft/onnxruntime/onnxruntime-android/${ORT_VERSION}/onnxruntime-android-${ORT_VERSION}.aar"
)

download_ok=false
for url in "${URLS[@]}"; do
  if curl -fL --connect-timeout 15 --retry 2 --retry-delay 1 -o "${TMP_AAR}" "${url}"; then
    download_ok=true
    break
  fi
done

if [[ "${download_ok}" != "true" ]]; then
  echo "Failed to download onnxruntime-android-${ORT_VERSION}.aar" >&2
  exit 1
fi

unzip -oq "${TMP_AAR}" "headers/*" "jni/*" -d "${ORT_DIR}"

if [[ ! -f "${ORT_DIR}/jni/arm64-v8a/libonnxruntime.so" ]]; then
  echo "Missing ${ORT_DIR}/jni/arm64-v8a/libonnxruntime.so after extraction" >&2
  exit 1
fi

echo "ONNX Runtime Android native libs prepared: ${ORT_DIR}"
