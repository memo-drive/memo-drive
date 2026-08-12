package admin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestArchiveBackupRejectsArchiveInsideBackupDirectory(t *testing.T) {
	backupPath := filepath.Join(t.TempDir(), "backup")
	if err := os.MkdirAll(backupPath, 0o755); err != nil {
		t.Fatalf("create backup directory: %v", err)
	}
	archivePath := filepath.Join(backupPath, "backup.zip")
	if err := os.WriteFile(archivePath, []byte("existing"), 0o644); err != nil {
		t.Fatalf("create existing archive fixture: %v", err)
	}

	err := ArchiveBackup(backupPath, archivePath)
	if err == nil || !strings.Contains(err.Error(), "inside backup directory") {
		t.Fatalf("ArchiveBackup() error = %v, want archive-inside-backup rejection", err)
	}
}
