import { describe, expect, it } from "vitest";
import type { DriveFile } from "../../types";
import {
  buildDriveCrumbs,
  driveFolderPath,
  driveParentPath,
} from "../../workflows/driveWorkflow";

describe("drivePath", () => {
  it("builds compact breadcrumbs for deep folders", () => {
    expect(buildDriveCrumbs("/A/B/C/D", "Root", 3)).toEqual([
      { label: "Root", path: "/" },
      { label: "...", path: "" },
      { label: "C", path: "/A/B/C" },
      { label: "D", path: "/A/B/C/D" },
    ]);
  });

  it("returns the parent folder path", () => {
    expect(driveParentPath("/")).toBeNull();
    expect(driveParentPath("/A")).toBe("/");
    expect(driveParentPath("/A/B")).toBe("/A");
  });

  it("builds a folder path from its current listing location", () => {
    expect(driveFolderPath(makeFolder({ path: "/" }))).toBe("/Invoices");
    expect(driveFolderPath(makeFolder({ path: "/Work" }))).toBe("/Work/Invoices");
  });
});

function makeFolder(overrides: Partial<DriveFile> = {}): DriveFile {
  return {
    id: "folder-1",
    name: "Invoices",
    path: "/",
    storage_path: "Invoices",
    size: 0,
    mime_type: "",
    is_dir: true,
    status: "ready",
    chunk_count: 0,
    created_at: "2026-05-10T00:00:00Z",
    updated_at: "2026-05-10T00:00:00Z",
    ...overrides,
  };
}
