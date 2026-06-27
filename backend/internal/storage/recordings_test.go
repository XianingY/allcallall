package storage

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalRecordingStorageSaveAndResolve(t *testing.T) {
	root := filepath.Join(t.TempDir(), "recordings")
	store, err := NewRecordingStorage(Config{
		Driver:    DriverLocal,
		LocalRoot: root,
	})
	if err != nil {
		t.Fatalf("new storage failed: %v", err)
	}

	srcFile := filepath.Join(t.TempDir(), "sample.txt")
	if err := os.WriteFile(srcFile, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write source failed: %v", err)
	}

	ref, err := store.SaveFile(context.Background(), srcFile, "org-1/room-2/session-3/sample.txt", "text/plain")
	if err != nil {
		t.Fatalf("save file failed: %v", err)
	}
	if ref.Driver != DriverLocal {
		t.Fatalf("expected local driver, got %s", ref.Driver)
	}
	if _, err := os.Stat(ref.Key); err != nil {
		t.Fatalf("expected saved file to exist: %v", err)
	}

	localPath, ok := store.OpenLocal(*ref)
	if !ok || localPath == "" {
		t.Fatal("expected local path to resolve")
	}
	if localPath != ref.Key {
		t.Fatalf("expected local path %s, got %s", ref.Key, localPath)
	}

	downloadPath, err := store.SignedDownloadURL(context.Background(), *ref, 0)
	if err != nil {
		t.Fatalf("expected local download path: %v", err)
	}
	if downloadPath != ref.Key {
		t.Fatalf("expected download path %s, got %s", ref.Key, downloadPath)
	}
	reader, err := store.Open(context.Background(), *ref)
	if err != nil {
		t.Fatalf("open saved file failed: %v", err)
	}
	opened, err := io.ReadAll(reader)
	_ = reader.Close()
	if err != nil || string(opened) != "hello" {
		t.Fatalf("unexpected opened content=%q err=%v", string(opened), err)
	}
	if err := store.Delete(context.Background(), *ref); err != nil {
		t.Fatalf("delete saved file failed: %v", err)
	}
	if _, err := os.Stat(ref.Key); !os.IsNotExist(err) {
		t.Fatalf("expected saved file to be removed, got err=%v", err)
	}
}

func TestLocalRecordingStorageRejectsEscapingObjectKeys(t *testing.T) {
	root := filepath.Join(t.TempDir(), "recordings")
	store, err := NewRecordingStorage(Config{
		Driver:    DriverLocal,
		LocalRoot: root,
	})
	if err != nil {
		t.Fatalf("new storage failed: %v", err)
	}

	srcFile := filepath.Join(t.TempDir(), "sample.txt")
	if err := os.WriteFile(srcFile, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write source failed: %v", err)
	}

	if _, err := store.SaveFile(context.Background(), srcFile, "../outside.txt", "text/plain"); err == nil {
		t.Fatal("expected path traversal object key to be rejected")
	}
	if _, ok := store.OpenLocal(ObjectRef{
		Driver: DriverLocal,
		Key:    filepath.Join(root, "..", "outside.txt"),
	}); ok {
		t.Fatal("expected escaped local object path to be rejected")
	}
}

func TestS3RecordingStorageRequiresBucket(t *testing.T) {
	if _, err := NewRecordingStorage(Config{Driver: DriverS3}); err == nil {
		t.Fatal("expected missing bucket to fail")
	}
}

func TestS3RecordingStoragePublicBaseURL(t *testing.T) {
	store, err := NewRecordingStorage(Config{
		Driver:        DriverS3,
		S3Bucket:      "recordings",
		S3Region:      "us-east-1",
		S3AccessKeyID: "test",
		S3SecretKey:   "test",
		PublicBaseURL: "https://cdn.example.com/recordings",
	})
	if err != nil {
		t.Fatalf("new s3 storage failed: %v", err)
	}

	url, err := store.SignedDownloadURL(context.Background(), ObjectRef{
		Driver: DriverS3,
		Bucket: "recordings",
		Key:    "org-1//room-2/session-3/sample.ogg",
	}, 0)
	if err != nil {
		t.Fatalf("signed public url failed: %v", err)
	}
	if url != "https://cdn.example.com/recordings/org-1/room-2/session-3/sample.ogg" {
		t.Fatalf("unexpected public url: %s", url)
	}
}

func TestS3RecordingStorageRejectsInvalidObjectKeys(t *testing.T) {
	store, err := NewRecordingStorage(Config{
		Driver:        DriverS3,
		S3Bucket:      "recordings",
		S3Region:      "us-east-1",
		S3AccessKeyID: "test",
		S3SecretKey:   "test",
		PublicBaseURL: "https://cdn.example.com/recordings",
	})
	if err != nil {
		t.Fatalf("new s3 storage failed: %v", err)
	}

	invalidRefs := []ObjectRef{
		{Driver: DriverS3, Bucket: "recordings", Key: ""},
		{Driver: DriverS3, Bucket: "recordings", Key: "../secret.ogg"},
		{Driver: DriverS3, Bucket: "recordings", Key: "/absolute/secret.ogg"},
		{Driver: DriverS3, Bucket: "recordings", Key: "org-1/../secret.ogg"},
		{Driver: DriverLocal, Bucket: "recordings", Key: "org-1/sample.ogg"},
	}
	for _, ref := range invalidRefs {
		if _, err := store.SignedDownloadURL(context.Background(), ref, 0); err == nil {
			t.Fatalf("expected invalid object ref to fail: %+v", ref)
		}
	}

	if _, err := store.SaveFile(context.Background(), filepath.Join(t.TempDir(), "missing.ogg"), "", "audio/ogg"); err == nil || errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected empty key validation before source open, got %v", err)
	}
}
