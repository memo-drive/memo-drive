export interface DriveFolderDraft {
  open: boolean;
  draftName: string;
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
