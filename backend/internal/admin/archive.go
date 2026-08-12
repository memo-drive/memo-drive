package admin

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ArchiveBackup writes an optional ZIP64-capable archive while preserving the
// directory backup as the canonical restore and verification format.
func ArchiveBackup(backupPath, archivePath string) error {
	backupPath, err := filepath.Abs(backupPath)
	if err != nil {
		return fmt.Errorf("resolve backup path: %w", err)
	}
	archivePath, err = filepath.Abs(archivePath)
	if err != nil {
		return fmt.Errorf("resolve archive path: %w", err)
	}
	if pathWithin(backupPath, archivePath) {
		return fmt.Errorf("backup archive must not be inside backup directory: %s", archivePath)
	}
	if _, err := os.Stat(archivePath); err == nil {
		return fmt.Errorf("backup archive already exists: %s", archivePath)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect backup archive: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(archivePath), 0o755); err != nil {
		return fmt.Errorf("create archive parent: %w", err)
	}
	resolvedBackup, err := filepath.EvalSymlinks(backupPath)
	if err != nil {
		return fmt.Errorf("resolve backup directory: %w", err)
	}
	resolvedArchiveParent, err := filepath.EvalSymlinks(filepath.Dir(archivePath))
	if err != nil {
		return fmt.Errorf("resolve archive parent: %w", err)
	}
	if pathWithin(resolvedBackup, filepath.Join(resolvedArchiveParent, filepath.Base(archivePath))) {
		return fmt.Errorf("backup archive must not be inside backup directory: %s", archivePath)
	}
	temporary, err := os.CreateTemp(filepath.Dir(archivePath), ".memodrive-backup-*.zip")
	if err != nil {
		return fmt.Errorf("create archive staging: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)

	writer := zip.NewWriter(temporary)
	walkErr := filepath.WalkDir(backupPath, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == backupPath || entry.IsDir() {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() {
			return fmt.Errorf("unsupported archive entry %q", path)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(backupPath, path)
		if err != nil {
			return err
		}
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(relative)
		header.Method = zip.Deflate
		output, err := writer.CreateHeader(header)
		if err != nil {
			return err
		}
		input, err := os.Open(path)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(output, input)
		closeErr := input.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
	if walkErr != nil {
		_ = writer.Close()
		_ = temporary.Close()
		return fmt.Errorf("archive backup: %w", walkErr)
	}
	if err := writer.Close(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("finalize backup archive: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close backup archive: %w", err)
	}
	if err := os.Rename(temporaryPath, archivePath); err != nil {
		return fmt.Errorf("publish backup archive: %w", err)
	}
	return nil
}

func pathWithin(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return relative == "." || relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
