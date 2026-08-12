package service

import (
	"bytes"
	"context"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/memodrive/backend/internal/config"
	"github.com/memodrive/backend/internal/model"
	"github.com/memodrive/backend/internal/store"
)

func TestReconcilerPeriodicSweepFailsStuckTasks(t *testing.T) {
	cfg, db := newReconcilerTestStore(t)
	cfg.Janitor.MaxTaskAge = -time.Second
	reconciler := NewReconciler(cfg, db, nil, NewFileService(cfg, db, nil))

	file := &model.File{
		ID:          "file-1",
		Name:        "sample.md",
		Path:        "/",
		StoragePath: "sample.md",
		MimeType:    "text/markdown",
		Status:      model.FileStatusProcessing,
	}
	if err := db.CreateFile(context.Background(), file); err != nil {
		t.Fatalf("create file: %v", err)
	}
	task := &model.Task{
		ID:       "task-1",
		FileID:   file.ID,
		Type:     "pipeline",
		Status:   model.TaskStatusProcessing,
		Progress: 45,
	}
	if err := db.CreateTask(context.Background(), task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	if err := reconciler.PeriodicSweep(context.Background()); err != nil {
		t.Fatalf("PeriodicSweep returned error: %v", err)
	}
	updatedTask, err := db.GetTask(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if updatedTask.Status != model.TaskStatusFailed || updatedTask.Error == nil {
		t.Fatalf("expected failed task with error, got %#v", updatedTask)
	}
	updatedFile, err := db.GetFile(context.Background(), file.ID)
	if err != nil {
		t.Fatalf("get file: %v", err)
	}
	if updatedFile.Status != model.FileStatusFailed {
		t.Fatalf("expected failed file, got %q", updatedFile.Status)
	}
}

func TestReconcilerPeriodicSweepCleansWebDAVTemp(t *testing.T) {
	cfg, db := newReconcilerTestStore(t)
	cfg.Storage.UploadTTL = time.Hour
	reconciler := NewReconciler(cfg, db, nil, NewFileService(cfg, db, nil))
	webDAVTempDir := filepath.Join(cfg.Storage.TempDir, "webdav")
	if err := os.MkdirAll(webDAVTempDir, 0o755); err != nil {
		t.Fatalf("create webdav temp dir: %v", err)
	}
	expired := filepath.Join(webDAVTempDir, "expired.upload")
	if err := writeSmallFile(expired); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(expired, old, old); err != nil {
		t.Fatalf("age temp file: %v", err)
	}

	if err := reconciler.PeriodicSweep(context.Background()); err != nil {
		t.Fatalf("PeriodicSweep returned error: %v", err)
	}
	if exists(expired) {
		t.Fatal("expected periodic sweep to remove expired WebDAV temp file")
	}
}

func TestReconcilerPeriodicSweepCleansExpiredTerminalFileMutation(t *testing.T) {
	cfg, db := newReconcilerTestStore(t)
	cfg.Storage.UploadTTL = -time.Hour
	stagingRel := filepath.ToSlash(filepath.Join(".staging", "expired-mutation"))
	stagingPath := filepath.Join(cfg.Storage.Root, filepath.FromSlash(stagingRel))
	if err := os.MkdirAll(stagingPath, 0o755); err != nil {
		t.Fatalf("create mutation staging: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stagingPath, "rollback"), []byte("old"), 0o644); err != nil {
		t.Fatalf("write mutation rollback: %v", err)
	}
	if err := db.CreateFileMutation(context.Background(), &model.FileMutation{
		ID:               "expired-mutation",
		Kind:             model.FileMutationKindUploadReplace,
		State:            model.FileMutationStateFailed,
		VirtualPath:      "/report.txt",
		TargetFileID:     "report",
		StagedPath:       filepath.ToSlash(filepath.Join(stagingRel, "content")),
		OldStoragePath:   filepath.ToSlash(filepath.Join(stagingRel, "rollback")),
		FinalStoragePath: "report.txt",
		Error:            "rolled back",
	}); err != nil {
		t.Fatalf("create terminal mutation: %v", err)
	}

	reconciler := NewReconciler(cfg, db, nil, NewFileService(cfg, db, nil))
	if err := reconciler.PeriodicSweep(context.Background()); err != nil {
		t.Fatalf("PeriodicSweep returned error: %v", err)
	}
	if exists(stagingPath) {
		t.Fatal("expected expired terminal mutation staging to be removed")
	}
}

func TestReconcilerPeriodicSweepCleansExpiredUnjournaledStaging(t *testing.T) {
	cfg, db := newReconcilerTestStore(t)
	cfg.Storage.UploadTTL = time.Hour
	orphan := filepath.Join(cfg.Storage.Root, ".staging", "orphan-mutation")
	if err := os.MkdirAll(orphan, 0o755); err != nil {
		t.Fatalf("create orphan staging: %v", err)
	}
	if err := os.WriteFile(filepath.Join(orphan, "content"), []byte("partial"), 0o644); err != nil {
		t.Fatalf("write orphan staging content: %v", err)
	}
	expired := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(orphan, expired, expired); err != nil {
		t.Fatalf("age orphan staging: %v", err)
	}

	reconciler := NewReconciler(cfg, db, nil, NewFileService(cfg, db, nil))
	if err := reconciler.PeriodicSweep(context.Background()); err != nil {
		t.Fatalf("PeriodicSweep returned error: %v", err)
	}
	if exists(orphan) {
		t.Fatal("expected expired unjournaled staging to be removed")
	}
}

func TestReconcilerRecoverOnBootRequeuesThroughPipeline(t *testing.T) {
	cfg, db := newPipelineTestStore(t)
	cfg.Pipeline.Workers = 1
	cfg.Pipeline.EmbedBatchSize = 1
	provider := newBlockingEmbedProvider()
	pipeline := NewPipelineService(cfg, db, provider, noopVectorStore{}, nil, nil)
	reconciler := NewReconciler(cfg, db, pipeline, NewFileService(cfg, db, nil))

	activeFile := createPipelineTestFile(t, cfg, db, "active-file", "active.md")
	recoveredFile := createPipelineTestFile(t, cfg, db, "recovered-file", "recovered.md")
	activeTask, err := pipeline.Enqueue(context.Background(), activeFile)
	if err != nil {
		t.Fatalf("enqueue active file: %v", err)
	}
	provider.waitForEmbed(t)
	if err := db.UpdateTask(context.Background(), activeTask.ID, model.TaskStatusDone, 100, nil); err != nil {
		t.Fatalf("hide active task from recovery query: %v", err)
	}

	recoveredTask := &model.Task{
		ID:       "recovered-task",
		FileID:   recoveredFile.ID,
		Type:     "pipeline",
		Status:   model.TaskStatusPending,
		Progress: 0,
	}
	if err := db.CreateTask(context.Background(), recoveredTask); err != nil {
		t.Fatalf("create recovered task: %v", err)
	}

	if err := reconciler.RecoverOnBoot(context.Background()); err != nil {
		t.Fatalf("RecoverOnBoot returned error: %v", err)
	}
	defer provider.releaseEmbeds()

	updated, err := pipeline.GetTask(context.Background(), recoveredTask.ID)
	if err != nil {
		t.Fatalf("get recovered task: %v", err)
	}
	if updated.Status != model.TaskStatusFailed || updated.RetryCount != 0 || updated.Error == nil {
		t.Fatalf("expected original stuck Task to remain as failed audit history, got %#v", updated)
	}
	page, err := pipeline.ListTasks(context.Background(), "", recoveredFile.ID, "", 10)
	if err != nil {
		t.Fatalf("list recovered File Tasks: %v", err)
	}
	var retry *model.TaskListItem
	for i := range page.Items {
		if page.Items[i].RetryOfTaskID == recoveredTask.ID {
			retry = &page.Items[i]
			break
		}
	}
	if retry == nil || retry.RetryCount != 1 {
		t.Fatalf("expected linked recovery Task with retry_count 1, got %#v", page.Items)
	}
	assertTaskStaysStatus(t, pipeline, retry.ID, model.TaskStatusPending, 150*time.Millisecond)
}

func TestReconcilerRecoverOnBootRollsBackUncommittedFileMutationIdempotently(t *testing.T) {
	cfg, db := newReconcilerTestStore(t)
	file := &model.File{
		ID:          "replace-target",
		Name:        "report.txt",
		Path:        "/",
		StoragePath: "report.txt",
		Size:        3,
		MimeType:    "text/plain",
		Status:      model.FileStatusReady,
	}
	if err := db.CreateFile(context.Background(), file); err != nil {
		t.Fatalf("create target File: %v", err)
	}
	finalPath := filepath.Join(cfg.Storage.Root, file.StoragePath)
	if err := os.WriteFile(finalPath, []byte("new"), 0o644); err != nil {
		t.Fatalf("write uncommitted content: %v", err)
	}
	stagingRel := filepath.ToSlash(filepath.Join(".staging", "mutation-recover"))
	rollbackRel := filepath.ToSlash(filepath.Join(stagingRel, "rollback"))
	rollbackPath := filepath.Join(cfg.Storage.Root, filepath.FromSlash(rollbackRel))
	if err := os.MkdirAll(filepath.Dir(rollbackPath), 0o755); err != nil {
		t.Fatalf("create mutation staging: %v", err)
	}
	if err := os.WriteFile(rollbackPath, []byte("old"), 0o644); err != nil {
		t.Fatalf("write rollback content: %v", err)
	}
	mutation := &model.FileMutation{
		ID:               "mutation-recover",
		Kind:             model.FileMutationKindUploadReplace,
		State:            model.FileMutationStateFSApplied,
		VirtualPath:      "/report.txt",
		TargetFileID:     file.ID,
		StagedPath:       filepath.ToSlash(filepath.Join(stagingRel, "content")),
		OldStoragePath:   rollbackRel,
		FinalStoragePath: file.StoragePath,
	}
	if err := db.CreateFileMutation(context.Background(), mutation); err != nil {
		t.Fatalf("create interrupted mutation: %v", err)
	}

	reconciler := NewReconciler(cfg, db, nil, NewFileService(cfg, db, nil))
	for attempt := 1; attempt <= 2; attempt++ {
		if err := reconciler.RecoverOnBoot(context.Background()); err != nil {
			t.Fatalf("RecoverOnBoot attempt %d: %v", attempt, err)
		}
		content, err := os.ReadFile(finalPath)
		if err != nil {
			t.Fatalf("read recovered File attempt %d: %v", attempt, err)
		}
		if string(content) != "old" {
			t.Fatalf("expected old content after recovery attempt %d, got %q", attempt, content)
		}
	}
}

func TestReconcilerRecoverOnBootRemovesInterruptedFolderCopyAndMarksItFailed(t *testing.T) {
	cfg, db := newReconcilerTestStore(t)
	files := NewFileService(cfg, db, nil)
	root := &model.File{
		ID: "partial-copy", Name: "Folder-copy", Path: "/", StoragePath: "Folder-copy",
		IsDir: true, Status: model.FileStatusReady,
	}
	if err := os.MkdirAll(filepath.Join(cfg.Storage.Root, root.StoragePath), 0o755); err != nil {
		t.Fatalf("create partial copy storage: %v", err)
	}
	if err := db.CreateFile(context.Background(), root); err != nil {
		t.Fatalf("create partial copy root: %v", err)
	}
	if err := db.CreateFileCopyOperation(context.Background(), &model.FileCopyOperation{
		ID:         "copy-operation",
		SourceID:   "source-folder",
		RootFileID: root.ID,
		State:      model.FileCopyOperationStateRunning,
	}); err != nil {
		t.Fatalf("create interrupted Folder Copy operation: %v", err)
	}

	reconciler := NewReconciler(cfg, db, nil, files)
	for attempt := 1; attempt <= 2; attempt++ {
		if err := reconciler.RecoverOnBoot(context.Background()); err != nil {
			t.Fatalf("RecoverOnBoot attempt %d: %v", attempt, err)
		}
		if _, err := files.Get(context.Background(), root.ID); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("partial copy root after attempt %d = %v, want not found", attempt, err)
		}
	}
	operation, err := db.GetFileCopyOperation(context.Background(), "copy-operation")
	if err != nil {
		t.Fatalf("get recovered Folder Copy operation: %v", err)
	}
	if operation.State != model.FileCopyOperationStateFailed || operation.Error == "" {
		t.Fatalf("recovered Folder Copy operation = %#v, want failed with reason", operation)
	}
}

func TestReconcilerRecoverOnBootFailsInterruptedMergingUploads(t *testing.T) {
	cfg, db := newReconcilerTestStore(t)
	session := &model.UploadSession{
		ID:              "interrupted-upload",
		FileName:        "report.txt",
		RequestedName:   "report.txt",
		ResolvedName:    "report.txt",
		OverwritePolicy: string(ConflictReject),
		FileSize:        3,
		ChunkSize:       3,
		UploadedChunks:  []int{0},
		DestPath:        "/",
		Status:          model.UploadStatusMerging,
		ExpiresAt:       time.Now().Add(time.Hour),
	}
	if err := db.CreateUploadSession(context.Background(), session); err != nil {
		t.Fatalf("create interrupted Upload Session: %v", err)
	}

	reconciler := NewReconciler(cfg, db, nil, NewFileService(cfg, db, nil))
	if err := reconciler.RecoverOnBoot(context.Background()); err != nil {
		t.Fatalf("RecoverOnBoot returned error: %v", err)
	}
	recovered, err := db.GetUploadSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("get recovered Upload Session: %v", err)
	}
	if recovered.Status != model.UploadStatusFailed {
		t.Fatalf("expected interrupted Upload Session to be failed, got %q", recovered.Status)
	}
}

func TestReconcilerSweepThumbnailsRemovesOrphans(t *testing.T) {
	cfg, db := newReconcilerTestStore(t)
	reconciler := NewReconciler(cfg, db, nil, NewFileService(cfg, db, nil))
	orphan := filepath.Join(cfg.Storage.ThumbnailDir, "missing-file.jpg")
	if err := writeSmallFile(orphan); err != nil {
		t.Fatalf("write thumbnail: %v", err)
	}

	removed, err := reconciler.SweepThumbnails(context.Background())
	if err != nil {
		t.Fatalf("SweepThumbnails returned error: %v", err)
	}
	if removed != 1 {
		t.Fatalf("expected one thumbnail removed, got %d", removed)
	}
	if exists(orphan) {
		t.Fatal("expected orphan thumbnail to be removed")
	}
}

func TestReconcilerSweepWebDAVTempRemovesExpiredFiles(t *testing.T) {
	cfg, db := newReconcilerTestStore(t)
	cfg.Storage.UploadTTL = time.Hour
	reconciler := NewReconciler(cfg, db, nil, NewFileService(cfg, db, nil))
	webDAVTempDir := filepath.Join(cfg.Storage.TempDir, "webdav")
	if err := os.MkdirAll(webDAVTempDir, 0o755); err != nil {
		t.Fatalf("create webdav temp dir: %v", err)
	}
	expired := filepath.Join(webDAVTempDir, "expired.upload")
	if err := writeSmallFile(expired); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(expired, old, old); err != nil {
		t.Fatalf("age temp file: %v", err)
	}

	removed, err := reconciler.SweepWebDAVTemp(context.Background())
	if err != nil {
		t.Fatalf("SweepWebDAVTemp returned error: %v", err)
	}
	if removed != 1 {
		t.Fatalf("expected one expired WebDAV temp file removed, got %d", removed)
	}
	if exists(expired) {
		t.Fatal("expected expired WebDAV temp file to be removed")
	}
}

func TestReconcilerSweepWebDAVTempKeepsUnexpiredFiles(t *testing.T) {
	cfg, db := newReconcilerTestStore(t)
	cfg.Storage.UploadTTL = time.Hour
	reconciler := NewReconciler(cfg, db, nil, NewFileService(cfg, db, nil))
	webDAVTempDir := filepath.Join(cfg.Storage.TempDir, "webdav")
	if err := os.MkdirAll(webDAVTempDir, 0o755); err != nil {
		t.Fatalf("create webdav temp dir: %v", err)
	}
	active := filepath.Join(webDAVTempDir, "active.upload")
	if err := writeSmallFile(active); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	recent := time.Now().Add(-5 * time.Minute)
	if err := os.Chtimes(active, recent, recent); err != nil {
		t.Fatalf("age temp file: %v", err)
	}

	removed, err := reconciler.SweepWebDAVTemp(context.Background())
	if err != nil {
		t.Fatalf("SweepWebDAVTemp returned error: %v", err)
	}
	if removed != 0 {
		t.Fatalf("expected no active WebDAV temp files removed, got %d", removed)
	}
	if !exists(active) {
		t.Fatal("expected active WebDAV temp file to remain")
	}
}

func TestReconcilerSweepStorageNeverMovesFileMutationStaging(t *testing.T) {
	cfg, db := newReconcilerTestStore(t)
	reconciler := NewReconciler(cfg, db, nil, NewFileService(cfg, db, nil))
	staged := filepath.Join(cfg.Storage.Root, ".staging", "active-mutation", "content")
	if err := os.MkdirAll(filepath.Dir(staged), 0o755); err != nil {
		t.Fatalf("create active mutation staging: %v", err)
	}
	if err := os.WriteFile(staged, []byte("new"), 0o644); err != nil {
		t.Fatalf("write active mutation content: %v", err)
	}

	moved, err := reconciler.SweepStorage(context.Background())
	if err != nil {
		t.Fatalf("SweepStorage returned error: %v", err)
	}
	if moved != 0 {
		t.Fatalf("expected no staged files moved, got %d", moved)
	}
	if !exists(staged) {
		t.Fatal("expected active File Mutation staging to remain")
	}
}

func TestReconcilerSweepStorageKeepsRegisteredFileVersions(t *testing.T) {
	files, db, root := newFileServiceTestHarness(t)
	files.cfg.FileVersion.Enabled = true
	file := createServiceTestFile(t, db, root, &model.File{
		ID:          "storage-sweep-version",
		Name:        "sweep.md",
		Path:        "/",
		StoragePath: "sweep.md",
		Size:        3,
		MimeType:    "text/markdown",
		Status:      model.FileStatusReady,
	}, "old")
	base, err := files.MarkdownContent(context.Background(), file.ID)
	if err != nil {
		t.Fatalf("read Markdown before save: %v", err)
	}
	if _, err := files.UpdateMarkdownContent(context.Background(), file.ID, "new", base.UpdatedAt); err != nil {
		t.Fatalf("create File Version: %v", err)
	}
	versions, err := files.ListVersions(context.Background(), file.ID)
	if err != nil || len(versions) != 1 {
		t.Fatalf("list File Versions: versions=%#v err=%v", versions, err)
	}
	if _, err := NewReconciler(files.cfg, db, nil, files).SweepStorage(context.Background()); err != nil {
		t.Fatalf("sweep storage: %v", err)
	}
	_, path, err := files.VersionDownload(context.Background(), file.ID, versions[0].ID)
	if err != nil {
		t.Fatalf("download registered File Version after storage sweep: %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read registered File Version after storage sweep: %v", err)
	}
	if string(content) != "old" {
		t.Fatalf("registered File Version content after storage sweep = %q, want old", content)
	}
}

func TestReconcilerPeriodicSweepLogsWebDAVTempFailureAndContinues(t *testing.T) {
	cfg, db := newReconcilerTestStore(t)
	cfg.Storage.UploadTTL = time.Hour
	cfg.Trash.RetentionDays = 0
	files := NewFileService(cfg, db, nil)
	reconciler := NewReconciler(cfg, db, nil, files)
	brokenWebDAVTemp := filepath.Join(cfg.Storage.TempDir, "webdav")
	if err := writeSmallFile(brokenWebDAVTemp); err != nil {
		t.Fatalf("write broken webdav temp path: %v", err)
	}
	file := &model.File{
		ID:          "trash-after-webdav-temp-failure",
		Name:        "expired.txt",
		Path:        "/",
		StoragePath: "expired.txt",
		MimeType:    "text/plain",
		Status:      model.FileStatusReady,
	}
	if err := writeSmallFile(filepath.Join(cfg.Storage.Root, file.StoragePath)); err != nil {
		t.Fatalf("write storage file: %v", err)
	}
	if err := db.CreateFile(context.Background(), file); err != nil {
		t.Fatalf("create file: %v", err)
	}
	if err := files.SoftDelete(context.Background(), file.ID); err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	var logs bytes.Buffer
	previousOutput := log.Writer()
	log.SetOutput(&logs)
	t.Cleanup(func() {
		log.SetOutput(previousOutput)
	})

	if err := reconciler.PeriodicSweep(context.Background()); err != nil {
		t.Fatalf("PeriodicSweep returned error: %v", err)
	}
	if !strings.Contains(logs.String(), "event=webdav_temp_sweep_failed") {
		t.Fatalf("expected WebDAV temp sweep failure log, got %q", logs.String())
	}
	if _, err := db.GetFileIncludeDeleted(context.Background(), file.ID); err == nil {
		t.Fatal("expected trash sweep to continue and purge expired file")
	}
}

func TestReconcilerSweepTrashPurgesExpiredItems(t *testing.T) {
	cfg, db := newReconcilerTestStore(t)
	cfg.Trash.AutoPurgeEnabled = true
	cfg.Trash.RetentionDays = 0
	files := NewFileService(cfg, db, nil)
	reconciler := NewReconciler(cfg, db, nil, files)
	file := &model.File{
		ID:          "trash-file",
		Name:        "expired.txt",
		Path:        "/",
		StoragePath: "expired.txt",
		MimeType:    "text/plain",
		Status:      model.FileStatusReady,
	}
	if err := writeSmallFile(filepath.Join(cfg.Storage.Root, file.StoragePath)); err != nil {
		t.Fatalf("write storage file: %v", err)
	}
	if err := db.CreateFile(context.Background(), file); err != nil {
		t.Fatalf("create file: %v", err)
	}
	if err := files.SoftDelete(context.Background(), file.ID); err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	purged, err := reconciler.SweepTrash(context.Background())
	if err != nil {
		t.Fatalf("SweepTrash returned error: %v", err)
	}
	if purged != 1 {
		t.Fatalf("expected one trash item purged, got %d", purged)
	}
	if exists(filepath.Join(cfg.Storage.Root, file.StoragePath)) {
		t.Fatal("expected storage file to be removed")
	}
	if _, err := db.GetFileIncludeDeleted(context.Background(), file.ID); err == nil {
		t.Fatal("expected DB row to be purged")
	}
}

func TestReconcilerSweepsFileVersionsBeyondCountLimit(t *testing.T) {
	files, db, root := newFileServiceTestHarness(t)
	files.cfg.FileVersion.Enabled = true
	files.cfg.FileVersion.MaxCount = 2
	files.cfg.FileVersion.RetentionDays = 90
	file := createServiceTestFile(t, db, root, &model.File{
		ID:          "retained-note",
		Name:        "retained.md",
		Path:        "/",
		StoragePath: "retained.md",
		Size:        2,
		MimeType:    "text/markdown",
		Status:      model.FileStatusReady,
	}, "v0")
	for _, content := range []string{"v1", "v2", "v3"} {
		base, err := files.MarkdownContent(context.Background(), file.ID)
		if err != nil {
			t.Fatalf("read Markdown before save %q: %v", content, err)
		}
		if _, err := files.UpdateMarkdownContent(context.Background(), file.ID, content, base.UpdatedAt); err != nil {
			t.Fatalf("save Markdown %q: %v", content, err)
		}
	}
	reconciler := NewReconciler(files.cfg, db, nil, files)
	if err := reconciler.PeriodicSweep(context.Background()); err != nil {
		t.Fatalf("sweep File Versions: %v", err)
	}
	versions, err := files.ListVersions(context.Background(), file.ID)
	if err != nil {
		t.Fatalf("list retained File Versions: %v", err)
	}
	if len(versions) != 2 || versions[0].VersionNo != 3 || versions[1].VersionNo != 2 {
		t.Fatalf("retained File Versions = %#v, want version numbers 3 and 2", versions)
	}
}

func TestReconcilerRetentionAlwaysKeepsNewestFileVersion(t *testing.T) {
	files, db, root := newFileServiceTestHarness(t)
	files.cfg.FileVersion.Enabled = true
	files.cfg.FileVersion.MaxCount = 20
	files.cfg.FileVersion.RetentionDays = 0
	file := createServiceTestFile(t, db, root, &model.File{
		ID:          "expiring-note",
		Name:        "expiring.md",
		Path:        "/",
		StoragePath: "expiring.md",
		Size:        2,
		MimeType:    "text/markdown",
		Status:      model.FileStatusReady,
	}, "v0")
	for _, content := range []string{"v1", "v2", "v3"} {
		base, err := files.MarkdownContent(context.Background(), file.ID)
		if err != nil {
			t.Fatalf("read Markdown before save %q: %v", content, err)
		}
		if _, err := files.UpdateMarkdownContent(context.Background(), file.ID, content, base.UpdatedAt); err != nil {
			t.Fatalf("save Markdown %q: %v", content, err)
		}
	}
	if err := NewReconciler(files.cfg, db, nil, files).PeriodicSweep(context.Background()); err != nil {
		t.Fatalf("sweep expired File Versions: %v", err)
	}
	versions, err := files.ListVersions(context.Background(), file.ID)
	if err != nil {
		t.Fatalf("list File Versions after retention: %v", err)
	}
	if len(versions) != 1 || versions[0].VersionNo != 3 {
		t.Fatalf("retained File Versions = %#v, want only version 3", versions)
	}
}

func newReconcilerTestStore(t *testing.T) (*config.Config, *store.Store) {
	t.Helper()
	root := t.TempDir()
	cfg := &config.Config{
		Storage: config.StorageConfig{
			Root:         filepath.Join(root, "files"),
			DBPath:       filepath.Join(root, "db", "memodrive.db"),
			TempDir:      filepath.Join(root, "tmp"),
			ThumbnailDir: filepath.Join(root, "thumbnails"),
		},
		Janitor: config.JanitorConfig{
			MaxTaskAge: time.Minute,
		},
		Trash: config.TrashConfig{
			AutoPurgeEnabled: true,
			RetentionDays:    30,
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
	return cfg, db
}

func writeSmallFile(path string) error {
	return os.WriteFile(path, []byte("x"), 0o644)
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
