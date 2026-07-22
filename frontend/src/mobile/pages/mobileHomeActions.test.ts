import { describe, expect, it, vi } from "vitest";
import { buildMobileHomeSearchRequest, startMobileHomeUploads } from "./mobileHomeActions";

describe("mobileHomeActions", () => {
  it("builds an all-drive File query for Home search", () => {
    expect(buildMobileHomeSearchRequest("  旅行  ")).toEqual({
      category: "all",
      query: "旅行",
      sort: "updated_at",
      limit: 50,
    });
    expect(buildMobileHomeSearchRequest("   ")).toBeNull();
  });

  it("starts Home uploads in the root Folder", () => {
    const file = new File(["memo"], "memo.txt", { type: "text/plain" });
    const upload = vi.fn().mockResolvedValue(undefined);

    expect(startMobileHomeUploads([file], upload)).toBe(1);
    expect(upload).toHaveBeenCalledWith(file, "/");
  });
});
