import { describe, expect, it } from "vitest";
import type { DriveFile } from "../../types";
import {
  filePresentation,
  fileSizeLabel,
  formatFileSize,
} from "./filePresentation";

describe("filePresentation", () => {
  it("uses a stable fallback for unknown file types", () => {
    const file = makeFile({ mime_type: "" });

    expect(filePresentation(file)).toEqual({
      description: "File",
      iconName: "description",
      kind: "file",
    });
  });

  it("labels folders without a byte size", () => {
    const folder = makeFile({ is_dir: true, mime_type: "", size: 4096 });

    expect(filePresentation(folder)).toEqual({
      description: "Folder",
      iconName: "folder",
      kind: "folder",
    });
    expect(fileSizeLabel(folder)).toBe("--");
  });

  it("formats large files consistently", () => {
    const file = makeFile({ size: 3 * 1024 * 1024 * 1024 });

    expect(fileSizeLabel(file)).toBe("3.00 GB");
    expect(formatFileSize(1536)).toBe("1.5 KB");
  });
});

function makeFile(overrides: Partial<DriveFile> = {}): DriveFile {
  return {
    id: "file-1",
    name: "file.bin",
    path: "/",
    storage_path: "file.bin",
    size: 1,
    mime_type: "application/octet-stream",
    is_dir: false,
    status: "ready",
    chunk_count: 0,
    created_at: "2026-05-10T00:00:00Z",
    updated_at: "2026-05-10T00:00:00Z",
    ...overrides,
  };
}
