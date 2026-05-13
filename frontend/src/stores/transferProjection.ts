import type { UploadSession } from "../types";
import {
  uploadedBytesForChunks,
  uploadPercentForBytes,
} from "../utils/uploadProgress";

export type TransferStatus =
  | "uploading"
  | "paused"
  | "processing"
  | "done"
  | "failed"
  | "cancelled"
  | "expired";

export type ActiveTransferStatus = Extract<
  TransferStatus,
  "uploading" | "paused" | "processing"
>;

export interface TransferTask {
  id: string;
  fileName: string;
  fileSize: number;
  destPath: string;
  direction: "upload";
  status: TransferStatus;
  percent: number;
  uploadedChunks: number[];
  uploadedBytes: number;
  totalChunks: number;
  chunkSize: number;
  speed: number;
  error?: string;
  createdAt: number;
  updatedAt: number;
  expiresAt?: string;
  file?: File;
}

type ExistingTransferSnapshot = Partial<
  Pick<
    TransferTask,
    "file" | "status" | "speed" | "error" | "createdAt" | "updatedAt"
  >
>;

interface TransferStatusSnapshot {
  file?: File;
  status?: TransferStatus;
}

const TRANSFER_STATUS_LABEL_KEYS: Record<TransferStatus, string> = {
  uploading: "transfer.status.uploading",
  paused: "transfer.status.paused",
  processing: "transfer.status.processing",
  done: "transfer.status.done",
  failed: "transfer.status.failed",
  cancelled: "transfer.status.cancelled",
  expired: "transfer.status.expired",
};

export function isActiveTransferStatus(
  status: string,
): status is ActiveTransferStatus {
  return status === "uploading" || status === "paused" || status === "processing";
}

export function transferStatusLabelKey(status: TransferStatus): string {
  return TRANSFER_STATUS_LABEL_KEYS[status];
}

export function transferStatusFromSession(
  session: UploadSession,
  existing?: TransferStatusSnapshot,
): TransferStatus {
  switch (session.status) {
    case "done":
      return "done";
    case "cancelled":
      return "cancelled";
    case "expired":
      return "expired";
    case "failed":
      return "failed";
    case "merging":
      return "processing";
    case "uploading":
      return existing?.file && existing.status === "uploading" ? "uploading" : "paused";
  }
}

export function transferTaskFromSession(
  session: UploadSession,
  existing?: ExistingTransferSnapshot,
): TransferTask {
  const totalChunks = Math.max(
    1,
    Math.ceil(session.file_size / session.chunk_size),
  );
  const uploadedChunks = session.uploaded_chunks ?? [];
  const uploadedBytes = uploadedBytesForChunks(
    uploadedChunks,
    session.chunk_size,
    session.file_size,
  );
  const status = transferStatusFromSession(session, existing);
  return {
    id: session.id,
    fileName: session.file_name,
    fileSize: session.file_size,
    destPath: session.dest_path,
    direction: "upload",
    status,
    percent:
      status === "done"
        ? 100
        : status === "processing"
          ? 95
          : uploadPercentForBytes(uploadedBytes, session.file_size),
    uploadedChunks,
    uploadedBytes,
    totalChunks,
    chunkSize: session.chunk_size,
    speed: existing?.speed ?? 0,
    error: existing?.error,
    createdAt: session.created_at
      ? new Date(session.created_at).getTime()
      : existing?.createdAt ?? Date.now(),
    updatedAt: existing?.updatedAt ?? Date.now(),
    expiresAt: session.expires_at,
    file: existing?.file,
  };
}
