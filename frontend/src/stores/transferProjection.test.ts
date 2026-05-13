import { describe, expect, it } from "vitest";
import type { UploadSession } from "../types";
import {
  isActiveTransferStatus,
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
});

function makeSession(status: UploadSession["status"]): UploadSession {
  return {
    id: "upload-1",
    file_name: "Memo.pdf",
    file_size: 4096,
    chunk_size: 1024,
    uploaded_chunks: [0, 1],
    dest_path: "/Docs",
    status,
    created_at: "2026-05-10T00:00:00Z",
    expires_at: "2026-05-11T00:00:00Z",
  };
}
