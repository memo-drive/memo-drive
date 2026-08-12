import { renderToString } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import { UploadConflictDialog } from "./UploadConflictDialog";

describe("UploadConflictDialog", () => {
  it("renders all three decisions and the batch option", () => {
    const html = renderToString(
      <UploadConflictDialog
        conflict={{
          file: new File(["pdf"], "report.pdf"),
          item: {
            requested_name: "report.pdf",
            normalized_name: "report.pdf",
            conflict: true,
            rename_suggestion: "report (1).pdf",
			replace_allowed: true,
          },
          current: 1,
          total: 2,
        }}
        onDecision={vi.fn()}
      />,
    );

    expect(html).toContain("report.pdf");
    expect(html).toContain("report (1).pdf");
    expect(html).toContain("uploadConflict.keepBoth");
    expect(html).toContain("uploadConflict.replace");
    expect(html).toContain("uploadConflict.skip");
    expect(html).toContain("uploadConflict.applyToRemaining");
  });

  it("does not offer Replace when the conflicting target cannot be replaced", () => {
    const html = renderToString(
      <UploadConflictDialog
        conflict={{
          file: new File(["pdf"], "Reports"),
          item: {
            requested_name: "Reports",
            normalized_name: "Reports",
            conflict: true,
            rename_suggestion: "Reports (1)",
            replace_allowed: false,
          },
          current: 1,
          total: 1,
        }}
        onDecision={vi.fn()}
      />,
    );

    expect(html).not.toContain("uploadConflict.replace");
  });
});
