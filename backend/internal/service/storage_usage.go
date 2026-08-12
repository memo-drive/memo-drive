package service

import (
	"context"
	"errors"
	"log"
	"os"
	"path/filepath"

	"github.com/memodrive/backend/internal/config"
	"github.com/memodrive/backend/internal/store"
)

// StorageUsage reports logical storage categories and physical capacity. The
// legacy used_bytes and total_bytes fields remain for one compatibility cycle.
type StorageUsage struct {
	UsedBytes                int64 `json:"used_bytes"`  // Deprecated: use ActiveBytes.
	TotalBytes               int64 `json:"total_bytes"` // Deprecated: use FilesystemTotalBytes.
	ActiveBytes              int64 `json:"active_bytes"`
	TrashBytes               int64 `json:"trash_bytes"`
	VersionBytes             int64 `json:"version_bytes"`
	TempBytes                int64 `json:"temp_bytes"`
	AuxiliaryBytes           int64 `json:"auxiliary_bytes"`
	FilesystemTotalBytes     int64 `json:"filesystem_total_bytes"`
	FilesystemAvailableBytes int64 `json:"filesystem_available_bytes"`
	QuotaBytes               int64 `json:"quota_bytes"`
	ReservedBytes            int64 `json:"reserved_bytes"`
	UploadAvailableBytes     int64 `json:"upload_available_bytes"`
}

// CapacityService centralizes storage accounting and write admission.
type CapacityService struct {
	cfg   *config.Config
	store *store.Store
}

func NewCapacityService(cfg *config.Config, store *store.Store) *CapacityService {
	return &CapacityService{cfg: cfg, store: store}
}

func (s *FileService) StorageUsage(ctx context.Context) (*StorageUsage, error) {
	return NewCapacityService(s.cfg, s.store).Usage(ctx)
}

func (s *CapacityService) Usage(ctx context.Context) (*StorageUsage, error) {
	snapshot, usage, err := s.snapshot(ctx)
	if err != nil {
		return nil, err
	}
	usage.UploadAvailableBytes = snapshot.UploadAvailableBytes()
	return usage, nil
}

func (s *CapacityService) Check(ctx context.Context, request CapacityRequest) error {
	snapshot, _, err := s.snapshot(ctx)
	if err != nil {
		return err
	}
	return snapshot.Check(request)
}

func (s *CapacityService) snapshot(ctx context.Context) (CapacitySnapshot, *StorageUsage, error) {
	active, err := s.store.TotalActiveFileSize(ctx)
	if err != nil {
		log.Printf("level=error component=storage event=usage_sum_failed err=%q", err)
		return CapacitySnapshot{}, nil, err
	}
	trash, err := s.store.TotalTrashFileSize(ctx)
	if err != nil {
		log.Printf("level=error component=storage event=trash_sum_failed err=%q", err)
		return CapacitySnapshot{}, nil, err
	}
	versions, err := s.store.TotalFileVersionSize(ctx)
	if err != nil {
		log.Printf("level=error component=storage event=version_usage_sum_failed err=%q", err)
		return CapacitySnapshot{}, nil, err
	}
	capacity, err := filesystemCapacity(s.cfg.Storage.Root)
	if err != nil {
		log.Printf("level=error component=storage event=statfs_failed root=%q err=%q", s.cfg.Storage.Root, err)
		return CapacitySnapshot{}, nil, err
	}
	uploadTempBytes, err := pathBytes(s.cfg.Storage.TempDir)
	if err != nil {
		return CapacitySnapshot{}, nil, err
	}
	stagingBytes, err := pathBytes(filepath.Join(s.cfg.Storage.Root, ".staging"))
	if err != nil {
		return CapacitySnapshot{}, nil, err
	}
	thumbnailBytes, err := pathBytes(s.cfg.Storage.ThumbnailDir)
	if err != nil {
		return CapacitySnapshot{}, nil, err
	}
	databaseBytes, err := sqliteDatabaseBytes(s.cfg.Storage.DBPath)
	if err != nil {
		return CapacitySnapshot{}, nil, err
	}
	tempBytes := uploadTempBytes + stagingBytes
	auxiliaryBytes := thumbnailBytes + databaseBytes
	snapshot := CapacitySnapshot{
		ActiveBytes:              active + versions,
		TempBytes:                tempBytes,
		FilesystemTotalBytes:     capacity.TotalBytes,
		FilesystemAvailableBytes: capacity.AvailableBytes,
		QuotaBytes:               s.cfg.Storage.QuotaBytes,
		ReservedBytes:            s.cfg.Storage.ReservedBytes,
		TempLimitBytes:           s.cfg.Storage.TempLimitBytes,
	}
	usage := &StorageUsage{
		UsedBytes:                active,
		TotalBytes:               capacity.TotalBytes,
		ActiveBytes:              active,
		TrashBytes:               trash,
		VersionBytes:             versions,
		TempBytes:                tempBytes,
		AuxiliaryBytes:           auxiliaryBytes,
		FilesystemTotalBytes:     capacity.TotalBytes,
		FilesystemAvailableBytes: capacity.AvailableBytes,
		QuotaBytes:               s.cfg.Storage.QuotaBytes,
		ReservedBytes:            s.cfg.Storage.ReservedBytes,
	}
	return snapshot, usage, nil
}

func pathBytes(root string) (int64, error) {
	var total int64
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		}
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			total += info.Size()
		}
		return nil
	})
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	return total, err
}

func sqliteDatabaseBytes(databasePath string) (int64, error) {
	var total int64
	for _, path := range []string{
		databasePath,
		databasePath + "-wal",
		databasePath + "-shm",
	} {
		info, err := os.Stat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return 0, err
		}
		if info.Mode().IsRegular() {
			total += info.Size()
		}
	}
	return total, nil
}
