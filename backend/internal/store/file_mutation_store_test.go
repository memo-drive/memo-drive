package store

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/memodrive/backend/internal/config"
	"github.com/memodrive/backend/internal/model"
)

func TestFileMutationMigrationRecoversWhenSchemaExistsWithoutMarker(t *testing.T) {
	cfg := &config.Config{
		Storage: config.StorageConfig{
			DBPath: filepath.Join(t.TempDir(), "db", "memodrive.db"),
		},
	}
	if err := os.MkdirAll(filepath.Dir(cfg.Storage.DBPath), 0o755); err != nil {
		t.Fatalf("create database directory: %v", err)
	}
	first, err := Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("open initial store: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("close initial store: %v", err)
	}

	raw, err := sql.Open("sqlite3", cfg.Storage.DBPath)
	if err != nil {
		t.Fatalf("open migration database: %v", err)
	}
	if _, err := raw.Exec(`DELETE FROM schema_migrations WHERE id = '017_file_mutations'`); err != nil {
		_ = raw.Close()
		t.Fatalf("remove File Mutation migration marker: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("close migration database: %v", err)
	}

	reopened, err := Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("reopen partially migrated store: %v", err)
	}
	defer reopened.Close()
	mutation := &model.FileMutation{
		ID:               "migration-recovery",
		Kind:             model.FileMutationKindUploadCreate,
		State:            model.FileMutationStatePrepared,
		VirtualPath:      "/report.txt",
		StagedPath:       ".staging/migration-recovery/content",
		FinalStoragePath: "report.txt",
	}
	if err := reopened.CreateFileMutation(context.Background(), mutation); err != nil {
		t.Fatalf("create File Mutation after migration recovery: %v", err)
	}
	ids, err := reopened.ListFileMutationIDs(context.Background())
	if err != nil {
		t.Fatalf("list File Mutation IDs: %v", err)
	}
	if _, ok := ids[mutation.ID]; !ok {
		t.Fatalf("expected recovered journal to contain mutation %q", mutation.ID)
	}
}
