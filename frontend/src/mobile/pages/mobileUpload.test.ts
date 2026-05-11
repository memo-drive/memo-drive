import { describe, expect, it, vi } from "vitest";
import { startMobileDriveUploads } from "./mobileUpload";

describe("mobileUpload", () => {
  it("starts one Upload Session per selected File", async () => {
    const first = new File(["a"], "a.txt", { type: "text/plain" });
    const second = new File(["b"], "b.txt", { type: "text/plain" });
    const upload = vi.fn().mockResolvedValue(undefined);

    await expect(
      startMobileDriveUploads([first, second], "/Docs", upload),
    ).resolves.toBe(2);

    expect(upload).toHaveBeenCalledWith(first, "/Docs");
    expect(upload).toHaveBeenCalledWith(second, "/Docs");
  });

  it("ignores empty selections", async () => {
    const upload = vi.fn().mockResolvedValue(undefined);

    await expect(startMobileDriveUploads([], "/Docs", upload)).resolves.toBe(0);
    expect(upload).not.toHaveBeenCalled();
  });
});
