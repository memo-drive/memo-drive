import { describe, expect, it } from "vitest";
import type { DriveFile } from "../../types";
import {
  completeDriveMove,
  startDriveMove,
} from "./driveMove";

describe("driveMove", () => {
  it("starts move dialog with the chosen file", () => {
    const file = makeFile({ id: "file-1" });

    expect(startDriveMove(file)).toEqual({ target: file });
  });

  it("clears move target and selected file after move completes", () => {
    expect(completeDriveMove()).toEqual({
      moveTarget: null,
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
