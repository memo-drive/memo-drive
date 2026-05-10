import { describe, expect, it } from "vitest";
import type { DriveFile } from "../../types";
import {
  confirmDriveDelete,
  startDriveDelete,
} from "./driveDelete";

describe("driveDelete", () => {
  it("starts delete confirmation with the chosen file", () => {
    const file = makeFile({ id: "file-1" });

    expect(startDriveDelete(file)).toEqual({ target: file });
  });

  it("clears delete target and selected file after a confirmed delete", () => {
    expect(confirmDriveDelete()).toEqual({
      deleteTarget: null,
      selectedFile: undefined,
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
