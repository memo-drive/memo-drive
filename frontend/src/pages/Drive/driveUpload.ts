export type DriveUploadSelection = ArrayLike<File> | null;

export function shouldStartDriveUpload(files: DriveUploadSelection): boolean {
  return Boolean(files && files.length > 0);
}

export function selectedDriveUploadFiles(files: DriveUploadSelection): File[] {
  if (!shouldStartDriveUpload(files)) return [];
  return Array.from(files!);
}
