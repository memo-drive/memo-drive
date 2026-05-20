package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/memodrive/backend/internal/config"
	"github.com/memodrive/backend/internal/model"
)

func TestSearchFilesByName(t *testing.T) {
	db := newSearchTestStore(t)
	seedSearchFiles(t, db)

	hits, err := db.SearchFilesByName(context.Background(), FileSearchFilter{Keyword: "季报"})
	if err != nil {
		t.Fatalf("SearchFilesByName returned error: %v", err)
	}
	if len(hits) != 1 || hits[0].Name != "季报2024.pdf" {
		t.Fatalf("expected 季报 PDF hit, got %#v", hits)
	}
}

func TestNewFileHasEmptyLastViewedAt(t *testing.T) {
	db := newSearchTestStore(t)
	file := &model.File{
		ID:          "fresh-file",
		Name:        "fresh.pdf",
		Path:        "/",
		StoragePath: "fresh.pdf",
		Size:        128,
		MimeType:    "application/pdf",
		Status:      model.FileStatusReady,
	}
	if err := db.CreateFile(context.Background(), file); err != nil {
		t.Fatalf("create file: %v", err)
	}

	stored, err := db.GetFile(context.Background(), file.ID)
	if err != nil {
		t.Fatalf("get file: %v", err)
	}
	if stored.LastViewedAt != nil {
		t.Fatalf("expected new file last_viewed_at to be empty, got %s", stored.LastViewedAt.Format(time.RFC3339Nano))
	}
}

func TestExistingDatabaseMigratesLastViewedAt(t *testing.T) {
	dbPath := seedLegacyFileDatabaseWithoutLastViewedAt(t)
	db, err := Open(context.Background(), &config.Config{Storage: config.StorageConfig{DBPath: dbPath}})
	if err != nil {
		t.Fatalf("open migrated store: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	stored, err := db.GetFile(context.Background(), "legacy-file")
	if err != nil {
		t.Fatalf("get legacy file: %v", err)
	}
	if stored.LastViewedAt != nil {
		t.Fatalf("expected migrated legacy file last_viewed_at to be empty, got %s", stored.LastViewedAt.Format(time.RFC3339Nano))
	}

	viewedAt := time.Date(2026, 5, 19, 6, 30, 0, 0, time.UTC)
	if _, err := db.db.ExecContext(context.Background(), `UPDATE files SET last_viewed_at = ? WHERE id = ?`, viewedAt, stored.ID); err != nil {
		t.Fatalf("set last_viewed_at: %v", err)
	}
	stored, err = db.GetFile(context.Background(), stored.ID)
	if err != nil {
		t.Fatalf("get viewed legacy file: %v", err)
	}
	if stored.LastViewedAt == nil || !stored.LastViewedAt.Equal(viewedAt) {
		t.Fatalf("expected last_viewed_at %s, got %#v", viewedAt.Format(time.RFC3339Nano), stored.LastViewedAt)
	}
}

func TestListRecentlyViewedFilesFiltersSortsAndLimits(t *testing.T) {
	db := newSearchTestStore(t)
	files := []*model.File{
		{ID: "old", Name: "old.pdf", Path: "/", StoragePath: "old.pdf", Size: 1, MimeType: "application/pdf", Status: model.FileStatusReady},
		{ID: "new", Name: "new.pdf", Path: "/", StoragePath: "new.pdf", Size: 1, MimeType: "application/pdf", Status: model.FileStatusReady},
		{ID: "never-viewed", Name: "never.pdf", Path: "/", StoragePath: "never.pdf", Size: 1, MimeType: "application/pdf", Status: model.FileStatusReady},
		{ID: "folder", Name: "folder", Path: "/", StoragePath: "folder", IsDir: true, Status: model.FileStatusReady},
		{ID: "trashed", Name: "trashed.pdf", Path: "/", StoragePath: "trashed.pdf", Size: 1, MimeType: "application/pdf", Status: model.FileStatusReady},
	}
	for _, file := range files {
		if err := db.CreateFile(context.Background(), file); err != nil {
			t.Fatalf("create file %s: %v", file.ID, err)
		}
	}
	base := time.Date(2026, 5, 19, 7, 0, 0, 0, time.UTC)
	for _, item := range []struct {
		id       string
		viewedAt time.Time
	}{
		{id: "old", viewedAt: base},
		{id: "new", viewedAt: base.Add(time.Minute)},
		{id: "folder", viewedAt: base.Add(2 * time.Minute)},
		{id: "trashed", viewedAt: base.Add(3 * time.Minute)},
	} {
		if _, err := db.db.ExecContext(context.Background(), `UPDATE files SET last_viewed_at = ? WHERE id = ?`, item.viewedAt, item.id); err != nil {
			t.Fatalf("set viewed time for %s: %v", item.id, err)
		}
	}
	if err := db.SoftDeleteFile(context.Background(), "trashed", "trashed"); err != nil {
		t.Fatalf("soft delete trashed: %v", err)
	}

	recent, err := db.ListRecentlyViewedFiles(context.Background(), 2)
	if err != nil {
		t.Fatalf("ListRecentlyViewedFiles returned error: %v", err)
	}
	if len(recent) != 2 || recent[0].ID != "new" || recent[1].ID != "old" {
		t.Fatalf("expected recent files [new old], got %#v", recent)
	}
	if recent[0].LastViewedAt == nil || !recent[0].LastViewedAt.Equal(base.Add(time.Minute)) {
		t.Fatalf("expected new last_viewed_at to be loaded, got %#v", recent[0].LastViewedAt)
	}
}

func TestListRecentlyViewedFilesNormalizesLimit(t *testing.T) {
	db := newSearchTestStore(t)
	base := time.Date(2026, 5, 19, 8, 0, 0, 0, time.UTC)
	for i := 0; i < 12; i++ {
		id := fmt.Sprintf("viewed-%02d", i)
		if err := db.CreateFile(context.Background(), &model.File{
			ID:          id,
			Name:        id + ".pdf",
			Path:        "/",
			StoragePath: id + ".pdf",
			Size:        1,
			MimeType:    "application/pdf",
			Status:      model.FileStatusReady,
		}); err != nil {
			t.Fatalf("create file %s: %v", id, err)
		}
		if _, err := db.db.ExecContext(context.Background(), `UPDATE files SET last_viewed_at = ? WHERE id = ?`, base.Add(time.Duration(i)*time.Minute), id); err != nil {
			t.Fatalf("set viewed time for %s: %v", id, err)
		}
	}

	recent, err := db.ListRecentlyViewedFiles(context.Background(), 0)
	if err != nil {
		t.Fatalf("ListRecentlyViewedFiles default limit returned error: %v", err)
	}
	if len(recent) != 10 {
		t.Fatalf("expected default limit 10, got %d", len(recent))
	}

	recent, err = db.ListRecentlyViewedFiles(context.Background(), 1000)
	if err != nil {
		t.Fatalf("ListRecentlyViewedFiles max limit returned error: %v", err)
	}
	if len(recent) != 12 {
		t.Fatalf("expected max limit to allow all 12 seeded files, got %d", len(recent))
	}
}

func TestSearchFilesByMetadata(t *testing.T) {
	db := newSearchTestStore(t)
	seedSearchFiles(t, db)

	hits, err := db.SearchFilesByMetadata(context.Background(), FileSearchFilter{Keyword: "A7M3"})
	if err != nil {
		t.Fatalf("SearchFilesByMetadata returned error: %v", err)
	}
	if len(hits) != 1 || hits[0].File.Name != "图片A.jpg" {
		t.Fatalf("expected image metadata hit, got %#v", hits)
	}
	if hits[0].Snippet == "" || !strings.Contains(hits[0].Snippet, "A7M3") {
		t.Fatalf("expected snippet to include A7M3, got %q", hits[0].Snippet)
	}
}

func TestSearchFilesByNameFiltersMimeAndPath(t *testing.T) {
	db := newSearchTestStore(t)
	seedSearchFiles(t, db)

	hits, err := db.SearchFilesByName(context.Background(), FileSearchFilter{Keyword: "店", MimePrefix: "text/", PathPrefix: "/Notes"})
	if err != nil {
		t.Fatalf("SearchFilesByName returned error: %v", err)
	}
	if len(hits) != 1 || hits[0].Name != "奶茶店清单.md" {
		t.Fatalf("expected markdown hit, got %#v", hits)
	}

	hits, err = db.SearchFilesByName(context.Background(), FileSearchFilter{Keyword: "店", MimePrefix: "image/", PathPrefix: "/Notes"})
	if err != nil {
		t.Fatalf("SearchFilesByName returned error: %v", err)
	}
	if len(hits) != 0 {
		t.Fatalf("expected mime filter to exclude markdown, got %#v", hits)
	}
}

func TestSearchFilesByNameFiltersDateRange(t *testing.T) {
	db := newSearchTestStore(t)
	seedSearchFiles(t, db)
	now := time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)
	if _, err := db.db.ExecContext(context.Background(), `UPDATE files SET created_at = ? WHERE id = ?`, now.Add(-2*24*time.Hour), "report"); err != nil {
		t.Fatalf("backdate report: %v", err)
	}
	if _, err := db.db.ExecContext(context.Background(), `UPDATE files SET created_at = ? WHERE id = ?`, now.Add(-20*24*time.Hour), "milk-tea"); err != nil {
		t.Fatalf("backdate milk tea: %v", err)
	}

	from := now.Add(-7 * 24 * time.Hour)
	hits, err := db.SearchFilesByName(context.Background(), FileSearchFilter{Keyword: "季报", DateFrom: &from})
	if err != nil {
		t.Fatalf("SearchFilesByName returned error: %v", err)
	}
	if len(hits) != 1 || hits[0].ID != "report" {
		t.Fatalf("expected recent report hit, got %#v", hits)
	}

	to := now.Add(-7 * 24 * time.Hour)
	hits, err = db.SearchFilesByName(context.Background(), FileSearchFilter{Keyword: "季报", DateTo: &to})
	if err != nil {
		t.Fatalf("SearchFilesByName returned error: %v", err)
	}
	if len(hits) != 0 {
		t.Fatalf("expected date_to to exclude recent report, got %#v", hits)
	}
}

func TestListFileIDsByFilterFiltersTypeExtensionAndDate(t *testing.T) {
	db := newSearchTestStore(t)
	seedSearchFiles(t, db)
	now := time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)
	if _, err := db.db.ExecContext(context.Background(), `UPDATE files SET created_at = ? WHERE id = ?`, now.Add(-2*24*time.Hour), "report"); err != nil {
		t.Fatalf("backdate report: %v", err)
	}
	if _, err := db.db.ExecContext(context.Background(), `UPDATE files SET created_at = ? WHERE id = ?`, now.Add(-20*24*time.Hour), "image-a"); err != nil {
		t.Fatalf("backdate image: %v", err)
	}

	from := now.Add(-7 * 24 * time.Hour)
	ids, err := db.ListFileIDsByFilter(context.Background(), FileSearchFilter{
		MimePrefix: "application/pdf",
		Extensions: []string{".pdf"},
		DateFrom:   &from,
		Limit:      10,
	})
	if err != nil {
		t.Fatalf("ListFileIDsByFilter returned error: %v", err)
	}
	if len(ids) != 1 || ids[0] != "report" {
		t.Fatalf("expected only report, got %#v", ids)
	}

	ids, err = db.ListFileIDsByFilter(context.Background(), FileSearchFilter{
		MimePrefix: "image/",
		DateFrom:   &from,
		Limit:      10,
	})
	if err != nil {
		t.Fatalf("ListFileIDsByFilter returned error: %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("expected old image to be excluded, got %#v", ids)
	}
}

func TestTotalActiveFileSizeExcludesDirectoriesAndTrash(t *testing.T) {
	db := newSearchTestStore(t)
	seedDescendantFiles(t, db)
	if err := db.SoftDeleteFile(context.Background(), "bar", "bar"); err != nil {
		t.Fatalf("soft delete bar: %v", err)
	}

	total, err := db.TotalActiveFileSize(context.Background())
	if err != nil {
		t.Fatalf("TotalActiveFileSize returned error: %v", err)
	}
	if total != 10 {
		t.Fatalf("expected only active non-directory bytes, got %d", total)
	}
}

func TestListDescendants(t *testing.T) {
	db := newSearchTestStore(t)
	seedDescendantFiles(t, db)

	files, err := db.ListDescendants(context.Background(), "/A/B")
	if err != nil {
		t.Fatalf("ListDescendants returned error: %v", err)
	}
	if len(files) != 3 {
		t.Fatalf("expected 3 descendants, got %d: %#v", len(files), files)
	}
	got := []string{files[0].ID, files[1].ID, files[2].ID}
	want := []string{"foo", "sub", "bar"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected descendant order %v, got %v", want, got)
		}
	}
}

func TestExistsAtPath(t *testing.T) {
	db := newSearchTestStore(t)
	seedDescendantFiles(t, db)

	exists, err := db.ExistsAtPath(context.Background(), "/A", "b")
	if err != nil {
		t.Fatalf("ExistsAtPath returned error: %v", err)
	}
	if !exists {
		t.Fatal("expected /A/B to exist case-insensitively")
	}
	exists, err = db.ExistsAtPath(context.Background(), "/A", "C")
	if err != nil {
		t.Fatalf("ExistsAtPath returned error: %v", err)
	}
	if exists {
		t.Fatal("expected /A/C to be absent")
	}
}

func TestTrashStoreLifecycle(t *testing.T) {
	db := newSearchTestStore(t)
	file := &model.File{
		ID:          "trash-me",
		Name:        "note.md",
		Path:        "/Notes",
		StoragePath: "Notes/note.md",
		Size:        12,
		MimeType:    "text/markdown",
		Status:      model.FileStatusReady,
	}
	if err := db.CreateFile(context.Background(), file); err != nil {
		t.Fatalf("create file: %v", err)
	}
	if err := db.SoftDeleteFile(context.Background(), file.ID, file.ID); err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	if _, err := db.GetFile(context.Background(), file.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected active GetFile to return ErrNotFound, got %v", err)
	}
	trashed, err := db.ListTrashed(context.Background(), 10)
	if err != nil {
		t.Fatalf("list trashed: %v", err)
	}
	if len(trashed) != 1 || trashed[0].OriginalPath == nil || *trashed[0].OriginalPath != "/Notes" || trashed[0].OriginalName == nil || *trashed[0].OriginalName != "note.md" {
		t.Fatalf("unexpected trashed file: %#v", trashed)
	}
	if trashed[0].TrashRootID == nil || *trashed[0].TrashRootID != file.ID {
		t.Fatalf("expected trash root id to be set to file id, got %#v", trashed[0])
	}
	if trashed[0].Path != "/.trash" || trashed[0].DeletedAt == nil {
		t.Fatalf("expected trash placeholder path and deleted_at, got %#v", trashed[0])
	}

	if err := db.RestoreFile(context.Background(), file.ID, "/Notes", "note.md"); err != nil {
		t.Fatalf("restore: %v", err)
	}
	restored, err := db.GetFile(context.Background(), file.ID)
	if err != nil {
		t.Fatalf("get restored: %v", err)
	}
	if restored.DeletedAt != nil || restored.OriginalPath != nil || restored.OriginalName != nil || restored.TrashRootID != nil {
		t.Fatalf("expected restore to clear trash fields, got %#v", restored)
	}
}

func TestListTrashedReturnsOnlyTrashRoots(t *testing.T) {
	db := newSearchTestStore(t)
	seedDescendantFiles(t, db)
	for _, item := range []struct {
		id   string
		root string
	}{
		{id: "foo", root: "dir-b"},
		{id: "sub", root: "dir-b"},
		{id: "bar", root: "dir-b"},
		{id: "dir-b", root: "dir-b"},
	} {
		if err := db.SoftDeleteFile(context.Background(), item.id, item.root); err != nil {
			t.Fatalf("soft delete %s: %v", item.id, err)
		}
	}

	trashed, err := db.ListTrashed(context.Background(), 10)
	if err != nil {
		t.Fatalf("list trashed: %v", err)
	}
	if len(trashed) != 1 || trashed[0].ID != "dir-b" {
		t.Fatalf("expected only trash root dir-b, got %#v", trashed)
	}
}

func TestListExpiredTrashed(t *testing.T) {
	db := newSearchTestStore(t)
	oldFile := &model.File{ID: "old-trash", Name: "old.txt", Path: "/", StoragePath: "old.txt", MimeType: "text/plain", Status: model.FileStatusReady}
	newFile := &model.File{ID: "new-trash", Name: "new.txt", Path: "/", StoragePath: "new.txt", MimeType: "text/plain", Status: model.FileStatusReady}
	for _, file := range []*model.File{oldFile, newFile} {
		if err := db.CreateFile(context.Background(), file); err != nil {
			t.Fatalf("create file %s: %v", file.ID, err)
		}
		if err := db.SoftDeleteFile(context.Background(), file.ID, file.ID); err != nil {
			t.Fatalf("soft delete %s: %v", file.ID, err)
		}
	}
	oldDeletedAt := time.Now().Add(-31 * 24 * time.Hour)
	if _, err := db.db.ExecContext(context.Background(), `UPDATE files SET deleted_at = ? WHERE id = ?`, oldDeletedAt, oldFile.ID); err != nil {
		t.Fatalf("backdate deleted_at: %v", err)
	}

	expired, err := db.ListExpiredTrashed(context.Background(), time.Now().Add(-30*24*time.Hour))
	if err != nil {
		t.Fatalf("list expired: %v", err)
	}
	if len(expired) != 1 || expired[0].ID != oldFile.ID {
		t.Fatalf("expected only old trash item, got %#v", expired)
	}
}

func TestSoftDeletedFilesAreExcludedFromSearchAndConflicts(t *testing.T) {
	db := newSearchTestStore(t)
	seedSearchFiles(t, db)
	if err := db.SoftDeleteFile(context.Background(), "milk-tea", "milk-tea"); err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	nameHits, err := db.SearchFilesByName(context.Background(), FileSearchFilter{Keyword: "奶茶"})
	if err != nil {
		t.Fatalf("search by name: %v", err)
	}
	if len(nameHits) != 0 {
		t.Fatalf("expected trashed file to be excluded from name search, got %#v", nameHits)
	}

	exists, err := db.ExistsAtPath(context.Background(), "/Notes", "奶茶店清单.md")
	if err != nil {
		t.Fatalf("exists at path: %v", err)
	}
	if exists {
		t.Fatal("expected trashed file not to block same-name restore/create conflict checks")
	}
}

func newSearchTestStore(t *testing.T) *Store {
	t.Helper()
	root := t.TempDir()
	dbPath := filepath.Join(root, "db", "memodrive.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatalf("mkdir db dir: %v", err)
	}
	db, err := Open(context.Background(), &config.Config{Storage: config.StorageConfig{DBPath: dbPath}})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	return db
}

func seedSearchFiles(t *testing.T, db *Store) {
	t.Helper()
	files := []*model.File{
		{ID: "image-a", Name: "图片A.jpg", Path: "/Photos", StoragePath: "image-a.jpg", Size: 100, MimeType: "image/jpeg", Status: model.FileStatusReady},
		{ID: "report", Name: "季报2024.pdf", Path: "/", StoragePath: "report.pdf", Size: 200, MimeType: "application/pdf", Status: model.FileStatusReady},
		{ID: "milk-tea", Name: "奶茶店清单.md", Path: "/Notes", StoragePath: "milk-tea.md", Size: 300, MimeType: "text/markdown", Status: model.FileStatusReady},
	}
	for _, file := range files {
		if err := db.CreateFile(context.Background(), file); err != nil {
			t.Fatalf("create file %s: %v", file.ID, err)
		}
	}
	if err := db.UpsertMetadata(context.Background(), &model.FileMetadata{
		FileID:   "image-a",
		MetaJSON: `{"camera":"Sony A7M3","format":"JPEG"}`,
	}); err != nil {
		t.Fatalf("upsert metadata: %v", err)
	}
}

func seedDescendantFiles(t *testing.T, db *Store) {
	t.Helper()
	files := []*model.File{
		{ID: "dir-b", Name: "B", Path: "/A", StoragePath: "A/B", IsDir: true, Status: model.FileStatusReady},
		{ID: "foo", Name: "foo.txt", Path: "/A/B", StoragePath: "A/B/foo.txt", Size: 10, MimeType: "text/plain", Status: model.FileStatusReady},
		{ID: "sub", Name: "sub", Path: "/A/B", StoragePath: "A/B/sub", IsDir: true, Status: model.FileStatusReady},
		{ID: "bar", Name: "bar.txt", Path: "/A/B/sub", StoragePath: "A/B/sub/bar.txt", Size: 20, MimeType: "text/plain", Status: model.FileStatusReady},
	}
	for _, file := range files {
		if err := db.CreateFile(context.Background(), file); err != nil {
			t.Fatalf("create file %s: %v", file.ID, err)
		}
	}
}

func seedLegacyFileDatabaseWithoutLastViewedAt(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	dbPath := filepath.Join(root, "db", "memodrive.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatalf("mkdir db dir: %v", err)
	}
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open legacy db: %v", err)
	}
	defer db.Close()
	if _, err := db.ExecContext(context.Background(), `
CREATE TABLE files (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    path TEXT NOT NULL,
    storage_path TEXT NOT NULL,
    size INTEGER,
    mime_type TEXT,
    is_dir BOOLEAN DEFAULT FALSE,
    parent_id TEXT,
    status TEXT DEFAULT 'uploaded',
    chunk_count INTEGER DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    deleted_at DATETIME,
    original_path TEXT,
    original_name TEXT,
    trash_root_id TEXT
);
CREATE TABLE schema_migrations (
    id TEXT PRIMARY KEY,
    applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
INSERT INTO schema_migrations(id) VALUES
    ('010_trash_columns'),
    ('011_trash_root_id'),
    ('012_conversation_columns'),
    ('013_chunks_fts');
INSERT INTO files (id, name, path, storage_path, size, mime_type, is_dir, status, chunk_count, created_at, updated_at)
VALUES ('legacy-file', 'legacy.pdf', '/', 'legacy.pdf', 256, 'application/pdf', 0, 'ready', 0, '2026-05-18 10:00:00', '2026-05-18 10:00:00');
`); err != nil {
		t.Fatalf("seed legacy db: %v", err)
	}
	return dbPath
}
