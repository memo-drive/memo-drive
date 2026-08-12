import { renderToString } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import { MobileUploadConflictSheet } from "./MobileUploadConflictSheet";

describe("MobileUploadConflictSheet", () => {
  it("uses a dedicated Bottom Sheet with touch-sized conflict actions", () => {
    const html = renderToString(
      <MobileUploadConflictSheet
        conflict={{
          file: new File(["pdf"], "report.pdf"),
          item: {
            requested_name: "report.pdf",
            normalized_name: "report.pdf",
            conflict: true,
            rename_suggestion: "report (1).pdf",
            replace_allowed: true,
          },
          current: 2,
          total: 3,
        }}
        onDecision={vi.fn()}
      />,
    );

    expect(html).toContain('data-mobile-upload-conflict="bottom-sheet"');
    expect(html).toContain("report (1).pdf");
    expect(html).toContain("uploadConflict.keepBoth");
    expect(html).toContain("uploadConflict.replace");
    expect(html).toContain("uploadConflict.skip");
  });

  it("hides Replace when the conflicting target cannot be replaced", () => {
    const html = renderToString(
      <MobileUploadConflictSheet
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
