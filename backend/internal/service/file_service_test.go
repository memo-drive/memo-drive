package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/memodrive/backend/internal/config"
	"github.com/memodrive/backend/internal/model"
	"github.com/memodrive/backend/internal/store"
	"github.com/memodrive/backend/internal/vectordb"
)

func TestRenameMoveRenamesSingleFile(t *testing.T) {
	service, db, root := newFileServiceTestHarness(t)
	file := createServiceTestFile(t, db, root, &model.File{
		ID:          "file-1",
		Name:        "foo.txt",
		Path:        "/",
		StoragePath: "foo.txt",
		Size:        3,
		MimeType:    "text/plain",
		Status:      model.FileStatusReady,
	}, "foo")

	renamed, err := service.RenameMove(context.Background(), file.ID, "bar.txt", "")
	if err != nil {
		t.Fatalf("RenameMove returned error: %v", err)
	}
	if renamed.Name != "bar.txt" || renamed.Path != "/" {
		t.Fatalf("unexpected renamed file: %#v", renamed)
	}
	if renamed.StoragePath != "foo.txt" {
		t.Fatalf("expected storage path to keep stable file basename, got %q", renamed.StoragePath)
	}
	if _, err := os.Stat(filepath.Join(root, "foo.txt")); err != nil {
		t.Fatalf("expected physical file to remain available: %v", err)
	}
}

func TestRenameMoveRenamesDirectoryRecursively(t *testing.T) {
	service, db, root := newFileServiceTestHarness(t)
	seedServiceDirectoryTree(t, db, root)

	renamed, err := service.RenameMove(context.Background(), "dir-b", "X", "")
	if err != nil {
		t.Fatalf("RenameMove returned error: %v", err)
	}
	if renamed.Name != "X" || renamed.Path != "/A" || renamed.StoragePath != "A/X" {
		t.Fatalf("unexpected renamed directory: %#v", renamed)
	}
	assertServiceTestFileLocation(t, db, "foo", "/A/X", "A/X/foo.txt")
	assertServiceTestFileLocation(t, db, "sub", "/A/X", "A/X/sub")
	assertServiceTestFileLocation(t, db, "bar", "/A/X/sub", "A/X/sub/bar.txt")
	if _, err := os.Stat(filepath.Join(root, "A", "X", "sub", "bar.txt")); err != nil {
		t.Fatalf("expected physical descendant after directory rename: %v", err)
	}
}

func TestRenameMoveRejectsTargetConflict(t *testing.T) {
	service, db, root := newFileServiceTestHarness(t)
	createServiceTestFile(t, db, root, &model.File{
		ID:          "src",
		Name:        "foo.txt",
		Path:        "/A",
		StoragePath: "A/foo.txt",
		Size:        3,
		MimeType:    "text/plain",
		Status:      model.FileStatusReady,
	}, "src")
	createServiceTestFile(t, db, root, &model.File{
		ID:          "dst",
		Name:        "foo.txt",
		Path:        "/B",
		StoragePath: "B/foo.txt",
		Size:        3,
		MimeType:    "text/plain",
		Status:      model.FileStatusReady,
	}, "dst")

	_, err := service.RenameMove(context.Background(), "src", "", "/B")
	if !errors.Is(err, ErrPathConflict) {
		t.Fatalf("expected ErrPathConflict, got %v", err)
	}
}

func TestRenameMoveRejectsDirectoryIntoItself(t *testing.T) {
	service, db, root := newFileServiceTestHarness(t)
	createServiceTestFile(t, db, root, &model.File{
		ID:          "dir-a",
		Name:        "A",
		Path:        "/",
		StoragePath: "A",
		IsDir:       true,
		Status:      model.FileStatusReady,
	}, "")

	_, err := service.RenameMove(context.Background(), "dir-a", "", "/A/X")
	if !errors.Is(err, ErrPathConflict) {
		t.Fatalf("expected ErrPathConflict, got %v", err)
	}
}

func TestRenameMoveNoopReturnsWithoutUpdating(t *testing.T) {
	service, db, root := newFileServiceTestHarness(t)
	file := createServiceTestFile(t, db, root, &model.File{
		ID:          "file-1",
		Name:        "foo.txt",
		Path:        "/",
		StoragePath: "foo.txt",
		Size:        3,
		MimeType:    "text/plain",
		Status:      model.FileStatusReady,
	}, "foo")
	before, err := db.GetFile(context.Background(), file.ID)
	if err != nil {
		t.Fatalf("get before noop: %v", err)
	}

	after, err := service.RenameMove(context.Background(), file.ID, "foo.txt", "/")
	if err != nil {
		t.Fatalf("RenameMove returned error: %v", err)
	}
	stored, err := db.GetFile(context.Background(), file.ID)
	if err != nil {
		t.Fatalf("get after noop: %v", err)
	}
	if after.ID != file.ID || stored.UpdatedAt != before.UpdatedAt {
		t.Fatalf("expected noop without updated_at change, before=%s after=%s", before.UpdatedAt, stored.UpdatedAt)
	}
}

func TestSoftDeleteMovesFileToTrashWithoutRemovingPhysicalFile(t *testing.T) {
	service, db, root := newFileServiceTestHarness(t)
	file := createServiceTestFile(t, db, root, &model.File{
		ID:          "file-1",
		Name:        "foo.txt",
		Path:        "/Notes",
		StoragePath: "Notes/foo.txt",
		Size:        3,
		MimeType:    "text/plain",
		Status:      model.FileStatusReady,
	}, "foo")

	if err := service.SoftDelete(context.Background(), file.ID); err != nil {
		t.Fatalf("SoftDelete returned error: %v", err)
	}
	if _, err := db.GetFile(context.Background(), file.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected active get to miss soft-deleted file, got %v", err)
	}
	trashed, err := db.GetFileIncludeDeleted(context.Background(), file.ID)
	if err != nil {
		t.Fatalf("get include deleted: %v", err)
	}
	if trashed.DeletedAt == nil || trashed.Path != "/.trash" || trashed.OriginalPath == nil || *trashed.OriginalPath != "/Notes" || trashed.OriginalName == nil || *trashed.OriginalName != "foo.txt" {
		t.Fatalf("unexpected trashed file: %#v", trashed)
	}
	if _, err := os.Stat(filepath.Join(root, "Notes", "foo.txt")); err != nil {
		t.Fatalf("expected physical file to remain in place: %v", err)
	}
}

func TestSoftDeleteDirectoryRecursively(t *testing.T) {
	service, db, root := newFileServiceTestHarness(t)
	seedServiceDirectoryTree(t, db, root)

	if err := service.SoftDelete(context.Background(), "dir-b"); err != nil {
		t.Fatalf("SoftDelete returned error: %v", err)
	}
	active, err := db.ListFiles(context.Background(), "/A", "name")
	if err != nil {
		t.Fatalf("list active files: %v", err)
	}
	if len(active) != 0 {
		t.Fatalf("expected directory to disappear from active list, got %#v", active)
	}
	trashed, err := db.ListTrashed(context.Background(), 10)
	if err != nil {
		t.Fatalf("list trashed: %v", err)
	}
	if len(trashed) != 1 || trashed[0].ID != "dir-b" {
		t.Fatalf("expected only top-level deleted directory in trash list, got %#v", trashed)
	}
	descendants, err := db.ListTrashedDescendants(context.Background(), "/A/B")
	if err != nil {
		t.Fatalf("list trashed descendants: %v", err)
	}
	if len(descendants) != 3 {
		t.Fatalf("expected hidden trashed descendants to remain for restore/purge, got %d: %#v", len(descendants), descendants)
	}
}

func TestRestoreWithConflictUsesRestoredName(t *testing.T) {
	service, db, root := newFileServiceTestHarness(t)
	createServiceTestFile(t, db, root, &model.File{
		ID:          "old",
		Name:        "foo.txt",
		Path:        "/",
		StoragePath: "old-foo.txt",
		Size:        3,
		MimeType:    "text/plain",
		Status:      model.FileStatusReady,
	}, "old")
	if err := service.SoftDelete(context.Background(), "old"); err != nil {
		t.Fatalf("soft delete old: %v", err)
	}
	createServiceTestFile(t, db, root, &model.File{
		ID:          "new",
		Name:        "foo.txt",
		Path:        "/",
		StoragePath: "new-foo.txt",
		Size:        3,
		MimeType:    "text/plain",
		Status:      model.FileStatusReady,
	}, "new")

	restored, err := service.Restore(context.Background(), "old")
	if err != nil {
		t.Fatalf("Restore returned error: %v", err)
	}
	if restored.Name != "foo (restored).txt" || restored.Path != "/" {
		t.Fatalf("expected restored conflict name, got %#v", restored)
	}
}

func TestRestoreRejectsActiveFile(t *testing.T) {
	service, db, root := newFileServiceTestHarness(t)
	createServiceTestFile(t, db, root, &model.File{
		ID:          "active",
		Name:        "active.txt",
		Path:        "/",
		StoragePath: "active.txt",
		Size:        3,
		MimeType:    "text/plain",
		Status:      model.FileStatusReady,
	}, "active")

	_, err := service.Restore(context.Background(), "active")
	if !errors.Is(err, ErrNotInTrash) {
		t.Fatalf("expected ErrNotInTrash, got %v", err)
	}
	if _, err := db.GetFile(context.Background(), "active"); err != nil {
		t.Fatalf("expected active file to remain available: %v", err)
	}
}

func TestRestoreDirectoryRecursively(t *testing.T) {
	service, db, root := newFileServiceTestHarness(t)
	seedServiceDirectoryTree(t, db, root)
	if err := service.SoftDelete(context.Background(), "dir-b"); err != nil {
		t.Fatalf("soft delete directory: %v", err)
	}

	restored, err := service.Restore(context.Background(), "dir-b")
	if err != nil {
		t.Fatalf("Restore returned error: %v", err)
	}
	if restored.Name != "B" || restored.Path != "/A" {
		t.Fatalf("unexpected restored directory: %#v", restored)
	}
	assertServiceTestFileLocation(t, db, "dir-b", "/A", "A/B")
	assertServiceTestFileLocation(t, db, "foo", "/A/B", "A/B/foo.txt")
	assertServiceTestFileLocation(t, db, "sub", "/A/B", "A/B/sub")
	assertServiceTestFileLocation(t, db, "bar", "/A/B/sub", "A/B/sub/bar.txt")
	trashed, err := db.ListTrashed(context.Background(), 10)
	if err != nil {
		t.Fatalf("list trash: %v", err)
	}
	if len(trashed) != 0 {
		t.Fatalf("expected recursive restore to empty trash, got %#v", trashed)
	}
}

func TestRestoreDirectoryRecursivelyMapsConflictedRootName(t *testing.T) {
	service, db, root := newFileServiceTestHarness(t)
	seedServiceDirectoryTree(t, db, root)
	if err := service.SoftDelete(context.Background(), "dir-b"); err != nil {
		t.Fatalf("soft delete directory: %v", err)
	}
	createServiceTestFile(t, db, root, &model.File{
		ID:          "new-b",
		Name:        "B",
		Path:        "/A",
		StoragePath: "A/new-b",
		IsDir:       true,
		Status:      model.FileStatusReady,
	}, "")

	restored, err := service.Restore(context.Background(), "dir-b")
	if err != nil {
		t.Fatalf("Restore returned error: %v", err)
	}
	if restored.Name != "B (restored)" || restored.Path != "/A" {
		t.Fatalf("expected conflicted root restore name, got %#v", restored)
	}
	assertServiceTestFileLocation(t, db, "foo", "/A/B (restored)", "A/B/foo.txt")
	assertServiceTestFileLocation(t, db, "sub", "/A/B (restored)", "A/B/sub")
	assertServiceTestFileLocation(t, db, "bar", "/A/B (restored)/sub", "A/B/sub/bar.txt")
}

func TestPurgeRemovesSoftDeletedFileAndVectors(t *testing.T) {
	service, db, root := newFileServiceTestHarness(t)
	vectorDB := &recordingVectorStore{}
	service.vectorDB = vectorDB
	file := createServiceTestFile(t, db, root, &model.File{
		ID:          "file-1",
		Name:        "foo.txt",
		Path:        "/",
		StoragePath: "foo.txt",
		Size:        3,
		MimeType:    "text/plain",
		Status:      model.FileStatusReady,
		ChunkCount:  2,
	}, "foo")
	if err := service.SoftDelete(context.Background(), file.ID); err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	if err := service.Purge(context.Background(), file.ID); err != nil {
		t.Fatalf("Purge returned error: %v", err)
	}
	if _, err := db.GetFileIncludeDeleted(context.Background(), file.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected DB row to be purged, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "foo.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected physical file to be removed, got %v", err)
	}
	want := []string{"file-1#0", "file-1#1"}
	if len(vectorDB.deletedIDs) != len(want) {
		t.Fatalf("expected vector delete ids %v, got %v", want, vectorDB.deletedIDs)
	}
	for i := range want {
		if vectorDB.deletedIDs[i] != want[i] {
			t.Fatalf("expected vector delete ids %v, got %v", want, vectorDB.deletedIDs)
		}
	}
}

func TestPurgeDirectoryRemovesDescendantsChunksVectorsAndStorage(t *testing.T) {
	service, db, root := newFileServiceTestHarness(t)
	vectorDB := &recordingVectorStore{}
	service.vectorDB = vectorDB
	seedServiceDirectoryTree(t, db, root)
	if err := db.UpdateFileChunkCount(context.Background(), "foo", 2); err != nil {
		t.Fatalf("update foo chunk count: %v", err)
	}
	if err := db.UpdateFileChunkCount(context.Background(), "bar", 1); err != nil {
		t.Fatalf("update bar chunk count: %v", err)
	}
	if err := db.UpsertChunks(context.Background(), []store.ChunkRow{
		{ID: "foo#0", FileID: "foo", FileName: "foo.txt", ChunkIndex: 0, Text: "foo one"},
		{ID: "foo#1", FileID: "foo", FileName: "foo.txt", ChunkIndex: 1, Text: "foo two"},
		{ID: "bar#0", FileID: "bar", FileName: "bar.txt", ChunkIndex: 0, Text: "bar one"},
	}); err != nil {
		t.Fatalf("upsert chunks: %v", err)
	}
	if err := service.SoftDelete(context.Background(), "dir-b"); err != nil {
		t.Fatalf("soft delete directory: %v", err)
	}

	if err := service.Purge(context.Background(), "dir-b"); err != nil {
		t.Fatalf("Purge returned error: %v", err)
	}

	for _, id := range []string{"dir-b", "foo", "sub", "bar"} {
		if _, err := db.GetFileIncludeDeleted(context.Background(), id); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("expected file %s to be purged, got %v", id, err)
		}
	}
	for _, chunkID := range []string{"foo#0", "foo#1", "bar#0"} {
		if _, err := db.GetChunkText(context.Background(), chunkID); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("expected chunk %s to be purged, got %v", chunkID, err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "A", "B")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected physical directory tree to be removed, got %v", err)
	}
	assertDeletedVectorIDs(t, vectorDB.deletedIDs, []string{"foo#0", "foo#1", "bar#0"})
}

func TestPurgeRejectsActiveFile(t *testing.T) {
	service, db, root := newFileServiceTestHarness(t)
	createServiceTestFile(t, db, root, &model.File{
		ID:          "active",
		Name:        "active.txt",
		Path:        "/",
		StoragePath: "active.txt",
		Size:        3,
		MimeType:    "text/plain",
		Status:      model.FileStatusReady,
	}, "active")

	err := service.Purge(context.Background(), "active")
	if !errors.Is(err, ErrNotInTrash) {
		t.Fatalf("expected ErrNotInTrash, got %v", err)
	}
	if _, err := db.GetFile(context.Background(), "active"); err != nil {
		t.Fatalf("expected active file to remain: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "active.txt")); err != nil {
		t.Fatalf("expected physical file to remain: %v", err)
	}
}

func newFileServiceTestHarness(t *testing.T) (*FileService, *store.Store, string) {
	t.Helper()
	root := t.TempDir()
	storageRoot := filepath.Join(root, "files")
	dbPath := filepath.Join(root, "db", "memodrive.db")
	cfg := &config.Config{
		Storage: config.StorageConfig{
			Root:         storageRoot,
			DBPath:       dbPath,
			TempDir:      filepath.Join(root, "tmp"),
			ThumbnailDir: filepath.Join(root, "thumbs"),
		},
	}
	if err := cfg.EnsureDirs(); err != nil {
		t.Fatalf("ensure dirs: %v", err)
	}
	db, err := store.Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	return NewFileService(cfg, db, nil), db, storageRoot
}

func createServiceTestFile(t *testing.T, db *store.Store, root string, file *model.File, content string) *model.File {
	t.Helper()
	absPath := filepath.Join(root, filepath.FromSlash(file.StoragePath))
	if file.IsDir {
		if err := os.MkdirAll(absPath, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", file.StoragePath, err)
		}
	} else {
		if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
			t.Fatalf("mkdir parent %s: %v", file.StoragePath, err)
		}
		if err := os.WriteFile(absPath, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", file.StoragePath, err)
		}
	}
	if err := db.CreateFile(context.Background(), file); err != nil {
		t.Fatalf("create file %s: %v", file.ID, err)
	}
	return file
}

func seedServiceDirectoryTree(t *testing.T, db *store.Store, root string) {
	t.Helper()
	createServiceTestFile(t, db, root, &model.File{ID: "dir-b", Name: "B", Path: "/A", StoragePath: "A/B", IsDir: true, Status: model.FileStatusReady}, "")
	createServiceTestFile(t, db, root, &model.File{ID: "foo", Name: "foo.txt", Path: "/A/B", StoragePath: "A/B/foo.txt", Size: 10, MimeType: "text/plain", Status: model.FileStatusReady}, "foo")
	createServiceTestFile(t, db, root, &model.File{ID: "sub", Name: "sub", Path: "/A/B", StoragePath: "A/B/sub", IsDir: true, Status: model.FileStatusReady}, "")
	createServiceTestFile(t, db, root, &model.File{ID: "bar", Name: "bar.txt", Path: "/A/B/sub", StoragePath: "A/B/sub/bar.txt", Size: 20, MimeType: "text/plain", Status: model.FileStatusReady}, "bar")
}

func assertServiceTestFileLocation(t *testing.T, db *store.Store, id, wantPath, wantStorage string) {
	t.Helper()
	file, err := db.GetFile(context.Background(), id)
	if err != nil {
		t.Fatalf("get file %s: %v", id, err)
	}
	if file.Path != wantPath || file.StoragePath != wantStorage {
		t.Fatalf("file %s expected path/storage %q/%q, got %q/%q", id, wantPath, wantStorage, file.Path, file.StoragePath)
	}
}

type recordingVectorStore struct {
	deletedIDs []string
}

func (r *recordingVectorStore) EnsureCollection(context.Context, string) error {
	return nil
}

func (r *recordingVectorStore) Upsert(context.Context, string, []string, [][]float32, []string, []map[string]any) error {
	return nil
}

func (r *recordingVectorStore) Query(context.Context, string, []float32, int) (*vectordb.QueryResult, error) {
	return nil, nil
}

func (r *recordingVectorStore) Delete(_ context.Context, _ string, ids []string) error {
	r.deletedIDs = append(r.deletedIDs, ids...)
	return nil
}

func assertDeletedVectorIDs(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("expected vector delete ids %v, got %v", want, got)
	}
	seen := make(map[string]bool, len(got))
	for _, id := range got {
		seen[id] = true
	}
	for _, id := range want {
		if !seen[id] {
			t.Fatalf("expected vector delete ids %v, got %v", want, got)
		}
	}
}
