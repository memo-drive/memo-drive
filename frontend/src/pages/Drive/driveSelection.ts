import type { DriveFile } from "../../types";
import { driveFolderPath } from "./drivePath";

export interface DrivePickResult {
  selectedFile: DriveFile;
  previewFile: DriveFile | null;
  nextPath: string | null;
}

export function pickDriveFile(file: DriveFile): DrivePickResult {
  return {
    selectedFile: file,
    previewFile: file.is_dir ? null : file,
    nextPath: null,
  };
}

export function pickSearchResult(file: DriveFile): DrivePickResult {
  return {
    selectedFile: file,
    previewFile: file.is_dir ? null : file,
    nextPath: file.is_dir ? driveFolderPath(file) : null,
  };
}
