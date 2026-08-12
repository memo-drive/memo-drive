import type { DriveFile } from "../types";

export interface DriveCrumb {
  label: string;
  path: string;
}

export interface DriveFolderDraft {
  open: boolean;
  draftName: string;
}

export interface DriveMarkdownDraft {
  open: boolean;
  draftName: string;
}

export interface DriveMoveDraft {
  target: DriveFile;
}

export interface DriveMoveCompletedState {
  moveTarget: null;
  selectedFile: undefined;
}

export interface DriveDeleteDraft {
  target: DriveFile;
}

export interface DriveDeleteConfirmedState {
  deleteTarget: null;
  selectedFile: undefined;
}

export interface DrivePickResult {
  selectedFile: DriveFile;
  previewFile: DriveFile | null;
  nextPath: string | null;
}

export interface DriveFolderEntry {
  enteringFolderId: string;
  nextPath: string;
}

export interface DriveRenameDraft {
  target: DriveFile;
  draftName: string;
}

export type DrivePreviewBadgeTone = "processing" | "failed";

export interface DrivePreviewBadge {
  labelKey: string;
  tone: DrivePreviewBadgeTone;
}

export interface DrivePreviewTitle {
  fileName: string;
  downloadFileName: string;
  downloadHref: string;
  badge: DrivePreviewBadge | null;
}

export interface DriveSearchRequest {
  query: string;
  path: string;
  semantic: boolean;
  limit: number;
}

export type DriveUploadSelection = ArrayLike<File> | null;

export const DRIVE_SEARCH_LIMIT = 50;

export function buildDriveCrumbs(
  currentPath: string,
  rootLabel: string,
  maxLevels: number,
): DriveCrumb[] {
  const parts = currentPath.split("/").filter(Boolean);
  const all: DriveCrumb[] = [
    { label: rootLabel, path: "/" },
    ...parts.map((part, index) => ({
      label: part,
      path: "/" + parts.slice(0, index + 1).join("/"),
    })),
  ];
  if (all.length <= maxLevels) return all;
  return [
    all[0],
    { label: "...", path: "" },
    ...all.slice(all.length - (maxLevels - 1)),
  ];
}

export function driveParentPath(currentPath: string): string | null {
  if (currentPath === "/") return null;
  const parts = currentPath.split("/").filter(Boolean);
  parts.pop();
  return parts.length === 0 ? "/" : "/" + parts.join("/");
}

export function driveFolderPath(file: DriveFile): string {
  return file.path === "/" ? `/${file.name}` : `${file.path}/${file.name}`;
}

export function startDriveFolderEntry(
  file: DriveFile,
  activeEnteringFolderId: string | null,
): DriveFolderEntry | null {
  if (!file.is_dir || activeEnteringFolderId) return null;
  return {
    enteringFolderId: file.id,
    nextPath: driveFolderPath(file),
  };
}

export function startDriveFolderCreate(): DriveFolderDraft {
  return {
    open: true,
    draftName: "",
  };
}

export function driveFolderPayloadName(draftName: string): string {
  return draftName.trim();
}

export function canSubmitDriveFolder(draftName: string): boolean {
  return Boolean(driveFolderPayloadName(draftName));
}

export function completeDriveFolderCreate(): DriveFolderDraft {
  return {
    open: false,
    draftName: "",
  };
}

export function startDriveMarkdownCreate(): DriveMarkdownDraft {
  return {
    open: true,
    draftName: "",
  };
}

export function driveMarkdownPayloadName(draftName: string): string {
  return draftName.trim();
}

export function canSubmitDriveMarkdown(draftName: string): boolean {
  const trimmed = driveMarkdownPayloadName(draftName);
  return Boolean(trimmed) && !trimmed.includes("/");
}

export function driveMarkdownErrorKey(draftName: string): string | null {
  if (driveMarkdownPayloadName(draftName).includes("/")) {
    return "drive.nameNoSlash";
  }
  return null;
}

export function completeDriveMarkdownCreate(): DriveMarkdownDraft {
  return {
    open: false,
    draftName: "",
  };
}

export function startDriveMove(file: DriveFile): DriveMoveDraft {
	return { target: file };
}

export function startDriveVersionHistory(file: DriveFile): { target: DriveFile } | null {
	return file.is_dir ? null : { target: file };
}

export function completeDriveMove(): DriveMoveCompletedState {
  return {
    moveTarget: null,
    selectedFile: undefined,
  };
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

export function startDriveRename(file: DriveFile): DriveRenameDraft {
  return {
    target: file,
    draftName: file.name,
  };
}

export function driveRenamePayloadName(draftName: string): string {
  return draftName.trim();
}

export function canSubmitDriveRename(
  target: DriveFile | null,
  draftName: string,
): boolean {
  if (!target) return false;
  const trimmed = driveRenamePayloadName(draftName);
  return Boolean(trimmed) && trimmed !== target.name && !trimmed.includes("/");
}

export function driveRenameErrorKey(draftName: string): string | null {
  if (driveRenamePayloadName(draftName).includes("/")) {
    return "drive.nameNoSlash";
  }
  return null;
}

export function buildDrivePreviewTitle(
  file: DriveFile,
  downloadHref: string,
): DrivePreviewTitle {
  return {
    fileName: file.name,
    downloadFileName: file.name,
    downloadHref,
    badge: previewBadge(file),
  };
}

export function buildDriveSearchRequest(
  query: string,
  currentPath: string,
  semantic: boolean,
): DriveSearchRequest | null {
  const text = query.trim();
  if (!text) return null;
  return {
    query: text,
    path: currentPath,
    semantic,
    limit: DRIVE_SEARCH_LIMIT,
  };
}

export function shouldStartDriveUpload(files: DriveUploadSelection): boolean {
  return Boolean(files && files.length > 0);
}

export function selectedDriveUploadFiles(files: DriveUploadSelection): File[] {
  if (!shouldStartDriveUpload(files)) return [];
  return Array.from(files!);
}

function previewBadge(file: DriveFile): DrivePreviewBadge | null {
  if (file.status === "processing") {
    return {
      labelKey: "drive.processing",
      tone: "processing",
    };
  }
  if (file.status === "failed") {
    return {
      labelKey: "drive.processFailed",
      tone: "failed",
    };
  }
  return null;
}
