import { describe, expect, it } from "vitest";
import {
  formatTransferSpeed,
  uploadedBytesForChunks,
  uploadPercentForBytes,
} from "./uploadProgress";

describe("uploadProgress", () => {
  it("counts the final partial chunk by actual file size", () => {
    expect(uploadedBytesForChunks([0, 1], 1024, 1500)).toBe(1500);
  });

  it("caps upload progress at the upload phase percent range", () => {
    expect(uploadPercentForBytes(500, 1000)).toBe(45);
    expect(uploadPercentForBytes(1000, 1000)).toBe(90);
  });

  it("formats transfer speed with byte units", () => {
    expect(formatTransferSpeed(1536)).toBe("1.5 KB/s");
  });
});
