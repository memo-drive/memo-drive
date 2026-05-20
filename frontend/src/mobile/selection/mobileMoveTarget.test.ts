import { describe, expect, it } from "vitest";
import type { DriveFile } from "../../types";
import { joinMobileMovePath, mobileMoveDisabledReason } from "./mobileMoveTarget";

describe("mobileMoveTarget", () => {
  it("joins Folder paths and blocks invalid move targets", () => {
    expect(joinMobileMovePath("/", "Docs")).toBe("/Docs");
    expect(joinMobileMovePath("/Docs", "AI")).toBe("/Docs/AI");

    expect(mobileMoveDisabledReason([makeFile({ path: "/Docs" })], "/Docs")).toBe("alreadyHere");
    expect(
      mobileMoveDisabledReason(
        [makeFile({ is_dir: true, path: "/Docs", name: "AI" })],
        "/Docs/AI/Notes",
      ),
    ).toBe("cannotMoveToSelf");
    expect(mobileMoveDisabledReason([makeFile({ path: "/Docs" })], "/Archive")).toBe("");
  });
});

function makeFile(overrides: Partial<DriveFile> = {}): DriveFile {
  return {
    id: "file-1",
    name: "Memo.pdf",
    path: "/",
    storage_path: "Memo.pdf",
    size: 1024,
    mime_type: "application/pdf",
    is_dir: false,
    status: "ready",
    chunk_count: 1,
    created_at: "2026-05-10T00:00:00Z",
    updated_at: "2026-05-10T00:00:00Z",
    ...overrides,
  };
}
