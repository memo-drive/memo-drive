import { create } from "zustand";
import type { DriveFile } from "../types";

interface FileState {
  currentPath: string;
  files: DriveFile[];
  selectedFile?: DriveFile;
  setCurrentPath: (path: string) => void;
  setFiles: (files: DriveFile[]) => void;
  setSelectedFile: (file?: DriveFile) => void;
}

export const useFileStore = create<FileState>((set) => ({
  currentPath: "/",
  files: [],
  selectedFile: undefined,
  setCurrentPath: (currentPath) => set({ currentPath, selectedFile: undefined }),
  setFiles: (files) => set({ files }),
  setSelectedFile: (selectedFile) => set({ selectedFile }),
}));
