import type { DriveFile } from "../../types";

export interface DriveRenameDraft {
  target: DriveFile;
  draftName: string;
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
