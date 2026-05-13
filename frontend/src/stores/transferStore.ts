import { create } from "zustand";
import {
  clearUploadSessions,
  deleteUploadSession,
  listUploadSessions,
} from "../api/uploadApi";
import { transferUploadedBytes } from "../utils/uploadProgress";
import {
  isActiveTransferStatus,
  transferTaskFromSession,
} from "./transferProjection";
import type { TransferTask } from "./transferProjection";

export type { TransferStatus, TransferTask } from "./transferProjection";

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

type PersistedTransferTask = Omit<TransferTask, "file" | "uploadedBytes"> & {
  uploadedBytes?: number;
};

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
    return tasks.map((task) => {
      const uploadedBytes = transferUploadedBytes(task);
      return {
        ...task,
        uploadedBytes,
        file: undefined,
        status: task.status === "uploading" ? "paused" : task.status,
      };
    });
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
    if (task && !isActiveTransferStatus(task.status)) {
      await deleteUploadSession(id);
    }
    commit(set, (tasks) => tasks.filter((item) => item.id !== id));
  },
  clearDone: async () => {
    await clearUploadSessions();
    commit(set, (tasks) =>
      tasks.filter((task) => isActiveTransferStatus(task.status)),
    );
  },
  loadSessions: async () => {
    set({ loadingHistory: true });
    try {
      const response = await listUploadSessions();
      const existing = new Map(get().tasks.map((task) => [task.id, task]));
      const remoteTasks = response.sessions.map((session) =>
        transferTaskFromSession(session, existing.get(session.id)),
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
