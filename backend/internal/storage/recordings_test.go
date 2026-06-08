package storage

import (
	"context"
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
