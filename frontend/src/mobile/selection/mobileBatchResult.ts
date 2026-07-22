import type { BatchFileResult } from "../../types";
import type { MessageType } from "../../components/base";

export function mobileBatchResultFeedback(
  result: BatchFileResult,
): { key: string; tone: MessageType } {
  if (result.failed <= 0) {
    return { key: "mobile.selection.batchSuccess", tone: "success" };
  }

  return {
    key: "mobile.selection.batchPartial",
    tone: result.succeeded > 0 ? "warning" : "error",
  };
}
