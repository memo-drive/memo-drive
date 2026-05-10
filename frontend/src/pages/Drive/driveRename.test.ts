import { describe, expect, it } from "vitest";
import type { DriveFile } from "../../types";
import {
  canSubmitDriveRename,
  driveRenameErrorKey,
  driveRenamePayloadName,
  startDriveRename,
} from "./driveRename";

describe("driveRename", () => {
  it("starts with the selected file name", () => {
    const file = makeFile({ name: "report.pdf" });

    expect(startDriveRename(file)).toEqual({
      draftName: "report.pdf",
      target: file,
    });
  });

  it("submits a trimmed changed name", () => {
    const file = makeFile({ name: "report.pdf" });

    expect(canSubmitDriveRename(file, "  annual.pdf  ")).toBe(true);
    expect(driveRenamePayloadName("  annual.pdf  ")).toBe("annual.pdf");
  });

  it("rejects empty, unchanged, or slash-containing names", () => {
    const file = makeFile({ name: "report.pdf" });

    expect(canSubmitDriveRename(file, "   ")).toBe(false);
    expect(canSubmitDriveRename(file, " report.pdf ")).toBe(false);
    expect(canSubmitDriveRename(file, "archive/report.pdf")).toBe(false);
    expect(canSubmitDriveRename(null, "report-final.pdf")).toBe(false);
  });

  it("returns a visible error only for slash-containing names", () => {
    expect(driveRenameErrorKey("archive/report.pdf")).toBe("drive.nameNoSlash");
    expect(driveRenameErrorKey("report-final.pdf")).toBeNull();
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
