import type { DriveFile } from "../../types";

export interface DriveMoveDraft {
  target: DriveFile;
}

export interface DriveMoveCompletedState {
  moveTarget: null;
  selectedFile: undefined;
}

export function startDriveMove(file: DriveFile): DriveMoveDraft {
  return { target: file };
}

export function completeDriveMove(): DriveMoveCompletedState {
  return {
    moveTarget: null,
    selectedFile: undefined,
  };
}
