import { useState } from "react";
import {
  cancelUpload,
  completeUpload,
  getUploadSession,
  initUpload,
  uploadChunk,
} from "../api/uploadApi";
import { useTransferStore } from "../stores/transferStore";
import type { TransferTask } from "../stores/transferStore";
import type { DriveFile, UploadSession } from "../types";

export interface UploadProgress {
  fileName: string;
  percent: number;
  status: "idle" | "uploading" | "processing" | "paused" | "done" | "failed" | "cancelled";
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

function uploadedPercent(uploadedCount: number, total: number) {
  return Math.min(90, Math.round((uploadedCount / Math.max(1, total)) * 90));
}

function isAbortError(error: unknown) {
  return error instanceof DOMException && error.name === "AbortError";
}

function taskFromUpload(file: File, session: UploadSession): TransferTask {
  const total = totalChunks(session);
  return {
    id: session.id,
    fileName: file.name,
    fileSize: file.size,
    destPath: session.dest_path,
    direction: "upload",
    status: "uploading",
    percent: uploadedPercent(session.uploaded_chunks.length, total),
    uploadedChunks: session.uploaded_chunks,
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
  pausedSessions.delete(session.id);
  cancelledSessions.delete(session.id);

  store.updateTask(session.id, {
    file,
    status: "uploading",
    uploadedChunks: [...uploaded].sort((a, b) => a - b),
    totalChunks: total,
    percent: uploadedPercent(uploaded.size, total),
    error: undefined,
  });
  callbacks.setProgress({
    fileName: file.name,
    percent: uploadedPercent(uploaded.size, total),
    status: "uploading",
  });

  try {
    for (let index = 0; index < total; index += 1) {
      if (pausedSessions.has(session.id)) {
        useTransferStore.getState().updateTask(session.id, { status: "paused" });
        callbacks.setProgress({
          fileName: file.name,
          percent: uploadedPercent(uploaded.size, total),
          status: "paused",
        });
        return undefined;
      }
      if (uploaded.has(index)) continue;

      const start = index * session.chunk_size;
      const end = Math.min(start + session.chunk_size, file.size);
      const controller = new AbortController();
      controllers.set(session.id, controller);
      const result = await uploadChunk(
        session.id,
        index,
        file.slice(start, end),
        controller.signal,
      );
      if (controllers.get(session.id) === controller) {
        controllers.delete(session.id);
      }

      uploaded.clear();
      for (const chunkIndex of result.uploaded_chunks) {
        uploaded.add(chunkIndex);
      }
      const uploadedChunks = [...uploaded].sort((a, b) => a - b);
      const elapsedSec = Math.max((Date.now() - startedAt) / 1000, 0.001);
      const uploadedBytes = Math.min(uploadedChunks.length * session.chunk_size, file.size);
      const percent = uploadedPercent(uploadedChunks.length, total);
      useTransferStore.getState().updateTask(session.id, {
        status: pausedSessions.has(session.id) ? "paused" : "uploading",
        percent,
        uploadedChunks,
        speed: uploadedBytes / elapsedSec,
      });
      callbacks.setProgress({
        fileName: file.name,
        percent,
        status: pausedSessions.has(session.id) ? "paused" : "uploading",
      });
    }

    if (pausedSessions.has(session.id)) {
      useTransferStore.getState().updateTask(session.id, { status: "paused" });
      callbacks.setProgress({
        fileName: file.name,
        percent: uploadedPercent(uploaded.size, total),
        status: "paused",
      });
      return undefined;
    }

    useTransferStore.getState().updateTask(session.id, {
      status: "processing",
      percent: 95,
    });
    callbacks.setProgress({ fileName: file.name, percent: 95, status: "processing" });
    const completed = await completeUpload(session.id);
    useTransferStore.getState().updateTask(session.id, {
      status: "done",
      percent: 100,
      speed: 0,
      file: undefined,
      error: undefined,
    });
    callbacks.setProgress({ fileName: file.name, percent: 100, status: "done" });
    callbacks.onUploaded(completed.file);
    return completed.file;
  } catch (error) {
    controllers.delete(session.id);
    if (isAbortError(error) || cancelledSessions.has(session.id)) {
      useTransferStore.getState().updateTask(session.id, {
        status: "cancelled",
        speed: 0,
        file: undefined,
      });
      callbacks.setProgress({
        fileName: file.name,
        percent: uploadedPercent(uploaded.size, total),
        status: "cancelled",
      });
      throw new Error("upload cancelled");
    }
    const message = error instanceof Error ? error.message : "upload failed";
    useTransferStore.getState().updateTask(session.id, {
      status: "failed",
      error: message,
      speed: 0,
    });
    callbacks.setProgress({
      fileName: file.name,
      percent: uploadedPercent(uploaded.size, total),
      status: "failed",
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

  async function upload(file: File, destPath: string) {
    setProgress({ fileName: file.name, percent: 0, status: "uploading" });
    let session: UploadSession;
    try {
      session = await initUpload(file, destPath);
    } catch (error) {
      setProgress({
        fileName: file.name,
        percent: 0,
        status: "failed",
        error: error instanceof Error ? error.message : "upload failed",
      });
      throw error;
    }
    useTransferStore.getState().addTask(taskFromUpload(file, session));
    return uploadRemainingChunks(file, session, { onUploaded, setProgress });
  }

  async function resume(id: string, file?: File) {
    const task = useTransferStore.getState().tasks.find((item) => item.id === id);
    const sourceFile = file ?? task?.file;
    if (!sourceFile || !task) {
      throw new Error("请选择原文件继续上传");
    }
    if (sourceFile.name !== task.fileName || sourceFile.size !== task.fileSize) {
      throw new Error("请选择同名且大小一致的原文件");
    }
    const session = await getUploadSession(id);
    if (session.status === "done") {
      useTransferStore.getState().updateTask(id, { status: "done", percent: 100 });
      return undefined;
    }
    if (session.status !== "uploading") {
      useTransferStore.getState().updateTask(id, {
        status: session.status === "expired" ? "expired" : "failed",
        error: `无法续传，当前状态为 ${session.status}`,
      });
      throw new Error(`无法续传，当前状态为 ${session.status}`);
    }
    return uploadRemainingChunks(sourceFile, session, { onUploaded, setProgress });
  }

  return {
    progress,
    upload,
    pause: pauseUploadTask,
    cancel: cancelUploadTask,
    resume,
  };
}
