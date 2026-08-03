package collaboration

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/allcallall/backend/internal/metrics"
	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/storage"
	"gorm.io/gorm"
)

// failingSaveRecordingStorage 让 SaveFile 永远失败，用于验证 Worker 的失败→退避/死亡分支。
type failingSaveRecordingStorage struct {
	storage.RecordingStorage
}

func (s failingSaveRecordingStorage) SaveFile(context.Context, string, string, string) (*storage.ObjectRef, error) {
	return nil, errors.New("save failed")
}

// countingRecordingStorage 在委托真实存储的同时统计 SaveFile 调用次数，用于验证
// 跨副本并发认领时上传动作只发生一次（不重复上传）。
type countingRecordingStorage struct {
	storage.RecordingStorage
	mu    sync.Mutex
	calls int
}

func (s *countingRecordingStorage) SaveFile(ctx context.Context, src, key, contentType string) (*storage.ObjectRef, error) {
	s.mu.Lock()
	s.calls++
	s.mu.Unlock()
	return s.RecordingStorage.SaveFile(ctx, src, key, contentType)
}

func (s *countingRecordingStorage) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func writeTempAudio(t testing.TB, name string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte("mock-audio-bytes"), 0o644); err != nil {
		t.Fatalf("write temp audio failed: %v", err)
	}
	return p
}

func reloadRecordingFile(t testing.TB, db *gorm.DB, id uint64) models.RecordingFile {
	t.Helper()
	var f models.RecordingFile
	if err := db.Where("id = ?", id).Take(&f).Error; err != nil {
		t.Fatalf("reload recording file failed: %v", err)
	}
	return f
}

// TestUploadBackoffExponentialAndCapped 验证指数退避公式与封顶行为。
func TestUploadBackoffExponentialAndCapped(t *testing.T) {
	cases := []struct {
		attempts int
		want     time.Duration
	}{
		{1, 30 * time.Second},
		{2, 60 * time.Second},
		{3, 120 * time.Second},
		{8, time.Hour}, // 30s * 128 = 64min，超过 1h 封顶
	}
	for _, c := range cases {
		if got := uploadBackoff(c.attempts); got != c.want {
			t.Fatalf("uploadBackoff(%d) = %v, want %v", c.attempts, got, c.want)
		}
	}
}

// TestProcessUploadBacklogSucceedsPendingToDone 验证 pending 行在本地源可用时上传成功并置为 done。
func TestProcessUploadBacklogSucceedsPendingToDone(t *testing.T) {
	svc, db, _ := newServiceTestEnv(t)
	ctx := context.Background()

	src := writeTempAudio(t, "track.ogg")
	file := models.RecordingFile{
		RecordingSessionID: 1,
		StorageDriver:      string(storage.DriverLocal),
		ObjectKey:          "org-1/room-1/session-1/track.ogg",
		ContentType:        "audio/ogg",
		LocalSrcPath:       src,
		UploadStatus:       uploadStatusPending,
		UploadAttempts:     0,
	}
	if err := db.Create(&file).Error; err != nil {
		t.Fatalf("create recording file failed: %v", err)
	}

	if err := svc.processUploadBacklog(ctx); err != nil {
		t.Fatalf("process upload backlog failed: %v", err)
	}

	got := reloadRecordingFile(t, db, file.ID)
	if got.UploadStatus != uploadStatusDone {
		t.Fatalf("expected done, got %s", got.UploadStatus)
	}
	if got.UploadAttempts != 1 {
		t.Fatalf("expected attempts 1, got %d", got.UploadAttempts)
	}
	if got.UploadLastError != "" {
		t.Fatalf("expected empty error, got %q", got.UploadLastError)
	}
}

// TestProcessUploadBacklogFailsThenBackoffPreventsImmediateRetry 验证失败行进入 failed，
// 且指数退避使得下一次立即扫描不会重复处理。
func TestProcessUploadBacklogFailsThenBackoffPreventsImmediateRetry(t *testing.T) {
	svc, db, _ := newServiceTestEnv(t)
	ctx := context.Background()
	svc.WithRecordingStorage(failingSaveRecordingStorage{RecordingStorage: svc.storage})

	src := writeTempAudio(t, "track.ogg")
	file := models.RecordingFile{
		RecordingSessionID: 1,
		StorageDriver:      string(storage.DriverLocal),
		ObjectKey:          "org-1/room-1/session-1/track.ogg",
		ContentType:        "audio/ogg",
		LocalSrcPath:       src,
		UploadStatus:       uploadStatusPending,
		UploadAttempts:     0,
	}
	if err := db.Create(&file).Error; err != nil {
		t.Fatalf("create recording file failed: %v", err)
	}

	if err := svc.processUploadBacklog(ctx); err != nil {
		t.Fatalf("process upload backlog failed: %v", err)
	}
	got := reloadRecordingFile(t, db, file.ID)
	if got.UploadStatus != uploadStatusFailed {
		t.Fatalf("expected failed, got %s", got.UploadStatus)
	}
	if got.UploadAttempts != 1 {
		t.Fatalf("expected attempts 1, got %d", got.UploadAttempts)
	}
	if got.NextRetryAt == nil {
		t.Fatal("expected next_retry_at to be set after failure")
	}
	if !got.NextRetryAt.After(time.Now()) {
		t.Fatalf("expected next_retry_at in the future, got %v", got.NextRetryAt)
	}

	// 立即再次扫描：退避未到期，不应重复处理。
	if err := svc.processUploadBacklog(ctx); err != nil {
		t.Fatalf("second process upload backlog failed: %v", err)
	}
	got2 := reloadRecordingFile(t, db, file.ID)
	if got2.UploadStatus != uploadStatusFailed {
		t.Fatalf("expected still failed (backoff honored), got %s", got2.UploadStatus)
	}
	if got2.UploadAttempts != 1 {
		t.Fatalf("expected attempts still 1 (no reprocessing), got %d", got2.UploadAttempts)
	}
}

// TestProcessUploadBacklogMarksDeadAfterMaxAttempts 验证失败次数达上限后进入 dead 并告警计数。
func TestProcessUploadBacklogMarksDeadAfterMaxAttempts(t *testing.T) {
	svc, db, _ := newServiceTestEnv(t)
	ctx := context.Background()
	store := metrics.NewCounterStore()
	svc.WithMetrics(store)
	svc.WithRecordingStorage(failingSaveRecordingStorage{RecordingStorage: svc.storage})

	src := writeTempAudio(t, "track.ogg")
	file := models.RecordingFile{
		RecordingSessionID: 1,
		StorageDriver:      string(storage.DriverLocal),
		ObjectKey:          "org-1/room-1/session-1/track.ogg",
		ContentType:        "audio/ogg",
		LocalSrcPath:       src,
		UploadStatus:       uploadStatusFailed,
		UploadAttempts:     maxUploadAttempts - 1, // 9：再认领一次即达到上限
	}
	if err := db.Create(&file).Error; err != nil {
		t.Fatalf("create recording file failed: %v", err)
	}

	if err := svc.processUploadBacklog(ctx); err != nil {
		t.Fatalf("process upload backlog failed: %v", err)
	}
	got := reloadRecordingFile(t, db, file.ID)
	if got.UploadStatus != uploadStatusDead {
		t.Fatalf("expected dead, got %s", got.UploadStatus)
	}
	if got.UploadAttempts != maxUploadAttempts {
		t.Fatalf("expected attempts %d, got %d", maxUploadAttempts, got.UploadAttempts)
	}
	if store.Snapshot()["recording_upload_dead_total"] < 1 {
		t.Fatal("expected recording_upload_dead_total metric to be incremented")
	}
}

// TestProcessUploadBacklogMarksDeadWhenLocalSourceMissing 验证本地源不可达（如已被清理）
// 时无法恢复，直接标记 dead。
func TestProcessUploadBacklogMarksDeadWhenLocalSourceMissing(t *testing.T) {
	svc, db, _ := newServiceTestEnv(t)
	ctx := context.Background()
	store := metrics.NewCounterStore()
	svc.WithMetrics(store)

	missing := filepath.Join(t.TempDir(), "does-not-exist.ogg")
	file := models.RecordingFile{
		RecordingSessionID: 1,
		StorageDriver:      string(storage.DriverLocal),
		ObjectKey:          "org-1/room-1/session-1/missing.ogg",
		ContentType:        "audio/ogg",
		LocalSrcPath:       missing,
		UploadStatus:       uploadStatusPending,
		UploadAttempts:     0,
	}
	if err := db.Create(&file).Error; err != nil {
		t.Fatalf("create recording file failed: %v", err)
	}

	if err := svc.processUploadBacklog(ctx); err != nil {
		t.Fatalf("process upload backlog failed: %v", err)
	}
	got := reloadRecordingFile(t, db, file.ID)
	if got.UploadStatus != uploadStatusDead {
		t.Fatalf("expected dead, got %s", got.UploadStatus)
	}
	if !strings.Contains(got.UploadLastError, "local source missing") {
		t.Fatalf("expected local source missing error, got %q", got.UploadLastError)
	}
	if store.Snapshot()["recording_upload_dead_total"] < 1 {
		t.Fatal("expected recording_upload_dead_total metric incremented")
	}
}

// TestProcessUploadBacklogSkipsAlreadyClaimedRow 模拟另一副本已将行认领为 uploading，
// 本副本的扫描应跳过该行（候选查询只选 pending/failed），避免重复上传。
func TestProcessUploadBacklogSkipsAlreadyClaimedRow(t *testing.T) {
	svc, db, _ := newServiceTestEnv(t)
	ctx := context.Background()
	// 若 SaveFile 被调用即说明未被正确跳过——此处用永远失败的存储作为保险。
	svc.WithRecordingStorage(failingSaveRecordingStorage{RecordingStorage: svc.storage})

	src := writeTempAudio(t, "track.ogg")
	file := models.RecordingFile{
		RecordingSessionID: 1,
		StorageDriver:      string(storage.DriverLocal),
		ObjectKey:          "org-1/room-1/session-1/track.ogg",
		ContentType:        "audio/ogg",
		LocalSrcPath:       src,
		UploadStatus:       uploadStatusUploading, // 模拟已被其他副本认领
		UploadAttempts:     1,
	}
	if err := db.Create(&file).Error; err != nil {
		t.Fatalf("create recording file failed: %v", err)
	}

	if err := svc.processUploadBacklog(ctx); err != nil {
		t.Fatalf("process upload backlog failed: %v", err)
	}
	got := reloadRecordingFile(t, db, file.ID)
	if got.UploadStatus != uploadStatusUploading {
		t.Fatalf("expected row to remain uploading (skipped), got %s", got.UploadStatus)
	}
}

// TestProcessUploadBacklogConcurrentClaimIsUnique 验证多副本并发扫描同一 pending 行时，
// 原子认领保证 SaveFile 只被调用一次（不会重复上传）。
func TestProcessUploadBacklogConcurrentClaimIsUnique(t *testing.T) {
	svc, db, _ := newServiceTestEnv(t)
	ctx := context.Background()
	counter := &countingRecordingStorage{RecordingStorage: svc.storage}
	svc.WithRecordingStorage(counter)

	src := writeTempAudio(t, "track.ogg")
	file := models.RecordingFile{
		RecordingSessionID: 1,
		StorageDriver:      string(storage.DriverLocal),
		ObjectKey:          "org-1/room-1/session-1/track.ogg",
		ContentType:        "audio/ogg",
		LocalSrcPath:       src,
		UploadStatus:       uploadStatusPending,
		UploadAttempts:     0,
	}
	if err := db.Create(&file).Error; err != nil {
		t.Fatalf("create recording file failed: %v", err)
	}

	const workers = 8
	var start sync.WaitGroup
	var done sync.WaitGroup
	start.Add(1)
	for i := 0; i < workers; i++ {
		done.Add(1)
		go func() {
			defer done.Done()
			start.Wait()
			_ = svc.processUploadBacklog(ctx)
		}()
	}
	start.Done()
	done.Wait()

	if got := counter.count(); got != 1 {
		t.Fatalf("expected SaveFile to be called exactly once across replicas, got %d", got)
	}
	got := reloadRecordingFile(t, db, file.ID)
	if got.UploadStatus != uploadStatusDone {
		t.Fatalf("expected row to be done, got %s", got.UploadStatus)
	}
}

// TestStartUploadWorkerProcessesPendingRow 验证后台 Worker 协程（ticker 驱动）能真正处理待上传行。
func TestStartUploadWorkerProcessesPendingRow(t *testing.T) {
	svc, db, _ := newServiceTestEnv(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	src := writeTempAudio(t, "track.ogg")
	file := models.RecordingFile{
		RecordingSessionID: 1,
		StorageDriver:      string(storage.DriverLocal),
		ObjectKey:          "org-1/room-1/session-1/track.ogg",
		ContentType:        "audio/ogg",
		LocalSrcPath:       src,
		UploadStatus:       uploadStatusPending,
		UploadAttempts:     0,
	}
	if err := db.Create(&file).Error; err != nil {
		t.Fatalf("create recording file failed: %v", err)
	}

	svc.StartUploadWorker(ctx, 20*time.Millisecond)

	deadline := time.Now().Add(3 * time.Second)
	for {
		got := reloadRecordingFile(t, db, file.ID)
		if got.UploadStatus == uploadStatusDone {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("worker did not process pending row in time, status=%s", got.UploadStatus)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// TestPersistRecordingArtifactsDefersToWorkerOnUploadFailure 验证 persist 阶段同步上传失败时
// 不阻断整体持久化流程，而是把文件行记为 pending 交给后台 Worker（至少一次投递 + S3 覆盖幂等）。
func TestPersistRecordingArtifactsDefersToWorkerOnUploadFailure(t *testing.T) {
	svc, db, _ := newServiceTestEnv(t)
	ctx := context.Background()
	store := metrics.NewCounterStore()
	svc.WithMetrics(store)
	svc.WithRecordingStorage(failingSaveRecordingStorage{RecordingStorage: svc.storage})
	t.Setenv("RECORDING_STORAGE_DIR", t.TempDir())

	now := time.Now()
	session := models.RecordingSession{
		OrganizationID: 1,
		RoomID:         1,
		StartedBy:      1,
		Status:         models.RecordingStatusStopped,
		StartedAt:      &now,
		StoppedAt:      &now,
	}
	if err := db.Create(&session).Error; err != nil {
		t.Fatalf("create session failed: %v", err)
	}

	// 即使对象存储写入失败，persist 也应返回 nil（失败被推迟到 Worker），而非中断整个流程。
	if err := svc.persistRecordingArtifacts(ctx, 1, 1, session, now); err != nil {
		t.Fatalf("persist should defer rather than fail: %v", err)
	}

	var files []models.RecordingFile
	if err := db.Where("recording_session_id = ?", session.ID).Find(&files).Error; err != nil {
		t.Fatalf("load files failed: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected one deferred manifest file, got %d", len(files))
	}
	if files[0].UploadStatus != uploadStatusPending {
		t.Fatalf("expected pending upload status, got %s", files[0].UploadStatus)
	}
	if files[0].LocalSrcPath == "" {
		t.Fatal("expected local source path to be recorded for retry")
	}
	if store.Snapshot()["recording_storage_write_fail_total"] < 1 {
		t.Fatal("expected recording_storage_write_fail_total metric to be incremented")
	}
}
