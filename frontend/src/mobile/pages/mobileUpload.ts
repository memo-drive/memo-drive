import {
  selectedDriveUploadFiles,
  type DriveUploadSelection,
} from "../../workflows/driveWorkflow";

export type MobileUpload = (file: File, path: string) => Promise<unknown>;
export type MobileUploadErrorHandler = (file: File, error: unknown) => void;

export function startMobileDriveUploads(
  files: DriveUploadSelection,
  path: string,
  upload: MobileUpload,
  onError?: MobileUploadErrorHandler,
): number {
  const selected = selectedDriveUploadFiles(files);
  selected.forEach((file) => {
    void upload(file, path).catch((error) => {
      onError?.(file, error);
    });
  });
  return selected.length;
}
