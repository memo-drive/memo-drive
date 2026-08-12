import type { StorageUsage } from "../types";

export interface StorageUsagePresentation {
  uploadCapacity: number;
  usedPercent: number;
  low: boolean;
}

export function storageUsagePresentation(
  usage: StorageUsage,
): StorageUsagePresentation {
  const uploadCapacity =
    usage.quota_bytes > 0
      ? usage.quota_bytes
      : Math.max(
          0,
          usage.filesystem_total_bytes - usage.reserved_bytes,
        );
  const usedPercent =
    uploadCapacity > 0
      ? Math.min(
          100,
          Math.max(
            0,
            ((uploadCapacity - usage.upload_available_bytes) /
              uploadCapacity) *
              100,
          ),
        )
      : 0;
  return {
    uploadCapacity,
    usedPercent,
    low:
      uploadCapacity > 0 &&
      usage.upload_available_bytes <= uploadCapacity * 0.1,
  };
}
