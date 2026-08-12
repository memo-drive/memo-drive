import { expect, it } from "vitest";
import type { DriveFile } from "../types";
import { appendFolderPage } from "./folderPagination";

it("appends the next Folder page without duplicating an overlapping File", () => {
  const current = [file("folder-a"), file("file-a")];
  const next = [file("file-a"), file("file-b")];

  expect(appendFolderPage(current, next).map((item) => item.id)).toEqual([
    "folder-a",
    "file-a",
    "file-b",
  ]);
});

function file(id: string): DriveFile {
  return {
    id,
    name: id,
    path: "/Docs",
    storage_path: `Docs/${id}`,
    size: 1,
    mime_type: "text/plain",
    is_dir: id.startsWith("folder"),
    status: "ready",
    chunk_count: 0,
    created_at: "2026-08-07T00:00:00Z",
    updated_at: "2026-08-07T00:00:00Z",
  };
}
