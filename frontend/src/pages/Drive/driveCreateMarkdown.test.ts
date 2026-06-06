import { describe, expect, it } from "vitest";
import {
  canSubmitDriveMarkdown,
  completeDriveMarkdownCreate,
  driveMarkdownErrorKey,
  driveMarkdownPayloadName,
  startDriveMarkdownCreate,
} from "../../workflows/driveWorkflow";

describe("driveCreateMarkdown", () => {
  it("opens with an empty draft name", () => {
    expect(startDriveMarkdownCreate()).toEqual({
      draftName: "",
      open: true,
    });
  });

  it("submits a trimmed markdown file name", () => {
    expect(canSubmitDriveMarkdown("  Notes  ")).toBe(true);
    expect(driveMarkdownPayloadName("  Notes  ")).toBe("Notes");
  });

  it("rejects blank names and path separators", () => {
    expect(canSubmitDriveMarkdown("   ")).toBe(false);
    expect(canSubmitDriveMarkdown("folder/note")).toBe(false);
    expect(driveMarkdownErrorKey("folder/note")).toBe("drive.nameNoSlash");
  });

  it("closes and clears the draft after a successful create", () => {
    expect(completeDriveMarkdownCreate()).toEqual({
      draftName: "",
      open: false,
    });
  });
});
