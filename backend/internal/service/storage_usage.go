package service

import (
	"context"
	"log"
)

type StorageUsage struct {
	UsedBytes  int64 `json:"used_bytes"`
	TotalBytes int64 `json:"total_bytes"`
}

func (s *FileService) StorageUsage(ctx context.Context) (*StorageUsage, error) {
	used, err := s.store.TotalActiveFileSize(ctx)
	if err != nil {
		log.Printf("level=error component=storage event=usage_sum_failed err=%q", err)
		return nil, err
	}
	total, err := filesystemTotalBytes(s.cfg.Storage.Root)
	if err != nil {
		log.Printf("level=error component=storage event=statfs_failed root=%q err=%q", s.cfg.Storage.Root, err)
		return nil, err
	}
	return &StorageUsage{
		UsedBytes:  used,
		TotalBytes: total,
	}, nil
}
