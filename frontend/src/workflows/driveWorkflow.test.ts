import { describe, expect, it } from "vitest";
import type { DriveFile } from "../types";
import { pickSearchResult } from "./driveWorkflow";

describe("driveWorkflow", () => {
  it("projects search picks into navigation or preview workflow state", () => {
    const file = makeFile({ id: "doc-1", name: "Plan.pdf", path: "/Work" });
    const folder = makeFile({
      id: "folder-1",
      is_dir: true,
      mime_type: "",
      name: "Invoices",
      path: "/Work",
      storage_path: "Work/Invoices",
    });

    expect(pickSearchResult(file)).toEqual({
      nextPath: null,
      previewFile: file,
      selectedFile: file,
    });
    expect(pickSearchResult(folder)).toEqual({
      nextPath: "/Work/Invoices",
      previewFile: null,
      selectedFile: folder,
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
