import { describe, expect, it } from "vitest";
import { buildDriveSearchRequest } from "../../workflows/driveWorkflow";

describe("driveSearch", () => {
  it("does not build a request for blank queries", () => {
    expect(buildDriveSearchRequest("   ", "/Work", true)).toBeNull();
  });

  it("trims the query and scopes search to the current folder", () => {
    expect(buildDriveSearchRequest("  合同  ", "/Work", false)).toEqual({
      limit: 50,
      path: "/Work",
      query: "合同",
      semantic: false,
    });
  });

  it("keeps semantic search preference in the request", () => {
    expect(buildDriveSearchRequest("invoice", "/", true)?.semantic).toBe(true);
  });
});
