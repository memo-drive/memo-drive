import type { DriveFile } from "../../types";

export type FilePresentationKind =
  | "folder"
  | "image"
  | "video"
  | "audio"
  | "file";

export interface FilePresentation {
  description: string;
  iconName: string;
  kind: FilePresentationKind;
}

export function filePresentation(file: DriveFile): FilePresentation {
  if (file.is_dir) {
    return {
      description: "Folder",
      iconName: "folder",
      kind: "folder",
    };
  }
  const mimeType = file.mime_type || "";
  if (mimeType.startsWith("image/")) {
    return {
      description: mimeType,
      iconName: "image",
      kind: "image",
    };
  }
  if (mimeType.startsWith("video/")) {
    return {
      description: mimeType,
      iconName: "video_library",
      kind: "video",
    };
  }
  if (mimeType.startsWith("audio/")) {
    return {
      description: mimeType,
      iconName: "audio_file",
      kind: "audio",
    };
  }
  return {
    description: mimeType || "File",
    iconName: "description",
    kind: "file",
  };
}

export function fileSizeLabel(file: DriveFile): string {
  if (file.is_dir) return "--";
  return formatFileSize(file.size);
}

export function formatFileSize(size: number): string {
  if (size < 1024) return `${size} B`;
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`;
  if (size < 1024 * 1024 * 1024) {
    return `${(size / 1024 / 1024).toFixed(1)} MB`;
  }
  return `${(size / 1024 / 1024 / 1024).toFixed(2)} GB`;
}
