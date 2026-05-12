import { formatBytes } from "./formatBytes";

const UPLOAD_PROGRESS_PERCENT_MAX = 90;

interface TransferProgressSource {
  fileSize: number;
  chunkSize: number;
  uploadedChunks: number[];
  uploadedBytes?: number;
}

export function uploadedBytesForChunks(
  uploadedChunks: number[],
  chunkSize: number,
  fileSize: number,
): number {
  if (!Number.isFinite(fileSize) || fileSize <= 0) return 0;
  if (!Number.isFinite(chunkSize) || chunkSize <= 0) return 0;

  let bytes = 0;
  const seen = new Set(uploadedChunks.filter(Number.isInteger));
  for (const index of seen) {
    const start = index * chunkSize;
    if (start < 0 || start >= fileSize) continue;
    bytes += Math.min(chunkSize, fileSize - start);
  }
  return Math.min(bytes, fileSize);
}

export function uploadPercentForBytes(uploadedBytes: number, fileSize: number): number {
  if (!Number.isFinite(uploadedBytes) || uploadedBytes <= 0) return 0;
  if (!Number.isFinite(fileSize) || fileSize <= 0) return UPLOAD_PROGRESS_PERCENT_MAX;
  return Math.min(
    UPLOAD_PROGRESS_PERCENT_MAX,
    Math.round((Math.min(uploadedBytes, fileSize) / fileSize) * UPLOAD_PROGRESS_PERCENT_MAX),
  );
}

export function transferUploadedBytes(source: TransferProgressSource): number {
  if (Number.isFinite(source.uploadedBytes) && source.uploadedBytes! >= 0) {
    return Math.min(source.uploadedBytes!, source.fileSize);
  }
  return uploadedBytesForChunks(source.uploadedChunks, source.chunkSize, source.fileSize);
}

export function formatTransferSpeed(bytesPerSecond: number): string {
  if (!Number.isFinite(bytesPerSecond) || bytesPerSecond <= 0) return "";
  return `${formatBytes(bytesPerSecond)}/s`;
}
