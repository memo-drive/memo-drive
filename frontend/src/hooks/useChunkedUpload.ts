import { useState } from "react";
import {
  cancelUpload,
  completeUpload,
  getUploadSession,
  initUpload,
  uploadChunk,
} from "../api/uploadApi";
import { HttpError } from "../api/HttpClient";
import { useTransferStore } from "../stores/transferStore";
import type { TransferTask } from "../stores/transferStore";
import {
  preparingTransferTaskFromFile,
  type DirectoryTransferContext,
} from "../stores/transferProjection";
import type {
  DriveFile,
  FileConflictPolicy,
  UploadSession,
} from "../types";
import {
  uploadedBytesForChunks,
  uploadPercentForBytes,
} from "../utils/uploadProgress";

export interface UploadProgress {
  fileName: string;
  percent: number;
  status: "idle" | "uploading" | "processing" | "paused" | "done" | "failed" | "cancelled";
  uploadedBytes?: number;
  speed?: number;
  error?: string;
}

const controllers = new Map<string, AbortController>();
const pausedSessions = new Set<string>();
const cancelledSessions = new Set<string>();

interface UploadCallbacks {
  onUploaded: (file: DriveFile) => void;
  setProgress: (progress: UploadProgress | null) => void;
}

function totalChunks(session: UploadSession) {
  return Math.max(1, Math.ceil(session.file_size / session.chunk_size));
}

function isAbortError(error: unknown) {
  return error instanceof DOMException && error.name === "AbortError";
}

function taskFromUpload(file: File, session: UploadSession): TransferTask {
  const total = totalChunks(session);
  const uploadedBytes = uploadedBytesForChunks(
    session.uploaded_chunks,
    session.chunk_size,
    file.size,
  );
  return {
    id: session.id,
    fileName: file.name,
    requestedName: session.requested_name || file.name,
    resolvedName: session.resolved_name || session.file_name || file.name,
    overwritePolicy: session.overwrite_policy,
    fileSize: file.size,
    destPath: session.dest_path,
    direction: "upload",
    status: "uploading",
    percent: uploadPercentForBytes(uploadedBytes, file.size),
    uploadedChunks: session.uploaded_chunks,
    uploadedBytes,
    totalChunks: total,
    chunkSize: session.chunk_size,
    speed: 0,
    createdAt: session.created_at
      ? new Date(session.created_at).getTime()
      : Date.now(),
    updatedAt: Date.now(),
    expiresAt: session.expires_at,
    file,
  };
}

async function uploadRemainingChunks(
  file: File,
  session: UploadSession,
  callbacks: UploadCallbacks,
) {
  const store = useTransferStore.getState();
  const total = totalChunks(session);
  const uploaded = new Set(session.uploaded_chunks);
  const startedAt = Date.now();
  const initialUploadedBytes = uploadedBytesForChunks(
    [...uploaded],
    session.chunk_size,
    file.size,
  );
  let currentUploadedBytes = initialUploadedBytes;
  let lastLiveUpdateAt = 0;
  pausedSessions.delete(session.id);
  cancelledSessions.delete(session.id);

  const updateLiveProgress = (
    uploadedBytes: number,
    status: "uploading" | "paused" | "processing" | "failed" | "cancelled",
    uploadedChunks?: number[],
    force = true,
  ) => {
    const now = Date.now();
    if (!force && now - lastLiveUpdateAt < 250) return;
    lastLiveUpdateAt = now;
    currentUploadedBytes = Math.max(0, Math.min(uploadedBytes, file.size));
    const elapsedSec = Math.max((now - startedAt) / 1000, 0.001);
    const speed =
      status === "uploading"
        ? Math.max(0, currentUploadedBytes - initialUploadedBytes) / elapsedSec
        : 0;
    const percent =
      status === "processing"
        ? 95
        : uploadPercentForBytes(currentUploadedBytes, file.size);
    const patch: Partial<TransferTask> = {
      status,
      percent,
      uploadedBytes: currentUploadedBytes,
      speed,
    };
    if (uploadedChunks) {
      patch.uploadedChunks = uploadedChunks;
    }
    useTransferStore.getState().updateTask(session.id, patch);
    callbacks.setProgress({
      fileName: file.name,
      percent,
      status,
      uploadedBytes: currentUploadedBytes,
      speed,
    });
  };

  const initialUploadedChunks = [...uploaded].sort((a, b) => a - b);
  store.updateTask(session.id, {
    file,
    status: "uploading",
    uploadedChunks: initialUploadedChunks,
    uploadedBytes: initialUploadedBytes,
    totalChunks: total,
    percent: uploadPercentForBytes(initialUploadedBytes, file.size),
    error: undefined,
    errorCode: undefined,
    conflictRetryable: false,
  });
  callbacks.setProgress({
    fileName: file.name,
    percent: uploadPercentForBytes(initialUploadedBytes, file.size),
    status: "uploading",
    uploadedBytes: initialUploadedBytes,
    speed: 0,
  });

  try {
    for (let index = 0; index < total; index += 1) {
      if (pausedSessions.has(session.id)) {
        useTransferStore.getState().updateTask(session.id, { status: "paused" });
        callbacks.setProgress({
          fileName: file.name,
          percent: uploadPercentForBytes(currentUploadedBytes, file.size),
          status: "paused",
          uploadedBytes: currentUploadedBytes,
          speed: 0,
        });
        return undefined;
      }
      if (uploaded.has(index)) continue;

      const start = index * session.chunk_size;
      const end = Math.min(start + session.chunk_size, file.size);
      const chunkBytes = end - start;
      const committedBytes = uploadedBytesForChunks(
        [...uploaded],
        session.chunk_size,
        file.size,
      );
      const controller = new AbortController();
      controllers.set(session.id, controller);
      const result = await uploadChunk(
        session.id,
        index,
        file.slice(start, end),
        controller.signal,
        ({ loaded }) => {
          updateLiveProgress(
            committedBytes + Math.min(loaded, chunkBytes),
            "uploading",
            undefined,
            false,
          );
        },
      );
      if (controllers.get(session.id) === controller) {
        controllers.delete(session.id);
      }

      uploaded.clear();
      for (const chunkIndex of result.uploaded_chunks) {
        uploaded.add(chunkIndex);
      }
      const uploadedChunks = [...uploaded].sort((a, b) => a - b);
      updateLiveProgress(
        uploadedBytesForChunks(uploadedChunks, session.chunk_size, file.size),
        pausedSessions.has(session.id) ? "paused" : "uploading",
        uploadedChunks,
      );
    }

    if (pausedSessions.has(session.id)) {
      useTransferStore.getState().updateTask(session.id, { status: "paused" });
      callbacks.setProgress({
        fileName: file.name,
        percent: uploadPercentForBytes(currentUploadedBytes, file.size),
        status: "paused",
        uploadedBytes: currentUploadedBytes,
        speed: 0,
      });
      return undefined;
    }

    useTransferStore.getState().updateTask(session.id, {
      status: "processing",
      percent: 95,
      uploadedBytes: file.size,
      speed: 0,
    });
    callbacks.setProgress({
      fileName: file.name,
      percent: 95,
      status: "processing",
      uploadedBytes: file.size,
      speed: 0,
    });
    const completed = await completeUpload(session.id);
    useTransferStore.getState().updateTask(session.id, {
      status: "done",
      percent: 100,
      uploadedBytes: file.size,
      speed: 0,
      file: undefined,
      error: undefined,
      errorCode: undefined,
      conflictRetryable: false,
      resolvedName: completed.file.name,
    });
    callbacks.setProgress({
      fileName: file.name,
      percent: 100,
      status: "done",
      uploadedBytes: file.size,
      speed: 0,
    });
    callbacks.onUploaded(completed.file);
    return completed.file;
  } catch (error) {
    controllers.delete(session.id);
    if (isAbortError(error) || cancelledSessions.has(session.id)) {
      useTransferStore.getState().updateTask(session.id, {
        status: "cancelled",
        uploadedBytes: currentUploadedBytes,
        speed: 0,
        file: undefined,
      });
      callbacks.setProgress({
        fileName: file.name,
        percent: uploadPercentForBytes(currentUploadedBytes, file.size),
        status: "cancelled",
        uploadedBytes: currentUploadedBytes,
        speed: 0,
      });
      throw new Error("upload cancelled");
    }
    const message = error instanceof Error ? error.message : "upload failed";
    const errorCode = error instanceof HttpError ? error.code : undefined;
    const conflictRetryable =
      errorCode === "path_conflict" || errorCode === "name_exhausted";
    useTransferStore.getState().updateTask(session.id, {
      status: "failed",
      error: message,
      errorCode,
      conflictRetryable,
      uploadedBytes: currentUploadedBytes,
      speed: 0,
    });
    callbacks.setProgress({
      fileName: file.name,
      percent: uploadPercentForBytes(currentUploadedBytes, file.size),
      status: "failed",
      uploadedBytes: currentUploadedBytes,
      speed: 0,
      error: message,
    });
    throw error;
  }
}

export function pauseUploadTask(id: string) {
  pausedSessions.add(id);
  useTransferStore.getState().updateTask(id, { status: "paused", speed: 0 });
}

export async function cancelUploadTask(id: string) {
  cancelledSessions.add(id);
  controllers.get(id)?.abort();
  controllers.delete(id);
  pausedSessions.delete(id);
  useTransferStore.getState().updateTask(id, {
    status: "cancelled",
    speed: 0,
    file: undefined,
  });
  await cancelUpload(id).catch(() => undefined);
}

export function useChunkedUpload(onUploaded: (file: DriveFile) => void) {
  const [progress, setProgress] = useState<UploadProgress | null>(null);

  async function upload(
    file: File,
    destPath: string,
    overwritePolicy: FileConflictPolicy = "reject",
    directory?: DirectoryTransferContext,
  ) {
		const preparingTask = preparingTransferTaskFromFile(file, destPath, Date.now(), directory);
    preparingTask.overwritePolicy = overwritePolicy;
    useTransferStore.getState().addTask(preparingTask);
    setProgress({ fileName: file.name, percent: 0, status: "uploading" });
    let session: UploadSession;
    try {
      session = await initUpload(file, destPath, overwritePolicy);
    } catch (error) {
      const errorCode = error instanceof HttpError ? error.code : undefined;
      const conflictRetryable =
        errorCode === "path_conflict" || errorCode === "name_exhausted";
      useTransferStore.getState().updateTask(preparingTask.id, {
        status: "failed",
        error: error instanceof Error ? error.message : "upload failed",
        errorCode,
        conflictRetryable,
        speed: 0,
        file: conflictRetryable ? file : undefined,
      });
      setProgress({
        fileName: file.name,
        percent: 0,
        status: "failed",
        error: error instanceof Error ? error.message : "upload failed",
      });
      throw error;
    }
    useTransferStore.getState().updateTask(preparingTask.id, taskFromUpload(file, session));
    return uploadRemainingChunks(file, session, { onUploaded, setProgress });
  }

  async function resume(id: string, file?: File) {
    const task = useTransferStore.getState().tasks.find((item) => item.id === id);
    const sourceFile = file ?? task?.file;
    if (!sourceFile || !task) {
      throw new Error("upload.selectOriginalFile");
    }
    if (sourceFile.name !== task.fileName || sourceFile.size !== task.fileSize) {
      throw new Error("upload.selectSameFile");
    }
    const session = await getUploadSession(id);
    if (session.status === "done") {
      useTransferStore.getState().updateTask(id, { status: "done", percent: 100 });
      return undefined;
    }
    if (session.status !== "uploading") {
      useTransferStore.getState().updateTask(id, {
        status: session.status === "expired" ? "expired" : "failed",
        error: `upload.cannotResume.${session.status}`,
      });
      throw new Error(`upload.cannotResume.${session.status}`);
    }
    return uploadRemainingChunks(sourceFile, session, { onUploaded, setProgress });
  }

  async function retryConflict(
    task: TransferTask,
    overwritePolicy: Extract<FileConflictPolicy, "rename" | "replace">,
    file?: File,
  ) {
    const sourceFile = file ?? task.file;
    if (!sourceFile) {
      throw new Error("upload.selectOriginalFile");
    }
    const requestedName = task.requestedName ?? task.fileName;
    if (sourceFile.name !== requestedName || sourceFile.size !== task.fileSize) {
      throw new Error("upload.selectSameFile");
    }
    await useTransferStore.getState().removeTask(task.id);
		return upload(sourceFile, task.destPath, overwritePolicy, task.directoryBatchId && task.relativePath ? {
			batchId: task.directoryBatchId,
			relativePath: task.relativePath,
		} : undefined);
  }

  return {
    progress,
    upload,
    pause: pauseUploadTask,
    cancel: cancelUploadTask,
    resume,
    retryConflict,
  };
}
