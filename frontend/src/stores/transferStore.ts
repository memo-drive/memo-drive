import { create } from "zustand";
import {
  clearUploadSessions,
  deleteUploadSession,
  listUploadSessions,
} from "../api/uploadApi";
import type { UploadSession } from "../types";

export type TransferStatus =
  | "uploading"
  | "paused"
  | "processing"
  | "done"
  | "failed"
  | "cancelled"
  | "expired";

export interface TransferTask {
  id: string;
  fileName: string;
  fileSize: number;
  destPath: string;
  direction: "upload";
  status: TransferStatus;
  percent: number;
  uploadedChunks: number[];
  totalChunks: number;
  chunkSize: number;
  speed: number;
  error?: string;
  createdAt: number;
  updatedAt: number;
  expiresAt?: string;
  file?: File;
}

interface TransferState {
  tasks: TransferTask[];
  loadingHistory: boolean;
  addTask: (task: TransferTask) => void;
  updateTask: (id: string, patch: Partial<TransferTask>) => void;
  removeTask: (id: string) => Promise<void>;
  clearDone: () => Promise<void>;
  loadSessions: () => Promise<void>;
}

const STORAGE_KEY = "memodrive.transfer.tasks";
const MAX_TASKS = 100;

type PersistedTransferTask = Omit<TransferTask, "file">;

function persistTasks(tasks: TransferTask[]) {
  const serializable = tasks
    .slice(0, MAX_TASKS)
    .map(({ file: _file, ...task }) => task);
  localStorage.setItem(STORAGE_KEY, JSON.stringify(serializable));
}

function loadPersistedTasks(): TransferTask[] {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return [];
    const tasks = JSON.parse(raw) as PersistedTransferTask[];
    if (!Array.isArray(tasks)) return [];
    return tasks.map((task) => ({
      ...task,
      file: undefined,
      status: task.status === "uploading" ? "paused" : task.status,
    }));
  } catch {
    return [];
  }
}

function commit(
  set: (fn: (state: TransferState) => Partial<TransferState>) => void,
  updater: (tasks: TransferTask[]) => TransferTask[],
) {
  set((state) => {
    const tasks = updater(state.tasks).slice(0, MAX_TASKS);
    persistTasks(tasks);
    return { tasks };
  });
}

function sortTasks(tasks: TransferTask[]) {
  return [...tasks].sort((a, b) => b.createdAt - a.createdAt);
}

function isActiveStatus(status: TransferStatus) {
  return status === "uploading" || status === "paused" || status === "processing";
}

function taskFromSession(
  session: UploadSession,
  existing?: TransferTask,
): TransferTask {
  const totalChunks = Math.max(
    1,
    Math.ceil(session.file_size / session.chunk_size),
  );
  const uploadedChunks = session.uploaded_chunks ?? [];
  const uploadedPercent = Math.round((uploadedChunks.length / totalChunks) * 90);
  let status: TransferStatus;
  switch (session.status) {
    case "done":
      status = "done";
      break;
    case "cancelled":
      status = "cancelled";
      break;
    case "expired":
      status = "expired";
      break;
    case "merging":
      status = "processing";
      break;
    case "uploading":
      status = existing?.file && existing.status === "uploading" ? "uploading" : "paused";
      break;
    default:
      status = "failed";
      break;
  }
  return {
    id: session.id,
    fileName: session.file_name,
    fileSize: session.file_size,
    destPath: session.dest_path,
    direction: "upload",
    status,
    percent: status === "done" ? 100 : status === "processing" ? 95 : uploadedPercent,
    uploadedChunks,
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

export const useTransferStore = create<TransferState>((set, get) => ({
  tasks: loadPersistedTasks(),
  loadingHistory: false,
  addTask: (task) =>
    commit(set, (tasks) =>
      sortTasks([task, ...tasks.filter((item) => item.id !== task.id)]),
    ),
  updateTask: (id, patch) =>
    commit(set, (tasks) =>
      tasks.map((task) =>
        task.id === id
          ? { ...task, ...patch, updatedAt: patch.updatedAt ?? Date.now() }
          : task,
      ),
    ),
  removeTask: async (id) => {
    const task = get().tasks.find((item) => item.id === id);
    if (task && !isActiveStatus(task.status)) {
      await deleteUploadSession(id);
    }
    commit(set, (tasks) => tasks.filter((item) => item.id !== id));
  },
  clearDone: async () => {
    await clearUploadSessions();
    commit(set, (tasks) => tasks.filter((task) => isActiveStatus(task.status)));
  },
  loadSessions: async () => {
    set({ loadingHistory: true });
    try {
      const response = await listUploadSessions();
      const existing = new Map(get().tasks.map((task) => [task.id, task]));
      const remoteTasks = response.sessions.map((session) =>
        taskFromSession(session, existing.get(session.id)),
      );
      const remoteIDs = new Set(remoteTasks.map((task) => task.id));
      const localOnly = get().tasks.filter(
        (task) => !remoteIDs.has(task.id) && task.status === "failed",
      );
      const tasks = sortTasks([...remoteTasks, ...localOnly]).slice(0, MAX_TASKS);
      persistTasks(tasks);
      set({ tasks, loadingHistory: false });
    } catch {
      set({ loadingHistory: false });
    }
  },
}));
