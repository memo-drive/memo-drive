import { describe, expect, it } from "vitest";
import {
  mobileFilesHref,
  mobilePreviewHref,
  normalizeMobilePath,
} from "./mobilePath";

describe("mobilePath", () => {
  it("builds mobile Files and Preview URLs from Folder paths", () => {
    expect(normalizeMobilePath("Docs//AI/")).toBe("/Docs/AI");
    expect(mobileFilesHref("/")).toBe("/m");
    expect(mobileFilesHref("/Docs/AI")).toBe("/m?path=%2FDocs%2FAI");
    expect(mobilePreviewHref("file-1", "/Docs/AI")).toBe(
      "/m/preview/file-1?path=%2FDocs%2FAI",
    );
  });
});
