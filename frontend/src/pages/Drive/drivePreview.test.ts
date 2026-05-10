import { describe, expect, it } from "vitest";
import type { DriveFile } from "../../types";
import { buildDrivePreviewTitle } from "./drivePreview";

describe("drivePreview", () => {
  it("builds preview title data without a badge for ready files", () => {
    const file = makeFile({ name: "report.pdf", status: "ready" });

    expect(buildDrivePreviewTitle(file, "/download/file-1")).toEqual({
      badge: null,
      downloadFileName: "report.pdf",
      downloadHref: "/download/file-1",
      fileName: "report.pdf",
    });
  });

  it("adds a processing badge while File Indexing Pipeline is running", () => {
    const file = makeFile({ status: "processing" });

    expect(buildDrivePreviewTitle(file, "/download/file-1").badge).toEqual({
      labelKey: "drive.processing",
      tone: "processing",
    });
  });

  it("adds a failed badge when File Indexing Pipeline fails", () => {
    const file = makeFile({ status: "failed" });

    expect(buildDrivePreviewTitle(file, "/download/file-1").badge).toEqual({
      labelKey: "drive.processFailed",
      tone: "failed",
    });
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
