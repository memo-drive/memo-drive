import type { FileQueryRequest } from "../../types";
import {
  startMobileDriveUploads,
  type MobileUpload,
  type MobileUploadErrorHandler,
} from "./mobileUpload";
import type { DriveUploadSelection } from "../../workflows/driveWorkflow";

const HOME_UPLOAD_PATH = "/";

export function buildMobileHomeSearchRequest(query: string): FileQueryRequest | null {
  const trimmed = query.trim();
  if (!trimmed) return null;
  return {
    category: "all",
    query: trimmed,
    sort: "updated_at",
    limit: 50,
  };
}

export function startMobileHomeUploads(
  files: DriveUploadSelection,
  upload: MobileUpload,
  onError?: MobileUploadErrorHandler,
): number {
  return startMobileDriveUploads(files, HOME_UPLOAD_PATH, upload, onError);
}
