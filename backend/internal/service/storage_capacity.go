package service

import (
	"errors"
	"fmt"
)

var ErrInsufficientStorage = errors.New("insufficient storage")

// FilesystemCapacity reports the total capacity and bytes available to the
// current process. AvailableBytes intentionally excludes filesystem blocks
// reserved for privileged users.
type FilesystemCapacity struct {
	TotalBytes     int64
	AvailableBytes int64
}

// CapacitySnapshot is the admission-check view of logical and physical storage.
type CapacitySnapshot struct {
	ActiveBytes              int64
	TempBytes                int64
	FilesystemTotalBytes     int64
	FilesystemAvailableBytes int64
	QuotaBytes               int64
	ReservedBytes            int64
	TempLimitBytes           int64
}

// CapacityRequest describes the additional logical and physical bytes needed
// by one write operation.
type CapacityRequest struct {
	LogicalBytes         int64
	ReplacedLogicalBytes int64
	PhysicalNeedBytes    int64
	TempNeedBytes        int64
}

// InsufficientStorageError identifies the capacity boundary that rejected a
// write and exposes byte counts suitable for a structured API response.
type InsufficientStorageError struct {
	Constraint     string
	RequiredBytes  int64
	AvailableBytes int64
}

func (e *InsufficientStorageError) Error() string {
	return fmt.Sprintf(
		"insufficient storage: %s requires %d bytes, %d available",
		e.Constraint,
		e.RequiredBytes,
		e.AvailableBytes,
	)
}

func (e *InsufficientStorageError) Unwrap() error {
	return ErrInsufficientStorage
}

func IsInsufficientStorage(err error) bool {
	return errors.Is(err, ErrInsufficientStorage)
}

// Check verifies logical quota, physical reserve, and temporary-space limits.
func (s CapacitySnapshot) Check(request CapacityRequest) error {
	if s.QuotaBytes > 0 {
		available := nonNegative(s.QuotaBytes - s.ActiveBytes)
		required := nonNegative(request.LogicalBytes - request.ReplacedLogicalBytes)
		if required > available {
			return &InsufficientStorageError{
				Constraint:     "quota",
				RequiredBytes:  required,
				AvailableBytes: available,
			}
		}
	}
	physicalHeadroom := s.FilesystemAvailableBytes - s.ReservedBytes
	physicalAvailable := nonNegative(physicalHeadroom)
	physicalRequired := nonNegative(request.PhysicalNeedBytes)
	if physicalRequired > physicalHeadroom {
		return &InsufficientStorageError{
			Constraint:     "filesystem",
			RequiredBytes:  physicalRequired,
			AvailableBytes: physicalAvailable,
		}
	}
	tempLimit := s.TempLimitBytes
	if tempLimit <= 0 {
		if s.QuotaBytes > 0 {
			tempLimit = s.QuotaBytes
		} else {
			tempLimit = nonNegative(s.FilesystemTotalBytes - s.ReservedBytes)
		}
	}
	tempHeadroom := tempLimit - s.TempBytes
	tempAvailable := nonNegative(tempHeadroom)
	tempRequired := nonNegative(request.TempNeedBytes)
	if tempRequired > tempHeadroom {
		return &InsufficientStorageError{
			Constraint:     "temporary",
			RequiredBytes:  tempRequired,
			AvailableBytes: tempAvailable,
		}
	}
	return nil
}

func (s CapacitySnapshot) UploadAvailableBytes() int64 {
	available := nonNegative(
		s.FilesystemAvailableBytes - s.ReservedBytes,
	)
	if s.QuotaBytes > 0 {
		available = minInt64(
			available,
			nonNegative(s.QuotaBytes-s.ActiveBytes),
		)
	}
	tempLimit := s.TempLimitBytes
	if tempLimit <= 0 {
		if s.QuotaBytes > 0 {
			tempLimit = s.QuotaBytes
		} else {
			tempLimit = nonNegative(s.FilesystemTotalBytes - s.ReservedBytes)
		}
	}
	return minInt64(available, nonNegative(tempLimit-s.TempBytes))
}

func nonNegative(value int64) int64 {
	if value < 0 {
		return 0
	}
	return value
}

func minInt64(left, right int64) int64 {
	if left < right {
		return left
	}
	return right
}
