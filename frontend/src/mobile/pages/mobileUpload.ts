import {
  selectedDriveUploadFiles,
  type DriveUploadSelection,
} from "../../pages/Drive/driveUpload";

export type MobileUpload = (file: File, path: string) => Promise<unknown>;

export async function startMobileDriveUploads(
  files: DriveUploadSelection,
  path: string,
  upload: MobileUpload,
): Promise<number> {
  const selected = selectedDriveUploadFiles(files);
  await Promise.all(selected.map((file) => upload(file, path)));
  return selected.length;
}
