package service

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/memodrive/backend/internal/config"
	"github.com/memodrive/backend/internal/model"
	"github.com/memodrive/backend/internal/store"
)

func TestUploadCancelRejectsMergingSession(t *testing.T) {
	uploads, db, cfg := newUploadServiceTestHarness(t)
	session := &model.UploadSession{
		ID:             "upload-1",
		FileName:       "large.bin",
		FileSize:       10,
		ChunkSize:      5,
		UploadedChunks: []int{0, 1},
		DestPath:       "/",
		Status:         "merging",
		ExpiresAt:      time.Now().UTC().Add(time.Hour),
	}
	if err := os.MkdirAll(filepath.Join(cfg.Storage.TempDir, session.ID), 0o755); err != nil {
		t.Fatalf("create session dir: %v", err)
	}
	if err := db.CreateUploadSession(context.Background(), session); err != nil {
		t.Fatalf("create upload session: %v", err)
	}

	if err := uploads.CancelSession(context.Background(), session.ID); err == nil {
		t.Fatal("expected cancelling a merging session to return an error")
	}

	updated, err := db.GetUploadSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("get upload session: %v", err)
	}
	if updated.Status != "merging" {
		t.Fatalf("expected status to remain merging, got %q", updated.Status)
	}
	if _, err := os.Stat(filepath.Join(cfg.Storage.TempDir, session.ID)); err != nil {
		t.Fatalf("expected merging temp dir to remain: %v", err)
	}
}

func TestUploadCompleteFailureAfterMergingMarksSessionFailed(t *testing.T) {
	uploads, db, _ := newUploadServiceTestHarness(t)
	session := &model.UploadSession{
		ID:             "upload-1",
		FileName:       "large.bin",
		FileSize:       10,
		ChunkSize:      5,
		UploadedChunks: []int{0, 1},
		DestPath:       "/",
		Status:         model.UploadStatusUploading,
		ExpiresAt:      time.Now().UTC().Add(time.Hour),
	}
	if err := os.MkdirAll(uploads.sessionDir(session.ID), 0o755); err != nil {
		t.Fatalf("create session dir: %v", err)
	}
	if err := os.WriteFile(uploads.chunkPath(session.ID, 0), []byte("12345"), 0o644); err != nil {
		t.Fatalf("write first chunk: %v", err)
	}
	if err := db.CreateUploadSession(context.Background(), session); err != nil {
		t.Fatalf("create upload session: %v", err)
	}

	if _, err := uploads.Complete(context.Background(), session.ID); err == nil {
		t.Fatal("expected complete to fail when a recorded chunk file is missing")
	}

	updated, err := db.GetUploadSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("get upload session: %v", err)
	}
	if updated.Status != model.UploadStatusFailed {
		t.Fatalf("expected failed status, got %q", updated.Status)
	}
}

func TestUploadCleanupExpiredRemovesFailedSessionChunks(t *testing.T) {
	uploads, db, _ := newUploadServiceTestHarness(t)
	session := &model.UploadSession{
		ID:              "failed-upload",
		FileName:        "failed.bin",
		RequestedName:   "failed.bin",
		ResolvedName:    "failed.bin",
		OverwritePolicy: string(ConflictReject),
		FileSize:        5,
		ChunkSize:       5,
		UploadedChunks:  []int{0},
		DestPath:        "/",
		Status:          model.UploadStatusFailed,
		ExpiresAt:       time.Now().UTC().Add(-time.Minute),
	}
	if err := os.MkdirAll(uploads.sessionDir(session.ID), 0o755); err != nil {
		t.Fatalf("create failed Upload Session dir: %v", err)
	}
	if err := os.WriteFile(uploads.chunkPath(session.ID, 0), []byte("hello"), 0o644); err != nil {
		t.Fatalf("write failed Upload Session chunk: %v", err)
	}
	if err := db.CreateUploadSession(context.Background(), session); err != nil {
		t.Fatalf("create failed Upload Session: %v", err)
	}

	if err := uploads.CleanupExpired(context.Background()); err != nil {
		t.Fatalf("CleanupExpired returned error: %v", err)
	}
	if _, err := os.Stat(uploads.sessionDir(session.ID)); !os.IsNotExist(err) {
		t.Fatalf("expected failed Upload Session temp removed, got %v", err)
	}
	updated, err := db.GetUploadSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("get expired Upload Session: %v", err)
	}
	if updated.Status != model.UploadStatusExpired {
		t.Fatalf("expected failed Upload Session to become expired, got %q", updated.Status)
	}
}

func TestUploadCompleteCreatesPipelineTask(t *testing.T) {
	uploads, _, cfg := newUploadServiceTestHarness(t)
	body := []byte("# Notes\n\nThis uploaded File should enter the File Indexing Pipeline.")
	session := &model.UploadSession{
		ID:             "upload-md",
		FileName:       "notes.md",
		FileSize:       int64(len(body)),
		ChunkSize:      int64(len(body)),
		UploadedChunks: []int{0},
		DestPath:       "/Notes",
		Status:         model.UploadStatusUploading,
		ExpiresAt:      time.Now().UTC().Add(time.Hour),
	}
	if err := os.MkdirAll(uploads.sessionDir(session.ID), 0o755); err != nil {
		t.Fatalf("create session dir: %v", err)
	}
	if err := os.WriteFile(uploads.chunkPath(session.ID, 0), body, 0o644); err != nil {
		t.Fatalf("write upload chunk: %v", err)
	}
	if err := uploads.store.CreateUploadSession(context.Background(), session); err != nil {
		t.Fatalf("create upload session: %v", err)
	}

	completion, err := uploads.Complete(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("complete upload: %v", err)
	}

	if completion.File == nil {
		t.Fatal("expected completed upload to include a File")
	}
	if completion.Task == nil {
		t.Fatal("expected completed upload to include a Pipeline Task")
	}
	if completion.Task.FileID != completion.File.ID {
		t.Fatalf("expected task to index file %s, got %s", completion.File.ID, completion.Task.FileID)
	}
	if completion.File.Path != "/Notes" {
		t.Fatalf("expected File to be stored in /Notes, got %q", completion.File.Path)
	}
	if _, err := os.Stat(filepath.Join(cfg.Storage.Root, filepath.FromSlash(completion.File.StoragePath))); err != nil {
		t.Fatalf("expected uploaded File on disk: %v", err)
	}
}

func TestUploadCompleteStoresMOVAsVideoQuickTimeWithoutChangingOriginalFile(t *testing.T) {
	uploads, db, cfg := newUploadServiceTestHarness(t)
	original := []byte("original mov bytes")
	session := &model.UploadSession{
		ID:             "upload-mov",
		FileName:       "Meeting.MOV",
		FileSize:       int64(len(original)),
		ChunkSize:      int64(len(original)),
		UploadedChunks: []int{0},
		DestPath:       "/Videos",
		Status:         model.UploadStatusUploading,
		ExpiresAt:      time.Now().UTC().Add(time.Hour),
	}
	if err := os.MkdirAll(uploads.sessionDir(session.ID), 0o755); err != nil {
		t.Fatalf("create session dir: %v", err)
	}
	if err := os.WriteFile(uploads.chunkPath(session.ID, 0), original, 0o644); err != nil {
		t.Fatalf("write upload chunk: %v", err)
	}
	if err := db.CreateUploadSession(context.Background(), session); err != nil {
		t.Fatalf("create upload session: %v", err)
	}

	completion, err := uploads.Complete(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("complete mov upload: %v", err)
	}
	file := completion.File

	if file.MimeType != "video/quicktime" {
		t.Fatalf("expected MOV to be stored as video/quicktime, got %q", file.MimeType)
	}
	if filepath.Ext(file.StoragePath) != ".MOV" {
		t.Fatalf("expected storage path to keep original extension, got %q", file.StoragePath)
	}
	stored, err := os.ReadFile(filepath.Join(cfg.Storage.Root, filepath.FromSlash(file.StoragePath)))
	if err != nil {
		t.Fatalf("read stored original file: %v", err)
	}
	if string(stored) != string(original) {
		t.Fatalf("expected stored file to keep original bytes, got %q", string(stored))
	}
}

func TestUploadSaveChunkRejectsBytesBeyondExpectedChunkSize(t *testing.T) {
	uploads, _, _ := newUploadServiceTestHarness(t)
	session, err := uploads.Init(context.Background(), InitUploadInput{
		FileName: "large.bin",
		FileSize: 8,
		DestPath: "/",
	})
	if err != nil {
		t.Fatalf("init upload: %v", err)
	}

	if _, err := uploads.SaveChunk(context.Background(), session.ID, 0, []byte("123456")); err == nil {
		t.Fatal("expected oversized upload chunk to be rejected")
	}
	if _, err := os.Stat(uploads.chunkPath(session.ID, 0)); err == nil {
		t.Fatal("expected oversized upload chunk not to be written")
	}
}

func TestUploadCompleteRechecksFullStagingCapacity(t *testing.T) {
	uploads, _, cfg := newUploadServiceTestHarness(t)
	cfg.Storage.TempLimitBytes = 100
	session, err := uploads.Init(context.Background(), InitUploadInput{
		FileName: "capacity.bin",
		FileSize: 5,
		DestPath: "/",
	})
	if err != nil {
		t.Fatalf("init upload: %v", err)
	}
	if _, err := uploads.SaveChunk(
		context.Background(),
		session.ID,
		0,
		[]byte("12345"),
	); err != nil {
		t.Fatalf("save upload chunk: %v", err)
	}

	cfg.Storage.TempLimitBytes = 9
	_, err = uploads.Complete(context.Background(), session.ID)
	if !IsInsufficientStorage(err) {
		t.Fatalf("Complete() error = %v, want insufficient staging storage", err)
	}
	current, getErr := uploads.GetSession(context.Background(), session.ID)
	if getErr != nil {
		t.Fatalf("get upload session: %v", getErr)
	}
	if current.Status != model.UploadStatusUploading {
		t.Fatalf("status = %q, want retryable uploading state", current.Status)
	}
}

func TestUploadSaveChunkStopsTemporaryGrowthAtLimit(t *testing.T) {
	uploads, _, cfg := newUploadServiceTestHarness(t)
	cfg.Storage.TempLimitBytes = 6
	session, err := uploads.Init(context.Background(), InitUploadInput{
		FileName: "limited.bin",
		FileSize: 5,
		DestPath: "/",
	})
	if err != nil {
		t.Fatalf("init upload: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(cfg.Storage.TempDir, "other.tmp"),
		[]byte("12"),
		0o644,
	); err != nil {
		t.Fatalf("write competing temp file: %v", err)
	}

	_, err = uploads.SaveChunk(
		context.Background(),
		session.ID,
		0,
		[]byte("12345"),
	)
	if !IsInsufficientStorage(err) {
		t.Fatalf("SaveChunk() error = %v, want insufficient temporary storage", err)
	}
	if _, statErr := os.Stat(uploads.chunkPath(session.ID, 0)); !os.IsNotExist(statErr) {
		t.Fatalf("chunk file should not grow temp storage, stat error = %v", statErr)
	}
}

func TestConcurrentRenameUploadsRetryTransactionConflicts(t *testing.T) {
	uploads, _, cfg := newUploadServiceTestHarness(t)
	sessions := make([]*model.UploadSession, 0, 2)
	for i := 0; i < 2; i++ {
		session, err := uploads.Init(context.Background(), InitUploadInput{
			FileName:        "race.txt",
			FileSize:        5,
			DestPath:        "/",
			OverwritePolicy: ConflictRename,
		})
		if err != nil {
			t.Fatalf("init rename upload %d: %v", i, err)
		}
		if _, err := uploads.SaveChunk(context.Background(), session.ID, 0, []byte("hello")); err != nil {
			t.Fatalf("save rename upload %d: %v", i, err)
		}
		sessions = append(sessions, session)
	}

	blockerStarted := make(chan struct{})
	releaseBlocker := make(chan struct{})
	blockerResult := make(chan error, 1)
	go func() {
		_, err := NewFileMutationService(cfg, uploads.store).Apply(
			context.Background(),
			FileMutationInput{
				Kind: model.FileMutationKindUploadCreate,
				File: &model.File{
					ID:          "blocking-file",
					Name:        "race.txt",
					Path:        "/",
					StoragePath: "blocking-race.txt",
					Size:        7,
					MimeType:    "text/plain",
					Status:      model.FileStatusUploaded,
					ChunkCount:  1,
				},
			},
			func(writer io.Writer) error {
				close(blockerStarted)
				<-releaseBlocker
				_, err := writer.Write([]byte("blocker"))
				return err
			},
		)
		blockerResult <- err
	}()
	<-blockerStarted

	type completionResult struct {
		completion *UploadCompletion
		err        error
	}
	results := make(chan completionResult, len(sessions))
	for _, session := range sessions {
		sessionID := session.ID
		go func() {
			completion, err := uploads.Complete(context.Background(), sessionID)
			results <- completionResult{completion: completion, err: err}
		}()
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		allMerging := true
		for _, session := range sessions {
			current, err := uploads.GetSession(context.Background(), session.ID)
			if err != nil {
				t.Fatalf("get concurrent upload state: %v", err)
			}
			if current.Status != model.UploadStatusMerging {
				allMerging = false
				break
			}
		}
		if allMerging {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("concurrent uploads did not reach merging state")
		}
		time.Sleep(10 * time.Millisecond)
	}
	close(releaseBlocker)
	if err := <-blockerResult; err != nil {
		t.Fatalf("commit blocking mutation: %v", err)
	}

	names := make([]string, 0, len(sessions))
	for range sessions {
		result := <-results
		if result.err != nil {
			t.Fatalf("complete concurrent rename upload: %v", result.err)
		}
		names = append(names, result.completion.File.Name)
	}
	sort.Strings(names)
	want := []string{"race (1).txt", "race (2).txt"}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("expected concurrent rename results %v, got %v", want, names)
		}
	}
}

func TestFiftyConcurrentRejectUploadsKeepSingleActiveTarget(t *testing.T) {
	uploads, db, _ := newUploadServiceTestHarness(t)
	const uploadCount = 50
	sessions := make([]*model.UploadSession, 0, uploadCount)
	for i := 0; i < uploadCount; i++ {
		session, err := uploads.Init(context.Background(), InitUploadInput{
			FileName:        "same-target.txt",
			FileSize:        5,
			DestPath:        "/",
			OverwritePolicy: ConflictReject,
		})
		if err != nil {
			t.Fatalf("init upload %d: %v", i, err)
		}
		if _, err := uploads.SaveChunk(context.Background(), session.ID, 0, []byte("hello")); err != nil {
			t.Fatalf("save upload %d: %v", i, err)
		}
		sessions = append(sessions, session)
	}

	start := make(chan struct{})
	results := make(chan error, uploadCount)
	for _, session := range sessions {
		sessionID := session.ID
		go func() {
			<-start
			_, err := uploads.Complete(context.Background(), sessionID)
			results <- err
		}()
	}
	close(start)

	succeeded := 0
	conflicted := 0
	var unexpected []error
	for range sessions {
		err := <-results
		if err == nil {
			succeeded++
			continue
		}
		var conflict *FileConflictError
		if errors.As(err, &conflict) {
			conflicted++
			continue
		}
		unexpected = append(unexpected, err)
	}
	if len(unexpected) > 0 {
		t.Fatalf("unexpected concurrent upload errors: %v", unexpected)
	}
	if succeeded != 1 || conflicted != uploadCount-1 {
		t.Fatalf("expected 1 success and %d conflicts, got %d successes and %d conflicts",
			uploadCount-1, succeeded, conflicted)
	}
	files, err := db.ListFiles(context.Background(), "/", "created_at")
	if err != nil {
		t.Fatalf("list active root Files: %v", err)
	}
	activeTargets := 0
	for _, file := range files {
		if file.Name == "same-target.txt" {
			activeTargets++
		}
	}
	if activeTargets != 1 {
		t.Fatalf("expected one active Target Path, got %d", activeTargets)
	}
}

func newUploadServiceTestHarness(t *testing.T) (*UploadService, *store.Store, *config.Config) {
	t.Helper()
	root := t.TempDir()
	cfg := &config.Config{
		Storage: config.StorageConfig{
			Root:         filepath.Join(root, "files"),
			DBPath:       filepath.Join(root, "db", "memodrive.db"),
			TempDir:      filepath.Join(root, "tmp"),
			ThumbnailDir: filepath.Join(root, "thumbs"),
			MaxFileSize:  1024 * 1024,
			ChunkSize:    5,
			UploadTTL:    time.Hour,
		},
	}
	if err := cfg.EnsureDirs(); err != nil {
		t.Fatalf("ensure dirs: %v", err)
	}
	db, err := store.Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	for _, folder := range []*model.File{
		{ID: "notes-folder", Name: "Notes", Path: "/", StoragePath: "Notes", IsDir: true, Status: model.FileStatusReady},
		{ID: "videos-folder", Name: "Videos", Path: "/", StoragePath: "Videos", IsDir: true, Status: model.FileStatusReady},
	} {
		if err := db.CreateFile(context.Background(), folder); err != nil {
			t.Fatalf("create upload test Folder %s: %v", folder.Name, err)
		}
	}
	files := NewFileService(cfg, db, nil)
	pipeline := NewPipelineService(cfg, db, nil, nil, nil, nil)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = pipeline.Shutdown(ctx)
	})
	return NewUploadService(cfg, db, files, pipeline), db, cfg
}
