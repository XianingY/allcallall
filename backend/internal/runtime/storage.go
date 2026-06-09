package runtime

import (
	"os"
	"strings"

	"github.com/allcallall/backend/internal/storage"
)

func RecordingStorageFromEnv() (storage.RecordingStorage, error) {
	return storage.NewRecordingStorage(storage.Config{
		Driver:        storage.Driver(strings.TrimSpace(os.Getenv("RECORDING_STORAGE_DRIVER"))),
		LocalRoot:     strings.TrimSpace(os.Getenv("RECORDING_STORAGE_DIR")),
		S3Bucket:      strings.TrimSpace(os.Getenv("RECORDING_S3_BUCKET")),
		S3Region:      strings.TrimSpace(os.Getenv("RECORDING_S3_REGION")),
		S3Endpoint:    strings.TrimSpace(os.Getenv("RECORDING_S3_ENDPOINT")),
		S3AccessKeyID: strings.TrimSpace(os.Getenv("RECORDING_S3_ACCESS_KEY_ID")),
		S3SecretKey:   strings.TrimSpace(os.Getenv("RECORDING_S3_SECRET_ACCESS_KEY")),
		S3ForcePath:   strings.TrimSpace(os.Getenv("RECORDING_S3_FORCE_PATH_STYLE")) == "1",
		PublicBaseURL: strings.TrimSpace(os.Getenv("RECORDING_PUBLIC_BASE_URL")),
	})
}
