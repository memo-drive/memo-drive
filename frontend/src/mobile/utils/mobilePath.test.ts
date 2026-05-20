import { describe, expect, it } from "vitest";
import {
  mobileFilesHref,
  mobilePreviewReturnHref,
  mobilePreviewHref,
  normalizeMobilePath,
} from "./mobilePath";

describe("mobilePath", () => {
  it("builds mobile Files and Preview URLs from Folder paths", () => {
    expect(normalizeMobilePath("Docs//AI/")).toBe("/Docs/AI");
    expect(mobileFilesHref("/")).toBe("/m/files");
    expect(mobileFilesHref("/Docs/AI")).toBe("/m/files?path=%2FDocs%2FAI");
    expect(mobilePreviewHref("file-1", "/Docs/AI")).toBe(
      "/m/preview/file-1?path=%2FDocs%2FAI",
    );
    expect(mobilePreviewHref("file-1", "/Docs/AI", "/m/category/documents")).toBe(
      "/m/preview/file-1?path=%2FDocs%2FAI&returnTo=%2Fm%2Fcategory%2Fdocuments",
    );
    expect(mobilePreviewReturnHref("?path=%2FDocs%2FAI")).toBe("/m/files?path=%2FDocs%2FAI");
    expect(mobilePreviewReturnHref("?returnTo=%2Fm%2Fcategory%2Fdocuments")).toBe("/m/category/documents");
    expect(mobilePreviewReturnHref("?returnTo=https%3A%2F%2Fexample.com&path=%2FDocs")).toBe("/m/files?path=%2FDocs");
  });
});
