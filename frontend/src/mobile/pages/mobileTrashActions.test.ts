import { describe, expect, it, vi } from "vitest";
import type { DriveFile } from "../../types";
import {
  runMobileTrashEmpty,
  runMobileTrashPurge,
  runMobileTrashRestore,
} from "./mobileTrashActions";

describe("mobileTrashActions", () => {
  it("runs Trash Entry actions and refreshes after success", async () => {
    const file = makeFile();
    const refresh = vi.fn().mockResolvedValue(undefined);
    const restore = vi.fn().mockResolvedValue(undefined);
    const purge = vi.fn().mockResolvedValue(undefined);
    const empty = vi.fn().mockResolvedValue({ purged: 2 });

    await runMobileTrashRestore(file, { restore, refresh });
    await runMobileTrashPurge(file, { purge, refresh });
    const emptyResult = await runMobileTrashEmpty({ empty, refresh });

    expect(restore).toHaveBeenCalledWith(file.id);
    expect(purge).toHaveBeenCalledWith(file.id);
    expect(emptyResult).toEqual({ purged: 2 });
    expect(refresh).toHaveBeenCalledTimes(3);
  });
});

function makeFile(overrides: Partial<DriveFile> = {}): DriveFile {
  return {
    id: "trash-1",
    name: "trash-1-Memo.pdf",
    path: "/.trash",
    storage_path: ".trash/trash-1-Memo.pdf",
    size: 1024,
    mime_type: "application/pdf",
    is_dir: false,
    status: "ready",
    chunk_count: 1,
    created_at: "2026-05-10T00:00:00Z",
    updated_at: "2026-05-10T00:00:00Z",
    deleted_at: "2026-05-10T00:00:00Z",
    ...overrides,
  };
}
