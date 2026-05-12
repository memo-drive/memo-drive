import { describe, expect, it, vi } from "vitest";
import { startMobileDriveUploads } from "./mobileUpload";

describe("mobileUpload", () => {
  it("starts one Upload Session per selected File", async () => {
    const first = new File(["a"], "a.txt", { type: "text/plain" });
    const second = new File(["b"], "b.txt", { type: "text/plain" });
    const upload = vi.fn().mockResolvedValue(undefined);

    expect(startMobileDriveUploads([first, second], "/Docs", upload)).toBe(2);

    expect(upload).toHaveBeenCalledWith(first, "/Docs");
    expect(upload).toHaveBeenCalledWith(second, "/Docs");
  });

  it("ignores empty selections", async () => {
    const upload = vi.fn().mockResolvedValue(undefined);

    expect(startMobileDriveUploads([], "/Docs", upload)).toBe(0);
    expect(upload).not.toHaveBeenCalled();
  });

  it("queues mobile uploads without waiting for the transfer to finish", () => {
    const file = new File(["slow"], "slow.jpg", { type: "image/jpeg" });
    const upload = vi.fn().mockReturnValue(new Promise(() => undefined));

    const count = startMobileDriveUploads([file], "/Camera", upload);

    expect(count).toBe(1);
    expect(upload).toHaveBeenCalledWith(file, "/Camera");
  });

  it("reports asynchronous upload failures to the caller", async () => {
    const file = new File(["bad"], "bad.jpg", { type: "image/jpeg" });
    const error = new Error("network down");
    const upload = vi.fn().mockRejectedValue(error);
    const onError = vi.fn();

    startMobileDriveUploads([file], "/Camera", upload, onError);
    await Promise.resolve();

    expect(onError).toHaveBeenCalledWith(file, error);
  });
});
