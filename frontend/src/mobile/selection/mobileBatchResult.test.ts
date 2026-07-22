import { describe, expect, it } from "vitest";
import { mobileBatchResultFeedback } from "./mobileBatchResult";

describe("mobileBatchResultFeedback", () => {
  it("summarizes successful batch work without exposing per-file details", () => {
    expect(mobileBatchResultFeedback({ total: 3, succeeded: 3, failed: 0 })).toEqual({
      key: "mobile.selection.batchSuccess",
      tone: "success",
    });
  });

  it("uses a partial summary when some items fail", () => {
    expect(mobileBatchResultFeedback({ total: 3, succeeded: 2, failed: 1 })).toEqual({
      key: "mobile.selection.batchPartial",
      tone: "warning",
    });
  });

  it("marks fully failed batches as errors while keeping the same summary shape", () => {
    expect(mobileBatchResultFeedback({ total: 3, succeeded: 0, failed: 3 })).toEqual({
      key: "mobile.selection.batchPartial",
      tone: "error",
    });
  });
});
