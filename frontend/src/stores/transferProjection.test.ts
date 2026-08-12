import { describe, expect, it } from "vitest";
import type { UploadSession } from "../types";
import {
  directoryTransferSummaries,
  preparingTransferTaskFromFile,
  isActiveTransferStatus,
  isLocalTransferTaskID,
  transferTaskFromSession,
  transferStatusFromSession,
  transferStatusLabelKey,
} from "./transferProjection";

describe("transferProjection", () => {
  it("projects Upload Session states into Transfer states", () => {
    expect(transferStatusFromSession(makeSession("done"))).toBe("done");
    expect(transferStatusFromSession(makeSession("cancelled"))).toBe("cancelled");
    expect(transferStatusFromSession(makeSession("expired"))).toBe("expired");
    expect(transferStatusFromSession(makeSession("failed"))).toBe("failed");
    expect(transferStatusFromSession(makeSession("merging"))).toBe("processing");
    expect(transferStatusFromSession(makeSession("uploading"))).toBe("paused");
    expect(
      transferStatusFromSession(makeSession("uploading"), {
        file: {} as File,
        status: "uploading",
      }),
    ).toBe("uploading");
  });

  it("exposes shared active-state and label-key rules", () => {
    expect(
      ["uploading", "paused", "processing"].filter(isActiveTransferStatus),
    ).toEqual(["uploading", "paused", "processing"]);
    expect(
      ["done", "failed", "cancelled", "expired"].some((status) =>
        isActiveTransferStatus(status),
      ),
    ).toBe(false);
    expect(transferStatusLabelKey("processing")).toBe("transfer.status.processing");
  });

  it("builds a Transfer task from an Upload Session with server progress", () => {
    const createdAt = new Date("2026-05-10T00:00:00Z").getTime();
    const task = transferTaskFromSession(makeSession("uploading"), {
      file: {} as File,
      status: "uploading",
      speed: 1536,
      error: "network jitter",
      createdAt: 123,
      updatedAt: 456,
    });

    expect(task).toMatchObject({
      id: "upload-1",
      fileName: "Memo.pdf",
      requestedName: "Memo.pdf",
      resolvedName: "Memo (1).pdf",
      overwritePolicy: "rename",
      fileSize: 4096,
      destPath: "/Docs",
      direction: "upload",
      status: "uploading",
      percent: 45,
      uploadedBytes: 2048,
      totalChunks: 4,
      chunkSize: 1024,
      speed: 1536,
      error: "network jitter",
      createdAt,
      updatedAt: 456,
      expiresAt: "2026-05-11T00:00:00Z",
    });

    expect(transferTaskFromSession(makeSession("merging")).percent).toBe(95);
  });

  it("builds a visible preparing task before an Upload Session exists", () => {
    const file = new File(["photo"], "IMG_8196.jpeg", { type: "image/jpeg" });

    const task = preparingTransferTaskFromFile(file, "/Camera", 12345);

    expect(task).toMatchObject({
      id: "local-upload:12345:IMG_8196.jpeg:5",
      fileName: "IMG_8196.jpeg",
      requestedName: "IMG_8196.jpeg",
      resolvedName: "IMG_8196.jpeg",
      overwritePolicy: "reject",
      fileSize: 5,
      destPath: "/Camera",
      direction: "upload",
      status: "preparing",
      percent: 0,
      uploadedBytes: 0,
      uploadedChunks: [],
      totalChunks: 1,
      speed: 0,
      createdAt: 12345,
      updatedAt: 12345,
      file,
    });
    expect(isActiveTransferStatus(task.status)).toBe(true);
    expect(transferStatusLabelKey(task.status)).toBe("transfer.status.preparing");
    expect(isLocalTransferTaskID(task.id)).toBe(true);
  });

  it("preserves directory batch context across Upload Session projection", () => {
    const file = new File(["a"], "a.txt");
    const preparing = preparingTransferTaskFromFile(file, "/Docs/Project", 123, {
      batchId: "batch-1",
      relativePath: "Project/a.txt",
    });

    expect(preparing).toMatchObject({
      directoryBatchId: "batch-1",
      relativePath: "Project/a.txt",
    });
    expect(transferTaskFromSession(makeSession("uploading"), preparing)).toMatchObject({
      directoryBatchId: "batch-1",
      relativePath: "Project/a.txt",
    });
  });

  it("derives directory progress from the existing File transfer tasks", () => {
    const first = preparingTransferTaskFromFile(
      new File(["1234"], "a.txt"),
      "/Docs/Project",
      1,
      { batchId: "batch-1", relativePath: "Project/a.txt" },
    );
    const second = preparingTransferTaskFromFile(
      new File(["123456"], "b.txt"),
      "/Docs/Project",
      2,
      { batchId: "batch-1", relativePath: "Project/b.txt" },
    );
    first.status = "done";
    first.uploadedBytes = 4;
    second.status = "uploading";
    second.uploadedBytes = 3;

    expect(directoryTransferSummaries([first, second])).toEqual([{
      batchId: "batch-1",
      name: "Project",
      fileCount: 2,
      completedCount: 1,
      failedCount: 0,
      uploadedBytes: 7,
      totalBytes: 10,
      percent: 70,
      status: "uploading",
    }]);
  });
});

function makeSession(status: UploadSession["status"]): UploadSession {
  return {
    id: "upload-1",
    file_name: "Memo.pdf",
    requested_name: "Memo.pdf",
    resolved_name: "Memo (1).pdf",
    overwrite_policy: "rename",
    file_size: 4096,
    chunk_size: 1024,
    uploaded_chunks: [0, 1],
    dest_path: "/Docs",
    status,
    created_at: "2026-05-10T00:00:00Z",
    expires_at: "2026-05-11T00:00:00Z",
  };
}
