import type { DriveFile } from "../../types";

export interface DriveDeleteDraft {
  target: DriveFile;
}

export interface DriveDeleteConfirmedState {
  deleteTarget: null;
  selectedFile: undefined;
}

export function startDriveDelete(file: DriveFile): DriveDeleteDraft {
  return { target: file };
}

export function confirmDriveDelete(): DriveDeleteConfirmedState {
  return {
    deleteTarget: null,
    selectedFile: undefined,
  };
}
