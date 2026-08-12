package service

import "testing"

func TestFilesystemCapacityReportsTotalAndCallerAvailableBytes(t *testing.T) {
	capacity, err := filesystemCapacity(t.TempDir())
	if err != nil {
		t.Fatalf("filesystemCapacity: %v", err)
	}
	if capacity.TotalBytes <= 0 {
		t.Fatalf("total bytes = %d, want positive", capacity.TotalBytes)
	}
	if capacity.AvailableBytes < 0 {
		t.Fatalf("available bytes = %d, want non-negative", capacity.AvailableBytes)
	}
	if capacity.AvailableBytes > capacity.TotalBytes {
		t.Fatalf(
			"available bytes = %d, want <= total bytes %d",
			capacity.AvailableBytes,
			capacity.TotalBytes,
		)
	}
}

func TestCapacitySnapshotRejectsLogicalGrowthBeyondQuota(t *testing.T) {
	snapshot := CapacitySnapshot{
		ActiveBytes:              90,
		FilesystemTotalBytes:     1000,
		FilesystemAvailableBytes: 900,
		QuotaBytes:               100,
	}

	err := snapshot.Check(CapacityRequest{LogicalBytes: 11})
	if !IsInsufficientStorage(err) {
		t.Fatalf("Check() error = %v, want insufficient storage", err)
	}
}

func TestCapacitySnapshotUsesOnlyReplaceGrowthAgainstLogicalQuota(t *testing.T) {
	snapshot := CapacitySnapshot{
		ActiveBytes:              90,
		FilesystemTotalBytes:     1000,
		FilesystemAvailableBytes: 900,
		QuotaBytes:               100,
	}

	err := snapshot.Check(CapacityRequest{
		LogicalBytes:         40,
		ReplacedLogicalBytes: 30,
	})
	if err != nil {
		t.Fatalf("Check() error = %v, want 10-byte replace growth to fit", err)
	}
}

func TestCapacitySnapshotRequiresFullStagingSpaceBeyondReservedBytes(t *testing.T) {
	snapshot := CapacitySnapshot{
		ActiveBytes:              90,
		FilesystemTotalBytes:     1000,
		FilesystemAvailableBytes: 120,
		QuotaBytes:               100,
		ReservedBytes:            50,
	}

	err := snapshot.Check(CapacityRequest{
		LogicalBytes:         100,
		ReplacedLogicalBytes: 90,
		PhysicalNeedBytes:    71,
	})
	if !IsInsufficientStorage(err) {
		t.Fatalf("Check() error = %v, want full staging space rejection", err)
	}
}

func TestCapacitySnapshotRejectsTemporaryGrowthBeyondLimit(t *testing.T) {
	snapshot := CapacitySnapshot{
		TempBytes:                80,
		FilesystemTotalBytes:     1000,
		FilesystemAvailableBytes: 900,
		TempLimitBytes:           100,
	}

	err := snapshot.Check(CapacityRequest{TempNeedBytes: 21})
	if !IsInsufficientStorage(err) {
		t.Fatalf("Check() error = %v, want temporary space rejection", err)
	}
}
