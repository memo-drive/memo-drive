import { describe, expect, it } from "vitest";
import type { DriveFile } from "../../types";
import {
  pickDriveFile,
  pickSearchResult,
} from "./driveSelection";

describe("driveSelection", () => {
  it("opens file previews without changing the current folder", () => {
    const file = makeFile({ id: "doc-1", name: "Plan.pdf", path: "/Work" });

    expect(pickSearchResult(file)).toEqual({
      nextPath: null,
      previewFile: file,
      selectedFile: file,
    });
  });

  it("opens folders from search results without a preview", () => {
    const folder = makeFile({
      id: "folder-1",
      is_dir: true,
      mime_type: "",
      name: "Invoices",
      path: "/Work",
      storage_path: "Work/Invoices",
    });

    expect(pickSearchResult(folder)).toEqual({
      nextPath: "/Work/Invoices",
      previewFile: null,
      selectedFile: folder,
    });
  });

  it("selects files from the current folder listing with preview state", () => {
    const file = makeFile({ id: "image-1", mime_type: "image/png" });
    const folder = makeFile({ id: "folder-1", is_dir: true, mime_type: "" });

    expect(pickDriveFile(file).previewFile).toBe(file);
    expect(pickDriveFile(folder).previewFile).toBeNull();
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
