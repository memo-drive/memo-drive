import { describe, expect, it } from "vitest";
import {
  canSubmitDriveFolder,
  completeDriveFolderCreate,
  driveFolderPayloadName,
  startDriveFolderCreate,
} from "../../workflows/driveWorkflow";

describe("driveCreateFolder", () => {
  it("opens with an empty draft name", () => {
    expect(startDriveFolderCreate()).toEqual({
      draftName: "",
      open: true,
    });
  });

  it("submits a trimmed folder name", () => {
    expect(canSubmitDriveFolder("  Notes  ")).toBe(true);
    expect(driveFolderPayloadName("  Notes  ")).toBe("Notes");
  });

  it("rejects blank folder names", () => {
    expect(canSubmitDriveFolder("   ")).toBe(false);
  });

  it("closes and clears the draft after a successful create", () => {
    expect(completeDriveFolderCreate()).toEqual({
      draftName: "",
      open: false,
    });
  });
});
