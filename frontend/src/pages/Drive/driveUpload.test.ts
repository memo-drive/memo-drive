import { describe, expect, it } from "vitest";
import {
  selectedDriveUploadFiles,
  shouldStartDriveUpload,
} from "../../workflows/driveWorkflow";

describe("driveUpload", () => {
  it("does not start upload for an empty selection", () => {
    expect(shouldStartDriveUpload(null)).toBe(false);
    expect(shouldStartDriveUpload([])).toBe(false);
  });

  it("converts selected files into a stable array", () => {
    const first = new File(["a"], "a.txt", { type: "text/plain" });
    const second = new File(["b"], "b.txt", { type: "text/plain" });

    expect(selectedDriveUploadFiles([first, second])).toEqual([first, second]);
  });
});
