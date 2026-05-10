import type { DriveFile } from "../../types";

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
